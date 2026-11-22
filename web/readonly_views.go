// Package web 的 readonly_views.go 提供只读扩展视角：
//   - GET /api/profiles  列举全部 Profile 及其成员在 Registry 中的有效性；
//   - GET /api/skill     返回单个已注册技能的评分明细、Lint 发现、Origin 元数据与 SKILL.md 内容；
//   - GET /api/plan      对已登记项目或当前启动工作区生成只读 Curation Plan 预览。
//
// 所有接口仅做只读查询，绝不写盘、绝不读取未登记的任意路径。
//
// Input: errors, fmt, net/http, os, path/filepath, sort, github.com/woyin/skills-manager/internal/curation, github.com/woyin/skills-manager/internal/profile, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/internal/tool
// Output: func (*Handler).handleProfiles, func (*Handler).handleSkill, func (*Handler).handlePlan
// Pos: API层-Web只读扩展视图处理器
package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"

	"github.com/woyin/skills-manager/internal/curation"
	"github.com/woyin/skills-manager/internal/profile"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

const maxSkillContentBytes = 256 * 1024

type profileItem struct {
	Name   string   `json:"name"`
	Skills []string `json:"skills"`
	MCP    []string `json:"mcp"`
	Valid  bool     `json:"valid"`
	Issues []string `json:"issues"`
}

type lintFindingItem struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

type skillLintDetail struct {
	HasErrors bool              `json:"has_errors"`
	Errors    int               `json:"errors"`
	Warnings  int               `json:"warnings"`
	Findings  []lintFindingItem `json:"findings"`
}

type skillOriginDetail struct {
	HasFile     bool   `json:"has_file"`
	Valid       bool   `json:"valid"`
	UpdateClass string `json:"update_class"`
	SourceKind  string `json:"source_kind,omitempty"`
	Source      string `json:"source,omitempty"`
	Ref         string `json:"ref,omitempty"`
	RefKind     string `json:"ref_kind,omitempty"`
	SubPath     string `json:"sub_path,omitempty"`
	Commit      string `json:"commit,omitempty"`
}

type skillDetailResponse struct {
	Name             string               `json:"name"`
	Category         string               `json:"category"`
	Path             string               `json:"path"`
	SourceURL        string               `json:"source_url,omitempty"`
	LastUpdated      string               `json:"last_updated"`
	Origin           skillOriginDetail    `json:"origin"`
	Score            *registry.SkillScore `json:"score"`
	Lint             skillLintDetail      `json:"lint"`
	Content          string               `json:"content"`
	ContentTruncated bool                 `json:"content_truncated,omitempty"`
}

type planResponse struct {
	Project             string              `json:"project"`
	Baseline            *curation.Baseline  `json:"baseline"`
	Bootstrap           bool                `json:"bootstrap"`
	Proposals           []curation.Proposal `json:"proposals"`
	Warnings            []string            `json:"warnings"`
	CheckOK             bool                `json:"check"`
	ExplicitTargetSet   bool                `json:"has_explicit_target"`
	RecommendedProfiles []string            `json:"recommended_profiles"`
	RecommendedSkills   []string            `json:"recommended_skills"`
	Agents              []string            `json:"agents"`
}

// handleProfiles 列举全部 Profile 并针对当前 Registry 校验其成员是否存在且唯一。
func (h *Handler) handleProfiles(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	if h.profilesDir == "" {
		writeJSON(w, []profileItem{})
		return
	}

	loader := profile.NewLoader(h.profilesDir)
	names, err := loader.List()
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, []profileItem{})
			return
		}
		writeError(w, http.StatusInternalServerError, "listing profiles: "+err.Error())
		return
	}
	sort.Strings(names)

	items := make([]profileItem, 0, len(names))
	for _, name := range names {
		p, loadErr := loader.Load(name)
		if loadErr != nil {
			items = append(items, profileItem{
				Name:   name,
				Skills: []string{},
				MCP:    []string{},
				Valid:  false,
				Issues: []string{loadErr.Error()},
			})
			continue
		}
		skills := nonNilStrings(p.Skills)
		mcps := nonNilStrings(p.MCP)
		issues := h.validateProfileMembers(skills, mcps)
		items = append(items, profileItem{
			Name:   name,
			Skills: skills,
			MCP:    mcps,
			Valid:  len(issues) == 0,
			Issues: issues,
		})
	}
	writeJSON(w, items)
}

func (h *Handler) validateProfileMembers(skills, mcps []string) []string {
	issues := make([]string, 0)
	for _, s := range skills {
		if _, err := os.Stat(filepath.Join(h.registry.Dir(), "skills", s)); err == nil {
			continue // 兼容 category 级引用
		}
		if _, err := h.registry.ResolveUniqueSkill(s); err != nil {
			issues = append(issues, fmt.Sprintf("skill %q: %v", s, err))
		}
	}
	for _, m := range mcps {
		if _, err := os.Stat(h.registry.GetMCPPath(m)); err != nil {
			issues = append(issues, fmt.Sprintf("mcp %q: not found in registry", m))
		}
	}
	return issues
}

// handleSkill 返回单个已注册技能的评分、Lint 发现、Origin 元数据与 SKILL.md 文本。
// 目标严格从 ListSkillDetails 查找，杜绝路径穿越。
func (h *Handler) handleSkill(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	name := r.URL.Query().Get("name")
	category := r.URL.Query().Get("category")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing required query parameter: name")
		return
	}

	details, err := h.registry.ListSkillDetails()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing skill details: "+err.Error())
		return
	}

	var matches []registry.ItemDetail
	for cat, items := range details {
		if category != "" && cat != category {
			continue
		}
		for _, item := range items {
			if item.Name == name {
				matches = append(matches, item)
			}
		}
	}
	if len(matches) == 0 {
		writeError(w, http.StatusNotFound, fmt.Sprintf("skill %q not found in registry", name))
		return
	}
	if len(matches) > 1 {
		writeError(w, http.StatusConflict, fmt.Sprintf("skill %q exists in multiple categories; specify category", name))
		return
	}

	item := matches[0]
	rel := filepath.Join(item.Category, item.Name)
	score := h.registry.ScoreSkill(rel)
	lint := h.registry.LintSkill(rel)
	originRead := h.registry.ReadOrigin(item.Path)

	findings := make([]lintFindingItem, 0, len(lint.Findings))
	errCount, warnCount := countLintFindings(lint)
	for _, f := range lint.Findings {
		level := "warning"
		if f.Level == registry.LintError {
			level = "error"
		}
		findings = append(findings, lintFindingItem{Level: level, Message: f.Message})
	}

	content, truncated, readErr := readSkillMarkdown(filepath.Join(item.Path, "SKILL.md"))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, "reading SKILL.md: "+readErr.Error())
		return
	}

	writeJSON(w, skillDetailResponse{
		Name:        item.Name,
		Category:    item.Category,
		Path:        item.Path,
		SourceURL:   item.SourceURL,
		LastUpdated: item.LastUpdated,
		Origin: skillOriginDetail{
			HasFile:     originRead.HasFile,
			Valid:       originRead.Valid,
			UpdateClass: string(originRead.Class),
			SourceKind:  string(originRead.Origin.SourceKind),
			Source:      originRead.Origin.Source,
			Ref:         originRead.Origin.Ref,
			RefKind:     string(originRead.Origin.RefKind),
			SubPath:     originRead.Origin.SubPath,
			Commit:      originRead.Origin.Commit,
		},
		Score: score,
		Lint: skillLintDetail{
			HasErrors: lint.HasErrors(),
			Errors:    errCount,
			Warnings:  warnCount,
			Findings:  findings,
		},
		Content:          content,
		ContentTruncated: truncated,
	})
}

// handlePlan 生成指定项目的只读 Curation Plan 预览。
// 仅允许预览已登记到数据库的项目或启动 sm web 时的当前工作目录，拒绝探测任意磁盘路径。
func (h *Handler) handlePlan(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}

	projectDir, statusCode, err := h.resolveAllowedProject(r.URL.Query().Get("project"))
	if err != nil {
		writeError(w, statusCode, err.Error())
		return
	}

	tools := h.resolvedTools()
	planner := curation.NewPlanner(h.registry.Dir(), h.profilesDir, tools, projectDir)
	plan, err := planner.PlanForProject()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "generating curation plan: "+err.Error())
		return
	}

	agentNames := make([]string, 0, len(tools))
	for _, t := range tools {
		agentNames = append(agentNames, t.Name)
	}
	proposals := plan.Proposals
	if proposals == nil {
		proposals = []curation.Proposal{}
	}

	writeJSON(w, planResponse{
		Project:             plan.Project,
		Baseline:            plan.Baseline,
		Bootstrap:           plan.Bootstrap,
		Proposals:           proposals,
		Warnings:            nonNilStrings(plan.Warnings),
		CheckOK:             plan.CheckOK,
		ExplicitTargetSet:   plan.ExplicitTargetSet,
		RecommendedProfiles: nonNilStrings(plan.RecommendedProfiles),
		RecommendedSkills:   nonNilStrings(plan.RecommendedSkills),
		Agents:              agentNames,
	})
}

func (h *Handler) resolveAllowedProject(requested string) (string, int, error) {
	allowed := make(map[string]string) // canonical -> original path
	addAllowed := func(p string) {
		if p == "" {
			return
		}
		allowed[canonicalPath(p)] = p
	}

	addAllowed(h.currentProject)
	if h.database != nil {
		projects, err := h.database.GetAllProjects()
		if err != nil {
			return "", http.StatusInternalServerError, fmt.Errorf("listing projects: %w", err)
		}
		for _, p := range projects {
			addAllowed(p.Path)
		}
	}

	target := requested
	if target == "" {
		if h.currentProject == "" {
			return "", http.StatusBadRequest, errors.New("missing required query parameter: project")
		}
		target = h.currentProject
	}

	matched, ok := allowed[canonicalPath(target)]
	if !ok {
		return "", http.StatusForbidden, errors.New("project path is not a registered project or active workspace")
	}

	info, err := os.Stat(matched)
	if err != nil {
		if os.IsNotExist(err) {
			return "", http.StatusNotFound, fmt.Errorf("project directory does not exist: %s", matched)
		}
		return "", http.StatusInternalServerError, fmt.Errorf("checking project directory: %w", err)
	}
	if !info.IsDir() {
		return "", http.StatusBadRequest, fmt.Errorf("project path is not a directory: %s", matched)
	}
	return matched, http.StatusOK, nil
}

func (h *Handler) resolvedTools() []tool.Tool {
	if h.detectTools != nil {
		return h.detectTools()
	}
	detected := tool.DetectInstalled(tool.AllTools())
	if len(detected) == 0 {
		return tool.DefaultTools()
	}
	return detected
}

func countLintFindings(lint *registry.LintResult) (errCount, warnCount int) {
	if lint == nil {
		return 0, 0
	}
	for _, f := range lint.Findings {
		if f.Level == registry.LintError {
			errCount++
		} else {
			warnCount++
		}
	}
	return errCount, warnCount
}

func readSkillMarkdown(path string) (content string, truncated bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxSkillContentBytes+1))
	if err != nil {
		return "", false, err
	}
	if len(data) <= maxSkillContentBytes {
		return string(data), false, nil
	}
	cut := maxSkillContentBytes
	for cut > 0 && !utf8.RuneStart(data[cut]) {
		cut--
	}
	return string(data[:cut]), true, nil
}

func canonicalPath(p string) string {
	abs, err := filepath.Abs(filepath.Clean(p))
	if err != nil {
		return filepath.Clean(p)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

func nonNilStrings(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}

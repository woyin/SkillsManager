// Package web 提供 sm 内嵌的 Web 仪表盘：静态前端 + JSON REST API。
//
// 路由（见 RegisterRoutes）：
//
//	/              内嵌的 index.html
//	/static/*      内嵌的静态资源（CSS/JS）
//	/api/registry  注册表内容（skills + MCP）
//	/api/projects  已记录的项目列表
//	/api/history   安装历史
//	/api/check     完整性检查（失效/孤立符号链接、丢失项目）
//	/api/tools     工具目录及安装状态
//	/api/aivo      aivo 状态（若安装）
//	/api/profiles  Profile 清单及成员校验状态
//	/api/skill     单个技能详情（评分、Lint、Origin 与 SKILL.md 内容）
//	/api/plan      指定项目的 Curation Plan 只读预览
//
// Input: embed, encoding/json, net/http, path/filepath, github.com/woyin/skills-manager/internal/aivo, github.com/woyin/skills-manager/internal/db, github.com/woyin/skills-manager/internal/health, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/internal/tool
// Output: type Handler, func NewHandler
// Pos: API层-Web仪表盘处理器
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package web

import (
	"embed"
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/woyin/skills-manager/internal/aivo"
	"github.com/woyin/skills-manager/internal/db"
	"github.com/woyin/skills-manager/internal/health"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

//go:embed static/*
var staticFiles embed.FS

// Handler 提供内嵌仪表盘 UI 与 JSON REST API。
type Handler struct {
	registry       *registry.Registry
	database       *db.DB
	profilesDir    string
	currentProject string
	detectTools    func() []tool.Tool
}

// aivoResponse 是 /api/aivo 端点的返回载荷。
type aivoResponse struct {
	Installed     bool   `json:"installed"`
	Version       string `json:"version"`
	Path          string `json:"path"`
	ActiveKey     string `json:"active_key,omitempty"`
	ActiveModel   string `json:"active_model,omitempty"`
	KeysCount     int    `json:"keys_count"`
	HealthyKeys   int    `json:"healthy_keys"`
	UnhealthyKeys int    `json:"unhealthy_keys"`
	TotalTokens   int64  `json:"total_tokens,omitempty"`
	Sessions      int    `json:"sessions,omitempty"`
	Models        int    `json:"models,omitempty"`
}

// skillQualitySummary 是注册表总览中单个技能的质量与来源摘要。
type skillQualitySummary struct {
	Score       int    `json:"score"`
	Errors      int    `json:"errors"`
	Warnings    int    `json:"warnings"`
	UpdateClass string `json:"update_class"`
	SourceKind  string `json:"source_kind,omitempty"`
	RefKind     string `json:"ref_kind,omitempty"`
}

// registryResponse 是 /api/registry 端点的返回载荷。
type registryResponse struct {
	Skills       map[string][]string              `json:"skills"`
	MCP          []string                         `json:"mcp"`
	SkillDetails map[string][]registry.ItemDetail `json:"skill_details"`
	MCPDetails   []registry.ItemDetail            `json:"mcp_details"`
	SkillQuality map[string]skillQualitySummary   `json:"skill_quality"`
}

// checkIssue 是 /api/check 报告的一条完整性问题。
type checkIssue struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// checkResponse 是 /api/check 端点的返回载荷。
type checkResponse struct {
	Status string       `json:"status"`
	Issues []checkIssue `json:"issues"`
}

// NewHandler 返回由指定 registry 与 database 支撑的 Handler。
// database 为 nil 时，project/history 端点返回空数据。
func NewHandler(reg *registry.Registry, database *db.DB) *Handler {
	return &Handler{registry: reg, database: database}
}

// WithProfilesDir 配置 profiles 目录，供 /api/profiles 与 /api/plan 预览使用。
func (h *Handler) WithProfilesDir(dir string) *Handler {
	h.profilesDir = dir
	return h
}

// WithCurrentProject 配置启动 sm web 时的工作目录，允许 /api/plan 只读预览该目录。
func (h *Handler) WithCurrentProject(dir string) *Handler {
	h.currentProject = dir
	return h
}

// RegisterRoutes 把所有只读 web API 端点挂到 mux 上，"/" 返回嵌入式前端，
// /static/ 托管静态资源。
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/registry", h.handleRegistry)
	mux.HandleFunc("/api/projects", h.handleProjects)
	mux.HandleFunc("/api/history", h.handleHistory)
	mux.HandleFunc("/api/check", h.handleCheck)
	mux.HandleFunc("/api/tools", h.handleTools)
	mux.HandleFunc("/api/aivo", h.handleAivo)
	mux.HandleFunc("/api/profiles", h.handleProfiles)
	mux.HandleFunc("/api/skill", h.handleSkill)
	mux.HandleFunc("/api/plan", h.handlePlan)
	mux.HandleFunc("/", h.handleIndex)
	mux.Handle("/static/", http.FileServer(http.FS(staticFiles)))
}

// requireGET 强制只读 API 仅接受 GET 请求，其他方法返回 405。
func requireGET(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return false
	}
	return true
}

// handleIndex 服务嵌入式前端入口 index.html；非根路径返回 404。
func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

// handleRegistry 聚合 registry 的技能/MCP 名称与详情，一次性返回给前端。
// 空切片归一化为 []（而非 null），方便前端解构。
func (h *Handler) handleRegistry(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	skills, err := h.registry.ListSkills()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing skills: "+err.Error())
		return
	}
	mcps, err := h.registry.ListMCP()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing mcp: "+err.Error())
		return
	}
	if mcps == nil {
		mcps = []string{}
	}
	skillDetails, err := h.registry.ListSkillDetails()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing skill details: "+err.Error())
		return
	}
	mcpDetails, err := h.registry.ListMCPDetails()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing mcp details: "+err.Error())
		return
	}
	if mcpDetails == nil {
		mcpDetails = []registry.ItemDetail{}
	}

	quality := make(map[string]skillQualitySummary)
	for cat, items := range skillDetails {
		for _, item := range items {
			rel := filepath.Join(cat, item.Name)
			score := h.registry.ScoreSkill(rel)
			lint := h.registry.LintSkill(rel)
			origin := h.registry.ReadOrigin(item.Path)
			errCount, warnCount := countLintFindings(lint)
			quality[cat+"/"+item.Name] = skillQualitySummary{
				Score:       score.Total,
				Errors:      errCount,
				Warnings:    warnCount,
				UpdateClass: string(origin.Class),
				SourceKind:  string(origin.Origin.SourceKind),
				RefKind:     string(origin.Origin.RefKind),
			}
		}
	}

	resp := registryResponse{
		Skills:       skills,
		MCP:          mcps,
		SkillDetails: skillDetails,
		MCPDetails:   mcpDetails,
		SkillQuality: quality,
	}
	writeJSON(w, resp)
}

// handleProjects 返回数据库中记录的全部项目；无数据库时返回空数组。
func (h *Handler) handleProjects(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	if h.database == nil {
		writeJSON(w, []any{})
		return
	}
	projects, err := h.database.GetAllProjects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing projects: "+err.Error())
		return
	}
	writeJSON(w, projects)
}

// handleHistory 返回全部安装历史记录；无数据库时返回空数组。
func (h *Handler) handleHistory(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	if h.database == nil {
		writeJSON(w, []any{})
		return
	}
	installs, err := h.database.GetAllInstallations()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing installations: "+err.Error())
		return
	}
	writeJSON(w, installs)
}

// handleCheck 执行与 `sm check` 一致的安装完整性检查，返回 JSON。
// 三类问题：
//   - broken_symlink    符号链接目标缺失（Stat 目标不存在）；
//   - orphaned_symlink  链接目标不在 registry 内（PointInside 判据）——即非
//     sm 管理的链接，无法被 update/check 修复；
//   - missing_project   数据库记录的项目目录已不在磁盘上。
//
// status 为 "ok"（无问题）或 "issues"。
func (h *Handler) handleCheck(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	tools := tool.AllTools()
	dirs := make([]string, 0, len(tools))
	for _, t := range tools {
		dirs = append(dirs, tool.GetGlobalSkillDir(t))
	}

	var knownProjects []string
	if h.database != nil {
		projects, err := h.database.GetAllProjects()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "listing projects: "+err.Error())
			return
		}
		for _, project := range projects {
			knownProjects = append(knownProjects, project.Path)
		}
	}

	report, err := health.Scan(dirs, h.registry.Dir(), knownProjects)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "checking installations: "+err.Error())
		return
	}

	issues := make([]checkIssue, 0, len(report.Issues))
	for _, issue := range report.Issues {
		issues = append(issues, checkIssue{Type: string(issue.Type), Path: issue.Path})
	}

	status := "ok"
	if len(issues) > 0 {
		status = "issues"
	}
	writeJSON(w, checkResponse{Status: status, Issues: issues})
}

// handleTools 返回每个支持工具的探测状态：是否安装、是否有 skill 目录。
// 供前端展示"Detected Agents"。
func (h *Handler) handleTools(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	tools := tool.AllTools()
	type toolInfo struct {
		Name        string `json:"name"`
		Installed   bool   `json:"installed"`
		HasSkillDir bool   `json:"has_skill_dir"`
	}

	result := make([]toolInfo, len(tools))
	for i, t := range tools {
		result[i] = toolInfo{
			Name:        t.Name,
			Installed:   tool.IsInstalled(t),
			HasSkillDir: tool.HasSkillDir(t),
		}
	}

	writeJSON(w, result)
}

// handleAivo 返回 aivo 的安装信息与当前激活的 API key/模型；未安装时仅
// 返回安装状态字段。
func (h *Handler) handleAivo(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	info := aivo.Detect()

	resp := aivoResponse{
		Installed: info.Installed,
		Version:   info.Version,
		Path:      info.Path,
	}

	if !info.Installed {
		writeJSON(w, resp)
		return
	}

	keys := aivo.ListKeys()
	active := aivo.ActiveKeyFromKeys(keys)
	if active != nil {
		resp.ActiveKey = active.Name
		resp.ActiveModel = active.BaseURL
	}

	resp.KeysCount = len(keys)

	healthy, unhealthy := 0, 0
	for _, k := range keys {
		if k.PingOK != nil && *k.PingOK {
			healthy++
		} else {
			unhealthy++
		}
	}
	resp.HealthyKeys = healthy
	resp.UnhealthyKeys = unhealthy

	if stats := aivo.GetStats(); stats != nil {
		resp.TotalTokens = stats.TotalTokens
		resp.Sessions = stats.Sessions
		resp.Models = stats.Models
	}

	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

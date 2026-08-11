// Package curation 实现 Curation Core：为项目生成、预览并原子应用
// Curation Plan（ADR 0020/0021/0022/0023/0027/0028）。
//
// Curation Plan 是针对项目 Installed Skills 的可解释、可预览的拟改集合，
// 每条拟改都带有理由。默认只读；必须由用户确认或通过显式 --apply 应用，
// 并采用与 Profile Install 相同的原子性标准。计划只能移除计划拥有的
// 项目级 Link Install（ADR 0023），绝不自动移除手动/Copy/未拥有实体。
//
// 分层 baseline（ADR 0021）：显式 Profile / .sm.json 优先，Team Catalog
// 政策次之（tranche 1 预留，未实现），项目环境推断仅作非绑定建议。
//
// Input: strings
// Output: type Action, type Proposal, type Plan, type Baseline
// Pos: 业务层-策划(Curation)模块
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package curation

import "strings"

// Action 是计划对某个技能拟议的动作。
type Action string

const (
	ActionAdd    Action = "add"
	ActionRemove Action = "remove"
	ActionLeave  Action = "leave"
)

// Proposal 描述计划对 (agent, skill) 的一条拟改。
type Proposal struct {
	Action   Action   `json:"action"`
	Skill    string   `json:"skill"`
	Agent    string   `json:"agent"`
	Reason   string   `json:"reason"`
	Evidence []string `json:"evidence,omitempty"`
	Owned    bool     `json:"owned"` // remove 时：是否计划拥有，可安全移除
	Path     string   `json:"path,omitempty"`
}

// Baseline 是计划所基于的显式组成目标。
type Baseline struct {
	Profile    string   `json:"profile,omitempty"`
	Skills     []string `json:"skills,omitempty"`
	MCP        []string `json:"mcp,omitempty"`
	HasProfile bool     `json:"has_profile"`
}

// IsBootstrap 报告是否无显式目标（需用户先选定目标才能应用）。
func (b *Baseline) IsBootstrap() bool {
	return b == nil || (!b.HasProfile && len(b.Skills) == 0 && len(b.MCP) == 0)
}

// Has 报告基线是否包含指定技能名（profile 展开 + 附加项）。
func (b *Baseline) Has(skill string) bool {
	return contains(b.Skills, skill)
}

// ExpandSkillNames 返回含 profile 展开的完整技能名集合。
func (b *Baseline) ExpandSkillNames(profileSkills []string) []string {
	var out []string
	if b != nil {
		out = append(out, profileSkills...)
		out = append(out, b.Skills...)
	}
	return dedupe(out)
}

func contains(items []string, s string) bool {
	for _, i := range items {
		if i == s {
			return true
		}
	}
	return false
}

func dedupe(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, i := range items {
		if !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}
	return out
}

// normalizedEqual 比较两个技能名集合是否相等（忽略顺序）。
func normalizedEqual(a, b []string) bool {
	return strings.Join(sortCopy(a), ",") == strings.Join(sortCopy(b), ",")
}

func sortCopy(items []string) []string {
	out := append([]string(nil), items...)
	// 简单插入排序避免引入额外依赖。
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Evaluated 是自动化的机器可读表示（ADR 0022：JSON 输出供 CI/集成）。
// 提供与 Plan 相同的信息但不绑定内部结构，便于稳定序列化。
type Evaluated struct {
	Project             string     `json:"project"`
	Baseline            *Baseline  `json:"baseline"`
	Bootstrap           bool       `json:"bootstrap"`
	HasExplicitTarget   bool       `json:"has_explicit_target"`
	Proposals           []Proposal `json:"proposals"`
	Warnings            []string   `json:"warnings,omitempty"`
	RecommendedProfiles []string   `json:"recommended_profiles,omitempty"`
	RecommendedSkills   []string   `json:"recommended_skills,omitempty"`
	Check               bool       `json:"check"`
}

// Evaluate 返回计划的可序列化表示。
func (pl *Plan) Evaluate() *Evaluated {
	return &Evaluated{
		Project:             pl.Project,
		Baseline:            pl.Baseline,
		Bootstrap:           pl.Bootstrap,
		HasExplicitTarget:   pl.ExplicitTargetSet,
		Proposals:           pl.Proposals,
		Warnings:            pl.Warnings,
		RecommendedProfiles: pl.RecommendedProfiles,
		RecommendedSkills:   pl.RecommendedSkills,
		Check:               pl.CheckOK,
	}
}

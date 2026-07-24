// Package registry 的 lint.go 对入库的 SKILL.md 做轻量质量校验。
//
// Skill（SKILL.md）正成为跨 agent 的事实标准单元。Anthropic 规范要求
// frontmatter 含 name 与 description，其中 description 直接决定 agent
// 何时触发该 skill。本文件复用 parseSkillFrontmatter 的解析结果，不重复
// 实现 YAML 扫描，只在其上加规则判断。
//
// Input: os, path/filepath
// Output: type LintLevel, type LintFinding, type LintResult, func LintSkill
// Pos: 数据层-技能lint
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package registry

import (
	"os"
	"path/filepath"
)

// LintLevel 标识问题严重程度。
type LintLevel int

const (
	LintError   LintLevel = iota // 阻断性：skill 基本不可用
	LintWarning                  // 建议修复：影响触发质量
)

// LintFinding 是一条校验发现。
type LintFinding struct {
	Level   LintLevel
	Message string
}

// LintResult 汇总单个技能目录的校验结果。无问题时 Findings 为空。
type LintResult struct {
	SkillDir string // 技能目录（相对注册表根）
	Findings []LintFinding
}

// HasErrors 报告是否存在 Error 级别发现。
func (r *LintResult) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Level == LintError {
			return true
		}
	}
	return false
}

// 依据 Anthropic Skill 规范与跨工具兼容经验设的阈值。
const (
	minDescriptionLen = 20   // 过短难以让 agent 判断触发时机
	maxDescriptionLen = 1024 // 超出常见 agent 上下文预算
)

// LintSkill 校验单个技能目录下的 SKILL.md frontmatter。
// skillDir 为注册表内的相对路径（如 "global/my-skill"）。
// 返回的 LintResult 始终非 nil。
func (r *Registry) LintSkill(skillDir string) *LintResult {
	res := &LintResult{SkillDir: skillDir}
	abs := filepath.Join(r.skillsDir(), skillDir)
	skillMD := filepath.Join(abs, "SKILL.md")

	fm := parseSkillFrontmatter(skillMD)
	// parseSkillFrontmatter 在文件缺失/读取失败时返回零值；
	// 这里通过判断 name 与 description 同时为空来识别"未解析到 frontmatter"。
	if fm.Name == "" && fm.Description == "" {
		// 区分"文件不存在"与"存在但无 frontmatter"。
		if _, err := os.Stat(skillMD); err != nil {
			res.Findings = append(res.Findings, LintFinding{LintError, "missing SKILL.md"})
			return res
		}
	}

	if fm.Name == "" {
		res.Findings = append(res.Findings, LintFinding{LintError, "frontmatter missing required field: name"})
	} else if !isValidSkillName(fm.Name) {
		res.Findings = append(res.Findings, LintFinding{
			LintWarning,
			"name contains characters outside [a-z0-9-]; may break cross-tool compatibility",
		})
	}

	if fm.Description == "" {
		res.Findings = append(res.Findings, LintFinding{LintError, "frontmatter missing required field: description"})
	} else {
		n := len([]rune(fm.Description))
		if n < minDescriptionLen {
			res.Findings = append(res.Findings, LintFinding{
				LintWarning,
				"description too short (< 20 chars); agent may never trigger this skill",
			})
		} else if n > maxDescriptionLen {
			res.Findings = append(res.Findings, LintFinding{
				LintWarning,
				"description too long (> 1024 chars); may exceed agent context budget",
			})
		}
	}

	return res
}

// isValidSkillName 校验技能名仅含小写字母、数字与连字符。
// 这是跨 agent 目录/URL 安全的字符集。
func isValidSkillName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
		default:
			return false
		}
	}
	return true
}

// FormatLintFindings 把一条结果格式化为多行可读文本，供 cmd 层打印。
// 每行以 "  - [LEVEL] message" 形式输出。
func (r *LintResult) FormatLintFindings() []string {
	var lines []string
	for _, f := range r.Findings {
		lvl := "WARN"
		if f.Level == LintError {
			lvl = "ERROR"
		}
		lines = append(lines, "  - ["+lvl+"] "+f.Message)
	}
	return lines
}

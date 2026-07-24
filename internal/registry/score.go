// Package registry 的 score.go 对入库技能做启发式评分（0-100）。
//
// 动机：sm update 无脑 git pull，新版未必更好。评分给"更新是否改善"一个
// 客观信号。评分纯启发式，只看 SKILL.md 元数据与正文的可量化的表面质量，
// 不评判语义。仅 advisory，不影响任何命令的行为。
//
// 四维度加权：frontmatter 35 + 内容量 25 + 结构 25 + 可疑内容 15 = 100。
//
// Input: bytes, os, path/filepath, unicode/utf8
// Output: type SkillScore, func ScoreSkill
// Pos: 数据层-技能评分
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package registry

import (
	"bytes"
	"os"
	"path/filepath"
	"unicode/utf8"
)

// 维度满分。
const (
	scoreFrontmatterMax = 35
	scoreContentMax     = 25
	scoreStructureMax   = 25
	scoreSuspiciousMax  = 15
)

// 内容量甜区（字节）。
const (
	contentMinBytes = 200
	contentMaxBytes = 5000
	contentTooLong  = 20000 // 超此线衰减到下限
)

// 结构维度阈值。
const structHeadingsForMax = 3

// SkillScore 是一个技能的启发式评分结果。
type SkillScore struct {
	Total     int            `json:"total"`
	Breakdown map[string]int `json:"breakdown"` // 维度名 → 得分
	Notes     []string       `json:"notes,omitempty"`
}

// ScoreSkill 对 skillDir（注册表内相对路径，如 "global/my-skill"）评分。
// 纯读，无副作用。SKILL.md 缺失时返回全 0 分 + note。
func (r *Registry) ScoreSkill(skillDir string) *SkillScore {
	s := &SkillScore{Breakdown: make(map[string]int, 4)}
	abs := filepath.Join(r.skillsDir(), skillDir)
	skillMD := filepath.Join(abs, "SKILL.md")

	data, err := os.ReadFile(skillMD)
	if err != nil {
		s.Notes = append(s.Notes, "missing SKILL.md")
		return s
	}

	// ── frontmatter（35）：复用 LintSkill 的 findings，按级别扣分。
	lint := r.LintSkill(skillDir)
	fm := scoreFrontmatterMax
	for _, f := range lint.Findings {
		switch f.Level {
		case LintError:
			fm -= 18
		case LintWarning:
			fm -= 8
		}
	}
	fm = clamp(fm, 0, scoreFrontmatterMax)
	s.Breakdown["frontmatter"] = fm
	if fm < scoreFrontmatterMax {
		s.Notes = append(s.Notes, "frontmatter 有扣分项")
	}

	body := extractBody(data)

	// ── 内容量（25）：甜区 [200,5000] 满分，两侧衰减。
	s.Breakdown["content"] = scoreContent(len(body))

	// ── 结构（25）：## 标题数，≥3 满分。
	headings := countH2(body)
	s.Breakdown["structure"] = scoreStructure(headings)
	if headings == 0 {
		s.Notes = append(s.Notes, "无结构化标题")
	}

	// ── 可疑内容（15）：起始满分，命中扣分。
	susp, hits := scoreSuspicious(body)
	s.Breakdown["suspicious"] = susp
	s.Notes = append(s.Notes, hits...)

	for _, v := range s.Breakdown {
		s.Total += v
	}
	return s
}

// extractBody 从 SKILL.md 原始字节中切出 frontmatter 之后的正文。
// 无 frontmatter（无起始 ---）则返回全文。
func extractBody(data []byte) []byte {
	const delim = "---\n"
	start := bytes.Index(data, []byte(delim))
	if start < 0 {
		return data
	}
	rest := data[start+len(delim):]
	// 结束分隔符为 "\n---\n"；找不到则 frontmatter 未闭合，返回其后全部。
	endDelim := "\n" + delim
	end := bytes.Index(rest, []byte(endDelim))
	if end < 0 {
		return rest
	}
	return rest[end+len(endDelim):]
}

// scoreContent 把字节数映射到 [0, 25]。
//   - [200, 5000] 满分
//   - < 200 线性到 0
//   - (5000, 20000] 从满分线性衰减到 10
//   - > 20000 封顶 10
func scoreContent(n int) int {
	switch {
	case n < contentMinBytes:
		return scoreContentMax * n / contentMinBytes
	case n <= contentMaxBytes:
		return scoreContentMax
	case n <= contentTooLong:
		// 5000→25, 20000→10 线性
		return 25 - (25-10)*(n-contentMaxBytes)/(contentTooLong-contentMaxBytes)
	default:
		return 10
	}
}

// scoreStructure 按 H2 标题数评分。
func scoreStructure(headings int) int {
	if headings >= structHeadingsForMax {
		return scoreStructureMax
	}
	return scoreStructureMax * headings / structHeadingsForMax
}

// countH2 数行首起 "## "（非 "###"）的标题数。
// 采用零拷贝行扫描：在字节切片上用 bytes.IndexByte 切行，
// 避免 bytes.Split 的子切片数组分配，降低大 SKILL.md 评分时的 GC 压力。
func countH2(body []byte) int {
	count := 0
	for {
		nl := bytes.IndexByte(body, '\n')
		var line []byte
		if nl < 0 {
			line = body
			body = nil
		} else {
			line = body[:nl]
			body = body[nl+1:]
		}
		if bytes.HasPrefix(line, []byte("## ")) && !bytes.HasPrefix(line, []byte("###")) {
			count++
		}
		if nl < 0 {
			break
		}
	}
	return count
}

// suspiciousPatterns 是 prompt-injection 风格或低质量信号短语。
var suspiciousPatterns = []string{
	"ignore previous instructions",
	"ignore all previous",
	"disregard the above",
	"you are now",
	"system prompt:",
}

// scoreSuspicious 扫正文，起始满分，每命中一项扣 5（扣到 0）。
// 返回（得分, 命中说明切片）。
func scoreSuspicious(body []byte) (int, []string) {
	score := scoreSuspiciousMax
	var hits []string
	lower := bytes.ToLower(body)

	for _, p := range suspiciousPatterns {
		if bytes.Contains(lower, []byte(p)) {
			score -= 5
			hits = append(hits, "含可疑指令短语: "+p)
		}
	}

	// 二进制/无效 UTF-8 块。
	if !utf8.Valid(body) {
		score -= 5
		hits = append(hits, "含非 UTF-8 字节")
	}

	// 超长单行（> 2000 字符）往往是压缩文本或 minified 内容。
	// 用零拷贝行扫描，避免 bytes.Split 的分配。
	for remaining := body; len(remaining) > 0; {
		nl := bytes.IndexByte(remaining, '\n')
		var line []byte
		if nl < 0 {
			line = remaining
			remaining = nil
		} else {
			line = remaining[:nl]
			remaining = remaining[nl+1:]
		}
		if utf8.RuneCount(line) > 2000 {
			score -= 5
			hits = append(hits, "含超长单行（>2000 字符）")
			break
		}
	}

	return clamp(score, 0, scoreSuspiciousMax), hits
}

// clamp 限制 v 到 [lo, hi]。
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

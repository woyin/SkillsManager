// Package registry 的 frontmatter.go 负责解析 SKILL.md 文件头部的 YAML
// frontmatter（位于两行 "---" 之间），抽取 description 字段和
// metadata.internal 标志。
//
// 我们刻意采用极简的行扫描解析器（而非引入完整 YAML 库），以与
// npx skills 的实现保持一致，同时避免新依赖、保持冷启动开销最低。
//
// Input: bytes, os, strings
// Output: func ParseFrontmatterDescription, func ParseFrontmatterFromBytes, func ParseFrontmatterFromString
// Pos: 数据层-frontmatter解析
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package registry

import (
	"bytes"
	"os"
	"strings"
)

// skillFrontmatter 是从 SKILL.md frontmatter 中抽取出的字段集合。
// 目前关心三项：技能名、技能描述和是否为内部技能。
type skillFrontmatter struct {
	Name        string
	Description string
	Internal    bool
}

// parseSkillFrontmatter 读取一个 SKILL.md 文件并解析其 frontmatter。
// 任何 I/O 错误都会返回零值 skillFrontmatter（调用方通常在文件不存在
// 时直接忽略该技能）。
func parseSkillFrontmatter(skillMDPath string) skillFrontmatter {
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		return skillFrontmatter{}
	}
	return parseFrontmatterBytes(data)
}

// parseFrontmatterBytes 直接从原始字节解析 frontmatter。
// 导出此函数，供 cmd/find、cmd/add 等包共享同一实现，避免各自重复
// 一份解析器。
//
// 实现采用零拷贝的行扫描：在字节切片上用 bytes.IndexByte 寻找换行符，
// 切出每一行（仍是原 data 的子切片），再做前缀判断；这样既避免
// `string(data)` 的整段复制，也避免 strings.Split 产生的字符串数组
// 分配，显著降低了在高密度调用（如 DiscoverSkills）下的 GC 压力。
func parseFrontmatterBytes(data []byte) skillFrontmatter {
	var fm skillFrontmatter

	// YAML frontmatter 以开头的 "---" 行作为起始标记。
	if !bytes.HasPrefix(data, []byte("---")) {
		return fm
	}

	// 跳过起始的三个字符，然后在剩余内容里寻找闭合的 "\n---"。
	rest := data[3:]
	endIdx := bytes.Index(rest, []byte("\n---"))
	if endIdx < 0 {
		// 兜底：闭合标记可能恰好出现在 EOF 处，没有尾随换行。
		endIdx = bytes.Index(rest, []byte("---"))
		if endIdx < 0 {
			return fm
		}
	}
	frontmatter := rest[:endIdx]

	return parseFrontmatterLines(frontmatter)
}

// parseFrontmatterLines 逐行扫描 frontmatter 区域，解析 name、description
// 与 metadata 块内的 internal 标志。
func parseFrontmatterLines(frontmatter []byte) skillFrontmatter {
	var fm skillFrontmatter
	inMetadata := false
	for {
		// 找到下一个换行符，切出当前行。
		nl := bytes.IndexByte(frontmatter, '\n')
		var line []byte
		if nl < 0 {
			line = frontmatter
			frontmatter = nil
		} else {
			line = frontmatter[:nl]
			frontmatter = frontmatter[nl+1:]
		}
		if len(line) == 0 && nl < 0 {
			break
		}

		inMetadata = parseFrontmatterLine(&fm, line, inMetadata)
		if nl < 0 {
			break
		}
	}
	return fm
}

// parseFrontmatterLine 解析一行 frontmatter，返回解析后是否仍处于
// metadata 块内。
func parseFrontmatterLine(fm *skillFrontmatter, line []byte, inMetadata bool) bool {
	trimmed := bytes.TrimSpace(line)

	// 进入 metadata: 块。块内的 internal: 依靠缩进识别，而非前缀匹配，
	// 因此顶层（未缩进）的 internal: 不会被误判为内部标志。
	if bytes.Equal(trimmed, []byte("metadata:")) || bytes.HasPrefix(trimmed, []byte("metadata:")) {
		return true
	}
	// 任何无缩进的顶层 key 都标志着 metadata 块结束。
	if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
		inMetadata = false
	}

	// 解析 name: 行。
	if bytes.HasPrefix(trimmed, []byte("name:")) {
		fm.Name = trimFrontmatterValue(string(trimmed[len("name:"):]))
	}

	// 解析 description: 行。
	if bytes.HasPrefix(trimmed, []byte("description:")) {
		// ponytail: 对值做一次 trim，足以覆盖现有 SKILL.md 的写法；
		// 若未来需要支持多行折叠描述，可在此扩展。
		fm.Description = trimFrontmatterValue(string(trimmed[len("description:"):]))
	}

	// 在 metadata 块内解析 internal: 标志。
	if inMetadata && bytes.HasPrefix(trimmed, []byte("internal:")) {
		val := strings.TrimSpace(string(trimmed[len("internal:"):]))
		switch val {
		case "true", "True", "TRUE", "yes", "1":
			fm.Internal = true
		}
	}
	return inMetadata
}

func trimFrontmatterValue(value string) string {
	value = strings.TrimSpace(value)
	// 去掉两侧成对的引号（单引号或双引号）。
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
		value = value[1 : len(value)-1]
	}
	return value
}

// ParseFrontmatterDescription 读取一个 SKILL.md 文件并返回其 frontmatter
// 中的 description 字符串。导出给 cmd 包使用，替代过去重复实现的
// 解析逻辑。
func ParseFrontmatterDescription(skillMDPath string) string {
	return parseSkillFrontmatter(skillMDPath).Description
}

// ParseFrontmatterFromBytes 从原始字节中抽取 description。
// 导出，使 cmd/find 能共享同一份解析器实现。
func ParseFrontmatterFromBytes(data []byte) string {
	return parseFrontmatterBytes(data).Description
}

// internalSkillsVisible 判断标记为 metadata.internal 的技能是否应当
// 显示。沿用 npx skills 的约定：仅当环境变量 INSTALL_INTERNAL_SKILLS
// 被设为真值时才可见。
func internalSkillsVisible() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("INSTALL_INTERNAL_SKILLS")))
	switch v {
	case "1", "true", "yes":
		return true
	}
	return false
}

// ParseFrontmatterFromString 从 SKILL.md 的字符串内容中抽取 description。
// 导出，使 cmd/find 能直接复用解析器，而无需一次多余的
// string→[]byte→string 往返转换。
func ParseFrontmatterFromString(content string) string {
	return parseFrontmatterBytes([]byte(content)).Description
}

// SanitizeMetadata strips terminal escape sequences and control characters
// from a metadata string (skill name, description), preventing ANSI injection
// when displaying untrusted frontmatter values. Newlines are collapsed to
// spaces. Aligned with npx skills' sanitizeMetadata.
func SanitizeMetadata(s string) string {
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == 0x1b: // ESC — skip entire escape sequence
			i = skipEscapeSequence(runes, i)
		case r == '\n' || r == '\r':
			if b.Len() > 0 && b.String()[b.Len()-1] != ' ' {
				b.WriteByte(' ')
			}
		case r < 0x20 && r != '\t': // control chars (except tab)
			continue
		case r == 0x7f: // DEL
			continue
		case r >= 0x80 && r <= 0x9f: // C1 control codes
			continue
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// skipEscapeSequence 跳过从 runes[i]（应为 ESC）开始的整个转义序列，
// 返回序列内最后一个待跳过字符的下标（调用方的 i++ 会越过它）：
//   - CSI（ESC [ …）：消费中间字节直到终结字节（0x40-0x7e）；
//   - OSC（ESC ] …）：消费直到 BEL（0x07）或 ST（ESC \\）。
func skipEscapeSequence(runes []rune, i int) int {
	i++ // consume char after ESC
	if i >= len(runes) {
		return i
	}
	switch runes[i] {
	case '[':
		i++
		for i < len(runes) && !(runes[i] >= 0x40 && runes[i] <= 0x7e) {
			i++
		}
	case ']':
		i++
		for i < len(runes) && runes[i] != 0x07 {
			if runes[i] == 0x1b && i+1 < len(runes) && runes[i+1] == '\\' {
				i++
				break
			}
			i++
		}
	}
	return i
}

// Package registry 的 source.go 负责解析与规范化"技能来源"字符串。
//
// 来源可以是以下任一形式：
//   - GitHub 简写： owner/repo 或 owner/repo/tree/<branch>/<path>
//   - 完整 HTTPS URL： https://github.com/...、gitlab.com、bitbucket.org
//   - SSH URL： git@github.com:owner/repo.git
//   - 本地路径： ./xxx、/abs/path
//
// 本文件提供三类函数：
//   - SkillNameFromPath：从来源推断技能名；
//   - IsGitURL：判断来源是否为 git 形式（决定克隆还是拷贝）；
//   - NormalizeGitURL / ParseTreeURL：把简写规范化为可克隆 URL，
//     必要时拆出 branch 与子路径。
//
// Input: strings
// Output: func SkillNameFromPath, func IsGitURL, func NormalizeGitURL, func ParseTreeURL
// Pos: 数据层-来源解析
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package registry

import (
	"net/url"
	"strings"
)

// SkillNameFromPath 从路径或 URL 抽取技能名。
// 对 tree URL，取 tree 子路径的最后一段；否则取整体最后一段，
// 并去掉 .git 后缀。
func SkillNameFromPath(source string) string {
	source = strings.TrimRight(source, "/")
	// tree URL：技能名为 /tree/<...> 的最后一段。
	if idx := strings.Index(source, "/tree/"); idx >= 0 {
		treePath := source[idx+6:] // 跳过 "/tree/"
		parts := strings.Split(treePath, "/")
		if len(parts) > 0 {
			name := parts[len(parts)-1]
			name = strings.TrimSuffix(name, ".git")
			if name != "" {
				return name
			}
		}
		// 兜底：回退到仓库段。
		source = source[:idx]
	}
	parts := strings.Split(source, "/")
	if len(parts) > 0 {
		name := parts[len(parts)-1]
		name = strings.TrimSuffix(name, ".git")
		return name
	}
	return source
}

// IsGitURL 判断 source 是否为 git 形式来源。
// 命中 SSH 前缀、.git 后缀、已知 HTTPS 主机，或 GitHub 简写即视为 git。
func IsGitURL(source string) bool {
	if strings.Contains(source, "::") && !strings.HasPrefix(source, "git@") {
		return false
	}
	// SSH URL。
	if strings.HasPrefix(source, "git@") {
		return true
	}
	// .git 后缀。
	if strings.HasSuffix(source, ".git") {
		return true
	}
	// 已知 HTTPS 主机。
	for _, prefix := range []string{
		"https://github.com/",
		"https://gitlab.com/",
		"https://bitbucket.org/",
		"http://github.com/",
		"http://gitlab.com/",
		"http://bitbucket.org/",
	} {
		if strings.HasPrefix(source, prefix) {
			return true
		}
	}
	// GitHub 简写（owner/repo 等）。
	if isGitHubShorthand(source) {
		return true
	}
	return false
}

// isGitHubShorthand 判断是否为 owner/repo 形式的 GitHub 简写。
// 排除本地路径（以 . 或 / 开头）和含冒号的 SSH URL。
func isGitHubShorthand(source string) bool {
	// 排除本地路径。
	if strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") {
		return false
	}
	// 排除含冒号的 URL（SSH 等）。
	if strings.Contains(source, ":") {
		return false
	}
	// owner/repo 或 owner/repo/tree/path：2~4 段。
	parts := strings.Split(source, "/")
	if len(parts) < 2 || len(parts) > 4 {
		return false
	}
	// 前两段（owner、repo）不得为空。
	for _, p := range parts[:2] {
		if p == "" {
			return false
		}
	}
	return true
}

// NormalizeGitURL 把简写规范化为可直接克隆的 URL。
//   - github.com/... → https://github.com/...
//   - owner/repo     → https://github.com/owner/repo（剥离 /tree/...）
//   - 其它（已是完整 URL）原样返回。
func NormalizeGitURL(source string) string {
	// 拒绝 git 协议扩展（ext:: 等），防止命令注入。
	if strings.Contains(source, "::") && !strings.HasPrefix(source, "git@") {
		return source
	}
	if strings.HasPrefix(source, "github.com/") {
		return "https://" + source
	}
	if isGitHubShorthand(source) {
		// 先剥离 /tree/ 子路径。
		base := source
		if idx := strings.Index(source, "/tree/"); idx >= 0 {
			base = source[:idx]
		}
		return "https://github.com/" + base
	}
	return source
}

// ParseTreeURL 拆解 tree URL，返回（仓库 URL、分支、子路径、是否匹配）。
// 例： https://github.com/owner/repo/tree/main/skills/my-skill
//
//	→ ("https://github.com/owner/repo", "main", "skills/my-skill", true)
//
// 非 tree URL（无 /tree/）返回仓库根 URL 与空分支/子路径。
func ParseTreeURL(source string) (repoURL, branch, subPath string, ok bool) {
	// 处理 GitHub 简写 + tree：owner/repo/tree/branch/path
	if !strings.Contains(source, "://") && !strings.HasPrefix(source, "git@") {
		if strings.Contains(source, "/tree/") {
			parts := strings.SplitN(source, "/", 3)
			if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
				source = "https://github.com/" + source
			}
		}
	}

	for _, host := range []string{"https://github.com/", "https://gitlab.com/", "https://bitbucket.org/"} {
		if strings.HasPrefix(source, host) {
			rest := source[len(host):]
			parts := strings.SplitN(rest, "/", 3) // owner, repo, tree/...
			if len(parts) < 2 {
				return "", "", "", false
			}
			repoURL = host + parts[0] + "/" + parts[1]
			// 剥离 repo 名上的 .git。
			repoURL = strings.TrimSuffix(repoURL, ".git")

			if len(parts) < 3 {
				return repoURL, "", "", true
			}

			treePath := parts[2]
			if !strings.HasPrefix(treePath, "tree/") {
				return repoURL, "", "", true
			}

			treePath = treePath[5:] // 剥离 "tree/"
			branchAndPath := strings.SplitN(treePath, "/", 2)
			branch = branchAndPath[0]
			if len(branchAndPath) > 1 {
				subPath = branchAndPath[1]
			}
			return repoURL, branch, subPath, true
		}
	}
	return "", "", "", false
}

// SanitizeSubpath validates a subpath for path traversal, returning the
// cleaned subpath or empty string if it contains ".." segments. This prevents
// malicious tree URLs like owner/repo/tree/main/../../etc from escaping the
// clone root.
func SanitizeSubpath(subPath string) string {
	if subPath == "" {
		return ""
	}
	normalized := strings.ReplaceAll(subPath, "\\", "/")
	for _, segment := range strings.Split(normalized, "/") {
		if segment == ".." {
			return ""
		}
	}
	return subPath
}

// ParsedSource is the structured decomposition of a source string.
// It captures the cloneable repo URL, an optional branch/tag/commit ref,
// an optional subpath within the repo, and an optional skill name filter.
type ParsedSource struct {
	URL         string // normalized, cloneable git URL
	Ref         string // branch, tag, or commit hash
	SubPath     string // path within the repo (e.g. skills/my-skill)
	SkillFilter string // skill name to select (from @skill syntax)
	IsLocal     bool   // true if source is a local path
	LocalPath   string // resolved local path (when IsLocal)
}

// ParseSource decomposes a source string into its structured form.
// Supported formats (aligned with npx skills):
//   - owner/repo                      → github shorthand
//   - owner/repo@skill                → shorthand + skill filter
//   - owner/repo#branch               → shorthand + branch ref
//   - owner/repo#branch@skill         → shorthand + branch + skill filter
//   - github:owner/repo               → explicit github prefix
//   - gitlab:owner/repo               → explicit gitlab prefix
//   - github.com/owner/repo/tree/b/p  → tree URL with branch + subpath
//   - full HTTPS / SSH / .git URLs
//   - ./local/path or /abs/path       → local source
func ParseSource(input string) ParsedSource {
	// Local path check.
	if isLocalPath(input) {
		return ParsedSource{IsLocal: true, LocalPath: input}
	}

	// Strip and capture fragment (#branch or #branch@skill or #skill).
	ref, skillFilter := "", ""
	if hashIdx := strings.Index(input, "#"); hashIdx >= 0 {
		beforeHash := input[:hashIdx]
		// Only interpret the fragment as a git ref/skill filter when the
		// pre-hash portion looks like a git source (mirrors npx skills
		// looksLikeGitSource gate). Otherwise the '#' is literal.
		if looksLikeGitSource(beforeHash) {
			fragment := input[hashIdx+1:]
			input = beforeHash
			if atIdx := strings.Index(fragment, "@"); atIdx >= 0 {
				ref = decodeFragmentValue(fragment[:atIdx])
				skillFilter = decodeFragmentValue(fragment[atIdx+1:])
			} else {
				ref = decodeFragmentValue(fragment)
			}
		}
	}

	// @skill filter (without #): owner/repo@skill.
	if atIdx := strings.Index(input, "@"); atIdx >= 0 && !strings.HasPrefix(input, "git@") {
		// Ensure this isn't an SSH URL (git@host:...).
		if !strings.Contains(input[:atIdx], "://") {
			skillFilter = input[atIdx+1:]
			input = input[:atIdx]
		}
	}

	// Prefix shortcuts.
	if rest, ok := strings.CutPrefix(input, "github:"); ok && rest != "" {
		input = rest
	}
	if rest, ok := strings.CutPrefix(input, "gitlab:"); ok && rest != "" {
		input = "https://gitlab.com/" + rest
	}

	// Now resolve input to URL + optional ref + subpath via ParseTreeURL.
	repoURL, treeBranch, subPath, ok := ParseTreeURL(input)
	if ok {
		// Tree URL branch takes precedence unless fragment ref overrides.
		effectiveRef := treeBranch
		if effectiveRef == "" {
			effectiveRef = ref
		}
		return ParsedSource{
			URL:         repoURL,
			Ref:         effectiveRef,
			SubPath:     subPath,
			SkillFilter: skillFilter,
		}
	}

	// Fallback: normalize as a plain git URL.
	return ParsedSource{
		URL:         NormalizeGitURL(input),
		Ref:         ref,
		SkillFilter: skillFilter,
	}
}

// Source reconstructs a source string (without the @skill filter) suitable
// for passing to CloneToTemp or AddSkillWithOptions. For git sources this is
// a tree URL encoding the ref and subpath; for local sources, the path.
func (p ParsedSource) Source() string {
	if p.IsLocal {
		return p.LocalPath
	}
	s := p.URL
	if p.Ref != "" {
		s += "/tree/" + p.Ref
		if p.SubPath != "" {
			s += "/" + p.SubPath
		}
	} else if p.SubPath != "" {
		// No branch but a subpath — can't encode without a ref, use bare URL.
		s = p.URL
	}
	return s
}

// isLocalPath reports whether source is a local filesystem path.
func isLocalPath(source string) bool {
	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") || strings.HasPrefix(source, "/") {
		return true
	}
	if strings.HasPrefix(source, "~") {
		return true
	}
	return false
}

// decodeFragmentValue URL-decodes a #fragment ref or skill filter value,
// so percent-encoded characters (e.g. %2F in a branch name like
// "feat/thing" encoded as "feat%2Fthing") resolve correctly. Mirrors npx
// skills decodeFragmentValue. Falls back to the raw value on decode error.
func decodeFragmentValue(v string) string {
	if decoded, err := url.QueryUnescape(v); err == nil {
		return decoded
	}
	return v
}

// looksLikeGitSource reports whether input resembles a git source, used to
// gate #fragment parsing (a '#' in a non-git string is literal). Mirrors npx
// skills looksLikeGitSource: prefix shortcuts, SSH, HTTP(S) git hosts, .git
// suffix, or owner/repo shorthand.
func looksLikeGitSource(input string) bool {
	if strings.HasPrefix(input, "github:") || strings.HasPrefix(input, "gitlab:") || strings.HasPrefix(input, "git@") {
		return true
	}
	if strings.HasPrefix(input, "ssh://") && strings.HasSuffix(input, ".git") {
		return true
	}
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		if parsed, err := url.Parse(input); err == nil && parsed.Host != "" {
			return true
		}
		return strings.HasSuffix(input, ".git")
	}
	// owner/repo shorthand: no colon, not a local path, matches the
	// owner/repo[/...][@...] shape.
	if strings.Contains(input, ":") || strings.HasPrefix(input, ".") || strings.HasPrefix(input, "/") {
		return false
	}
	parts := strings.Split(input, "/")
	return len(parts) >= 2 && parts[0] != "" && parts[1] != ""
}

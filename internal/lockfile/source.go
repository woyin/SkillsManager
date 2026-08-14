// Package lockfile 的 source.go 提供来源元数据推断与来源别名。
//
// 从来源字符串推断 sourceType（github/git/local）与可克隆的 sourceURL，
// 并对已知的来源简写做别名归一化（对齐 npx skills 的 SOURCE_ALIASES）。
//
// Input: strings, github.com/woyin/skills-manager/internal/registry
// Output: type SourceMeta, func ResolveAlias, func ClassifySource
// Pos: 业务层-来源元数据推断
package lockfile

import (
	"strings"

	"github.com/woyin/skills-manager/internal/sourceutil"
)

// sourceAliases 是已知的来源简写别名映射（对齐 npx skills 的 SOURCE_ALIASES）。
var sourceAliases = map[string]string{
	"coinbase/agentWallet":      "coinbase/agentic-wallet-skills",
	"vercel-labs/vercel-skills": "vercel-labs/agent-skills",
}

// ResolveAlias 把已知别名的来源归一化为规范来源。
// 非别名的来源原样返回。
func ResolveAlias(source string) string {
	if alias, ok := sourceAliases[source]; ok {
		return alias
	}
	return source
}

// SourceMeta 描述来源的类型与可克隆 URL。
type SourceMeta struct {
	SourceType string // "github"、"git"、"local"
	SourceURL  string // 可克隆的完整 URL（local 为空）
}

// ClassifySource 从来源字符串推断 SourceMeta。
//   - GitHub 简写（owner/repo）→ sourceType "github"，sourceURL 补全为 https://
//   - 其它 git URL（SSH、.git、已知 HTTPS 主机）→ sourceType "git"
//   - 本地路径 → sourceType "local"
func ClassifySource(source string) SourceMeta {
	// Tree URLs with /tree/ are git sources even if IsGitURL misses deep paths.
	if meta, ok := classifyTreeURL(source); ok {
		return meta
	}

	if !sourceutil.IsGitURL(source) {
		return SourceMeta{SourceType: "local"}
	}

	// GitHub 简写：owner/repo 形式 → github 类型。
	if isGitHubShorthand(source) {
		return SourceMeta{
			SourceType: "github",
			SourceURL:  "https://github.com/" + shorthandBase(source),
		}
	}

	// 其它 git URL（SSH、完整 HTTPS、.git）。
	return SourceMeta{
		SourceType: "git",
		SourceURL:  sourceutil.NormalizeGitURL(source),
	}
}

// classifyTreeURL 处理带 /tree/ 的树 URL：简写形式归为 github，
// 完整形式归为 git。非树 URL 返回 ok=false。
func classifyTreeURL(source string) (SourceMeta, bool) {
	if !strings.Contains(source, "/tree/") || strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") {
		return SourceMeta{}, false
	}
	repoURL, _, _, isTree := sourceutil.ParseTreeURL(source)
	if !isTree {
		return SourceMeta{}, false
	}
	if isShorthandURL(source) {
		base := source[:strings.Index(source, "/tree/")]
		return SourceMeta{SourceType: "github", SourceURL: "https://github.com/" + base}, true
	}
	return SourceMeta{SourceType: "git", SourceURL: repoURL}, true
}

// isShorthandURL 判断 source 是否为 owner/repo 简写（非完整 URL/SSH）。
func isShorthandURL(source string) bool {
	return !strings.Contains(source, "://") && !strings.HasPrefix(source, "git@")
}

// isGitHubShorthand 判断 source 是否为指向 GitHub 的 owner/repo 简写。
func isGitHubShorthand(source string) bool {
	if !isShorthandURL(source) {
		return false
	}
	normalized := sourceutil.NormalizeGitURL(source)
	return strings.HasPrefix(normalized, "https://github.com/") && !strings.HasPrefix(source, "https://")
}

// shorthandBase 返回简写形式的 base（去掉 /tree/ 后缀部分）。
func shorthandBase(source string) string {
	_, _, _, isTree := sourceutil.ParseTreeURL(source)
	base := source
	if isTree {
		base = source[:strings.Index(source, "/tree/")]
	}
	return base
}

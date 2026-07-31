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
	if strings.Contains(source, "/tree/") && !strings.HasPrefix(source, ".") && !strings.HasPrefix(source, "/") {
		isShorthand := !strings.Contains(source, "://") && !strings.HasPrefix(source, "git@")
		repoURL, _, _, isTree := sourceutil.ParseTreeURL(source)
		if isTree {
			if isShorthand {
				base := source[:strings.Index(source, "/tree/")]
				return SourceMeta{
					SourceType: "github",
					SourceURL:  "https://github.com/" + base,
				}
			}
			return SourceMeta{
				SourceType: "git",
				SourceURL:  repoURL,
			}
		}
	}

	if !sourceutil.IsGitURL(source) {
		return SourceMeta{SourceType: "local"}
	}

	// GitHub 简写：owner/repo 形式
	isShorthand := false
	if !strings.Contains(source, "://") && !strings.HasPrefix(source, "git@") {
		normalized := sourceutil.NormalizeGitURL(source)
		if strings.HasPrefix(normalized, "https://github.com/") && !strings.HasPrefix(source, "https://") {
			isShorthand = true
		}
	}

	if isShorthand {
		_, _, _, isTree := sourceutil.ParseTreeURL(source)
		base := source
		if isTree {
			base = source[:strings.Index(source, "/tree/")]
		}
		return SourceMeta{
			SourceType: "github",
			SourceURL:  "https://github.com/" + base,
		}
	}

	// 其它 git URL（SSH、完整 HTTPS、.git）
	return SourceMeta{
		SourceType: "git",
		SourceURL:  sourceutil.NormalizeGitURL(source),
	}
}

// Package registry 的 refkind.go 把"用户请求的 ref 字符串"解析为 RefKind
// （default-branch / branch / tag / commit），控制 update 行为（ADR 0014）。
//
// 解析规则：
//   - ref 为空 → default-branch（跟踪远端默认分支）；
//   - 以 refs/heads/ 开头 → branch；refs/tags/ → tag（显式限定）；
//   - 形如完整 commit hash（40 位十六进制）→ commit；
//   - 否则在 clonedRepo 中查 git：是 branch → branch；是 tag → tag；
//     都不是但能 checkout → commit；
//   - branch 与 tag 同名（未限定前缀）→ 拒绝，要求 refs/heads/ 或 refs/tags/。
//
// clonedRepo 为空时，无法查 git，只能按字面规则推断（commit hash 或限定前缀）；
// 非限定且非 hash 的 ref 返回 RefUnknown（调用方可按 pinned 处理）。
//
// Input: fmt, os/exec, strings
// Output: func ResolveRefKind, func IsCommitHash
// Pos: 数据层-git ref类型解析
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package registry

import (
	"fmt"
	"os/exec"
	"strings"
)

// ResolveRefKind 把请求的 ref 字符串解析为 RefKind。
// clonedRepo 非空时查询该 git 仓库以区分 branch/tag/commit。
//
//   - "" → RefDefaultBranch
//   - "refs/heads/x" → RefBranch（x）
//   - "refs/tags/x" → RefTag（x）
//   - 完整 commit hash → RefCommit
//   - clonedRepo 内查询：是 branch → RefBranch；是 tag → RefTag；
//     branch 与 tag 同名 → 错误（要求限定前缀）；否则 → RefCommit。
//
// 返回规范化的 ref（去掉 refs/heads/ refs/tags/ 前缀）与 RefKind。
func ResolveRefKind(ref, clonedRepo string) (normalized string, kind RefKind, err error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", RefDefaultBranch, nil
	}
	// 显式限定前缀。
	if strings.HasPrefix(ref, "refs/heads/") {
		return strings.TrimPrefix(ref, "refs/heads/"), RefBranch, nil
	}
	if strings.HasPrefix(ref, "refs/tags/") {
		return strings.TrimPrefix(ref, "refs/tags/"), RefTag, nil
	}
	// 完整 commit hash。
	if IsCommitHash(ref) {
		return ref, RefCommit, nil
	}
	// 无法查询 git：保守返回 unknown（调用方按 pinned 处理，不前进）。
	if clonedRepo == "" {
		return ref, RefUnknown, nil
	}
	// 查询仓库。
	isBranch, isTag, qerr := queryRefType(clonedRepo, ref)
	if qerr != nil {
		return ref, RefUnknown, qerr
	}
	if isBranch && isTag {
		return ref, RefUnknown, fmt.Errorf("ambiguous ref %q: exists as both branch and tag; qualify with refs/heads/%s or refs/tags/%s", ref, ref, ref)
	}
	if isBranch {
		return ref, RefBranch, nil
	}
	if isTag {
		return ref, RefTag, nil
	}
	// 既非 branch 也非 tag：当作 commit。
	return ref, RefCommit, nil
}

// IsCommitHash 报告 s 是否形如完整 Git commit hash（40 位十六进制）。
// 短 hash 不在此判定（无法可靠区分短 hash 与短 branch/tag 名）。
func IsCommitHash(s string) bool {
	if len(s) != 40 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// queryRefType 查询 clonedRepo：ref 是否同时是 branch 和/或 tag。
// 用 git rev-parse --verify refs/heads/<ref> 与 refs/tags/<ref>。
func queryRefType(clonedRepo, ref string) (isBranch, isTag bool, err error) {
	check := func(fullRef string) bool {
		out, err := exec.Command("git", "-C", clonedRepo, "rev-parse", "--verify", "--quiet", fullRef).CombinedOutput()
		if err != nil {
			return false
		}
		return len(strings.TrimSpace(string(out))) > 0
	}
	isBranch = check("refs/heads/" + ref)
	isTag = check("refs/tags/" + ref)
	return isBranch, isTag, nil
}

// Package registry 的 clone.go 把"克隆到临时目录"的样板代码集中起来。
//
// cmd/add 与 cmd/use 此前各自重复实现"创建临时目录 → 克隆 → 返回路径"
// 的流程；CloneToTemp 把它抽成一处，配合 RemoveCloneTemp 做清理。
//
// Input: fmt, os, path/filepath
// Output: func CloneToTemp, func RemoveCloneTemp
// Pos: 数据层-仓库克隆
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package registry

import (
	"fmt"
	"os"
	"path/filepath"
)

// CloneToTemp 把 source 克隆到一个新建临时目录，返回：
//   - repoDir：克隆产物路径（临时目录下的 "repo" 子目录）；
//   - tempDir：临时目录本身（用于后续清理）；
//   - err：失败时已自动清理临时目录。
//
// source 可以是 ParseTreeURL / NormalizeGitURL 接受的任何形式。
// 非 git 来源返回错误。
func CloneToTemp(source, tempPrefix string) (repoDir, tempDir string, err error) {
	if !IsGitURL(source) {
		return "", "", fmt.Errorf("not a git source: %s", source)
	}

	repoURL, branch, _, ok := ParseTreeURL(source)
	if !ok {
		repoURL = NormalizeGitURL(source)
	}

	tempDir, err = os.MkdirTemp("", tempPrefix)
	if err != nil {
		return "", "", fmt.Errorf("creating temp dir: %w", err)
	}

	repoDir = filepath.Join(tempDir, "repo")
	if branch != "" {
		err = CloneRepoWithBranch(repoURL, branch, repoDir)
	} else {
		err = CloneRepoShallow(repoURL, repoDir)
	}
	if err != nil {
		os.RemoveAll(tempDir)
		return "", "", fmt.Errorf("cloning %s: %w", repoURL, err)
	}
	return repoDir, tempDir, nil
}

// RemoveCloneTemp 删除 CloneToTemp 创建的临时目录。
// 传入空串时为 no-op，因此可安全地在 defer 中调用（即便克隆失败）。
func RemoveCloneTemp(tempDir string) {
	if tempDir != "" {
		os.RemoveAll(tempDir)
	}
}

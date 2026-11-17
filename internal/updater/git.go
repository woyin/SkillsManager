// git.go 提供更新流程需要的 git 判定，可独立测试。
//
// Input: os/exec, strings
// Output: func IsDetached, func IsDirty, func HeadHash
// Pos: 业务层-更新用 git 判定
package updater

import (
	"os/exec"
	"strings"
)

// IsDetached 报告仓库是否处于 detached HEAD（被 update 视为 pinned，跳过）。
// 利用 git symbolic-ref -q HEAD：附着分支时退出 0，detached 时退出 1，
// 其它错误才视为真实失败。
func IsDetached(repoPath string) (bool, error) {
	err := exec.Command("git", "-C", repoPath, "symbolic-ref", "-q", "HEAD").Run()
	if err == nil {
		return false, nil
	}
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		return true, nil
	}
	return false, err
}

// IsDirty 报告仓库工作区是否有本地改动。
func IsDirty(repoPath string) (bool, error) {
	out, err := exec.Command("git", "-C", repoPath, "status", "--porcelain").Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// HeadHash 返回 HEAD commit；失败时返回空串（调用方按“未知”处理）。
func HeadHash(repoPath string) string {
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

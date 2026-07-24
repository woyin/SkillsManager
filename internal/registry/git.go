// Package registry 的 git.go 封装 git 克隆相关的辅助函数。
//
// 所有克隆函数都先检查目标是否已存在（已存在则报错），再创建父目录，
// 最后调用系统 git 子进程。stdout/stderr 直接透传到调用进程，
// 以便用户实时看到 git 输出。
//
// Input: fmt, os, os/exec, path/filepath, github.com/woyin/skills-manager/internal/fsutil
// Output: func CloneRepo, func CloneRepoWithBranch
// Pos: 数据层-git操作
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package registry

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/woyin/skills-manager/internal/fsutil"
)

// CloneRepo 把 url 完整克隆（不限制深度）到 dest。
// dest 已存在则返回错误。导出以供 cmd 包共享，避免重复 exec 模板。
func CloneRepo(url, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("destination already exists: %s", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	cmd := exec.Command("git", "clone", url, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CloneRepoWithBranch 克隆 url 的指定分支（--depth 1 浅克隆）到 dest。
// branch 为空时不带 --branch 参数（克隆默认分支）。导出供 cmd 包共享。
func CloneRepoWithBranch(url, branch, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("destination already exists: %s", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	args := []string{"clone"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, "--depth", "1", url, dest)
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// copyDir 通过共享的 fsutil 助手拷贝目录，并强制"dest 不存在"契约。
func (r *Registry) copyDir(src, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("destination already exists: %s", dest)
	}
	return fsutil.CopyDir(src, dest)
}

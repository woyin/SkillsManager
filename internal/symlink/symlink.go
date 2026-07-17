// Package symlink 提供符号链接相关的辅助函数：创建、验证、查找、清理。
//
// sm 的核心设计是"注册表存原件，各代理目录放符号链接"，因此本包是
// 安装、检查、卸载等命令的基础设施。
package symlink

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Create 在 dst 处创建指向 src 的符号链接，必要时创建父目录。
// 若链接已存在且目标正确，直接返回；目标不同则报错（避免误覆盖）。
func Create(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("creating parent dir: %w", err)
	}

	// 已存在且目标正确：跳过。
	if target, err := os.Readlink(dst); err == nil {
		if target == src {
			return nil
		}
		return fmt.Errorf("symlink %s already exists pointing to %s (want %s)", dst, target, src)
	}

	return os.Symlink(src, dst)
}

// IsSymlink 判断 path 是否为符号链接。
func IsSymlink(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeSymlink != 0
}

// Verify 判断符号链接存在且其指向的目标也存在。
func Verify(path string) bool {
	if !IsSymlink(path) {
		return false
	}
	target, err := os.Readlink(path)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	_, err = os.Stat(target)
	return err == nil
}

// RemoveIfBroken 移除一个失效的符号链接；返回是否真正移除。
func RemoveIfBroken(path string) (bool, error) {
	if !IsSymlink(path) {
		return false, nil
	}
	if Verify(path) {
		return false, nil
	}
	return true, os.Remove(path)
}

// FindPointingTo 在 searchDir 中查找所有指向 target 的符号链接。
// target 与链接目标都会先做 EvalSymlinks 规整，确保可比性。
func FindPointingTo(searchDir, target string) ([]string, error) {
	var results []string
	absTarget, err := comparablePath(target)
	if err != nil {
		return nil, err
	}

	err = filepath.WalkDir(searchDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // 跳过错误条目
		}
		// WalkDir 对目录本身也会回调，且 DirEntry 不跟随符号链接；
		// 用 Type() 检测符号链接位。
		if d.Type()&os.ModeSymlink == 0 {
			return nil
		}
		linkTarget, err := os.Readlink(path)
		if err != nil {
			return nil
		}
		if !filepath.IsAbs(linkTarget) {
			linkTarget = filepath.Join(filepath.Dir(path), linkTarget)
		}
		linkTarget, err = comparablePath(linkTarget)
		if err != nil {
			return nil
		}
		if linkTarget == absTarget {
			results = append(results, path)
		}
		return nil
	})

	return results, err
}

// comparablePath 返回 path 的规整绝对路径（尽量 EvalSymlinks）。
// 即便 EvalSymlinks 失败也尽量返回可用结果，而非报错。
func comparablePath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		return resolved, nil
	}
	// 父目录可能可解析；只解析父目录再拼回文件名。
	parent, name := filepath.Split(absPath)
	if parent == "" || parent == absPath {
		return absPath, nil
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Clean(parent))
	if err != nil {
		return absPath, nil
	}
	return filepath.Join(resolvedParent, name), nil
}

// RemoveAll 移除一个符号链接或目录（等价于 os.RemoveAll）。
func RemoveAll(path string) error {
	return os.RemoveAll(path)
}

// PointInside 判断 linkPath 指向的目标是否落在 root 之内。
// 用于识别"由 sm 安装的"符号链接（其目标在注册表内）。
func PointInside(linkPath, root string) bool {
	target, err := os.Readlink(linkPath)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(linkPath), target)
	}
	// 若 target 已是绝对路径，无需再调 filepath.Abs。
	absTarget := target
	if !filepath.IsAbs(absTarget) {
		absTarget, err = filepath.Abs(target)
		if err != nil {
			return false
		}
	}
	absRoot := root
	if !filepath.IsAbs(absRoot) {
		absRoot, err = filepath.Abs(root)
		if err != nil {
			return false
		}
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	// rel 既不以 ".." 开头、也不等于 ".."，则目标在 root 内。
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

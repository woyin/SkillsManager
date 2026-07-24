// Package fsutil 提供跨 sm 各包共享的文件系统小工具。
//
// 中心化目录拷贝逻辑（此前在 internal/registry 与 cmd/add 中各自重复）。
// 设计目标：
//   - 流式拷贝（io.Copy），避免一次性把文件读入内存；
//   - 跟随符号链接，拷贝其指向的真实内容；
//   - 保留文件权限位；
//   - 跳过版本控制（.git）与依赖（node_modules）目录。
//
// Input: fmt, io, os, path/filepath
// Output: func CopyDir
// Pos: 工具层-文件系统工具
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package fsutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// skipDirs 是永远不拷贝的目录条目：版本控制元数据与依赖树，
// 拷贝它们既浪费空间又无还原价值。
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
}

// CopyDir 把 src 为根的整棵目录树拷贝到 dest。
//
// 行为约定：
//   - dest 必须不存在（沿用注册表的契约，调用方据此识别"已安装"）；
//   - 若 src 是符号链接，先解析再拷贝其目标树；
//   - 文件权限位被保留；
//   - 内容以流式方式拷贝，不在内存中缓冲整个文件。
func CopyDir(src, dest string) error {
	srcInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}

	// 若 src 是符号链接，解析后拷贝其目标树。
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		target, err := resolveSymlink(src)
		if err != nil {
			return err
		}
		return CopyDir(target, dest)
	}

	if err := os.MkdirAll(dest, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		// 跳过版本控制与依赖目录（整棵子树都不拷贝）。
		if entry.IsDir() && skipDirs[entry.Name()] {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		// 入口层也处理符号链接：解析后判断目标是目录还是文件。
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := resolveSymlink(srcPath)
			if err != nil {
				return err
			}
			targetInfo, err := os.Stat(target)
			if err != nil {
				return fmt.Errorf("stat symlink target %s: %w", target, err)
			}
			if targetInfo.IsDir() {
				if err := CopyDir(target, destPath); err != nil {
					return err
				}
			} else {
				if err := copyFile(target, destPath); err != nil {
					return err
				}
			}
		} else if entry.IsDir() {
			if err := CopyDir(srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, destPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolveSymlink 返回符号链接指向的绝对路径（若是相对路径，则基于
// 链接所在目录解析）。
func resolveSymlink(link string) (string, error) {
	target, err := os.Readlink(link)
	if err != nil {
		return "", fmt.Errorf("reading symlink %s: %w", link, err)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(link), target)
	}
	return target, nil
}

// copyFile 以流式方式拷贝单个文件并保留其权限位。
//
// 利用已打开的源文件句柄调用 Stat，避免再次 os.Stat 造成的额外系统
// 调用——在大目录递归拷贝（BenchmarkCopyDirRecursive）下能显著
// 减少系统调用次数。
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// 直接从已打开的句柄拿文件信息，省去一次独立的 os.Stat。
	srcInfo, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	// 若 io.Copy 失败，关闭前主动删除可能写了一半的目标文件，
	// 避免留下损坏内容。close 错误以 defer 形式忽略，因为拷贝失败
	// 本身已是更严重的错误。
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dest)
		return err
	}
	return out.Close()
}

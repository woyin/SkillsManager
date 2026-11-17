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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
)

// skipDirs 是永远不拷贝的目录条目：版本控制元数据、依赖树与构建产物，
// 拷贝它们既浪费空间又无还原价值（对齐 npx skills 的拷贝排除集）。
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"dist":         true,
	"build":        true,
	"__pycache__":  true,
}

// CopyDir 把 src 为根的整棵目录树拷贝到 dest。
//
// 行为约定：
//   - dest 不存在时创建；存在时合并写入同名文件；
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

		if err := copyEntry(srcPath, destPath, entry); err != nil {
			return err
		}
	}
	return nil
}

// copyEntry 拷贝单个目录项：符号链接解析后按目标类型递归拷贝或拷文件，
// 普通目录递归拷贝，普通文件直接拷贝。
func copyEntry(srcPath, destPath string, entry os.DirEntry) error {
	info, err := entry.Info()
	if err != nil {
		return err
	}

	// 入口层也处理符号链接：解析后判断目标是目录还是文件。
	if info.Mode()&os.ModeSymlink != 0 {
		return copySymlinkEntry(srcPath, destPath)
	}
	if entry.IsDir() {
		return CopyDir(srcPath, destPath)
	}
	return copyFile(srcPath, destPath)
}

// copySymlinkEntry 解析符号链接目标后按目录/文件递归拷贝。
func copySymlinkEntry(srcPath, destPath string) error {
	target, err := resolveSymlink(srcPath)
	if err != nil {
		return err
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("stat symlink target %s: %w", target, err)
	}
	if targetInfo.IsDir() {
		return CopyDir(target, destPath)
	}
	return copyFile(target, destPath)
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

// WriteTarget 描述一次写入最终落到哪个真实文件（symlink 已解析）。
// Exists=false 表示本次写入会新建该文件（原链接可能悬空）。
type WriteTarget struct {
	Path   string
	Exists bool
	Mode   os.FileMode
}

// ResolveWriteTarget 解析路径的实际写入目标：跟随文件 symlink 链（最多 8 跳，
// 相对目标按链接所在目录解析），允许最终 referent 不存在（悬空链接）。
// 目录目标或解析环/超跳返回错误，且不修改任何文件。
//
// 它同时服务 WriteFileAtomic 与需要记录写前状态的调用方（如项目配置快照），
// 使“写哪里”和“备份哪里”使用同一套解析规则。
func ResolveWriteTarget(path string) (WriteTarget, error) {
	if path == "" {
		return WriteTarget{}, fmt.Errorf("resolving write target: empty path")
	}
	current := path
	for hop := 0; hop < 8; hop++ {
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				// 悬空 referent：写入时创建它，链接保持不变。
				return WriteTarget{Path: current}, nil
			}
			return WriteTarget{}, fmt.Errorf("inspecting %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			if info.IsDir() {
				return WriteTarget{}, fmt.Errorf("writing %s: target is a directory", current)
			}
			return WriteTarget{Path: current, Exists: true, Mode: info.Mode().Perm()}, nil
		}
		link, readErr := os.Readlink(current)
		if readErr != nil {
			return WriteTarget{}, fmt.Errorf("reading symlink %s: %w", current, readErr)
		}
		if !filepath.IsAbs(link) {
			link = filepath.Join(filepath.Dir(current), link)
		}
		current = filepath.Clean(link)
	}
	return WriteTarget{}, fmt.Errorf("resolving %s: too many symlink levels", path)
}

// WriteFileAtomic 以“同目录临时文件 → 关闭 → rename”替换实际写入目标。
//
// 语义契约（plan A1）：
//   - symlink 链被解析到最终 referent，链接本身保持不变；悬空链接会创建 referent；
//   - 新建文件使用 perm，并**由内核应用 umask**（与 os.WriteFile 一致）；
//   - 既有文件的权限位被精确保留；
//   - 目标是只读文件时返回包裹 os.ErrPermission 的错误；
//   - 目录目标、解析环、referent 父目录缺失都报错且不破坏原状；
//   - 失败时不残留临时文件。
//
// 不提供 fsync 或目录同步，因此不构成掉电持久保证。
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	target, err := ResolveWriteTarget(path)
	if err != nil {
		return err
	}
	if target.Exists && !fileWritable(target.Mode) {
		return fmt.Errorf("writing %s (read-only target): %w", target.Path, os.ErrPermission)
	}

	dir := filepath.Dir(target.Path)
	base := filepath.Base(target.Path)
	var tmp *os.File
	var tmpName string
	if target.Exists {
		file, createErr := os.CreateTemp(dir, "."+base+".tmp-*")
		if createErr != nil {
			return fmt.Errorf("creating temporary file for %s: %w", target.Path, createErr)
		}
		tmp, tmpName = file, file.Name()
	} else {
		file, name, createErr := createTempWithPerm(dir, base, perm)
		if createErr != nil {
			return fmt.Errorf("creating temporary file for %s: %w", target.Path, createErr)
		}
		tmp, tmpName = file, name
	}
	discard := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, writeErr := tmp.Write(data); writeErr != nil {
		discard()
		return fmt.Errorf("writing temporary file for %s: %w", target.Path, writeErr)
	}
	if target.Exists {
		// 既有文件的 mode 已被观测，精确恢复不是放宽权限。
		if chmodErr := tmp.Chmod(target.Mode); chmodErr != nil {
			discard()
			return fmt.Errorf("setting permissions on temporary file for %s: %w", target.Path, chmodErr)
		}
	}
	if closeErr := tmp.Close(); closeErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("closing temporary file for %s: %w", target.Path, closeErr)
	}
	if renameErr := os.Rename(tmpName, target.Path); renameErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("renaming temporary file to %s: %w", target.Path, renameErr)
	}
	return nil
}

// atomicTempCounter 保证同进程内的临时文件名唯一。
var atomicTempCounter uint64

// createTempWithPerm 用 O_CREATE|O_EXCL 与调用方 perm 创建临时文件，让内核
// 应用 umask；只在名字冲突时重试，不 chmod。
func createTempWithPerm(dir, base string, perm os.FileMode) (*os.File, string, error) {
	for attempt := 0; attempt < 8; attempt++ {
		seq := atomic.AddUint64(&atomicTempCounter, 1)
		name := filepath.Join(dir, fmt.Sprintf(".%s.tmp-%d-%d", base, os.Getpid(), seq))
		file, openErr := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if openErr == nil {
			return file, name, nil
		}
		if !errors.Is(openErr, os.ErrExist) {
			return nil, "", openErr
		}
	}
	return nil, "", fmt.Errorf("creating unique temporary file for %s: too many attempts", base)
}

// fileWritable 报告 perm 是否包含任一写位。用权限位判断以保留既有
// “只读文件写入失败”的语义；不查询 ACL 或挂载选项。
func fileWritable(perm os.FileMode) bool {
	return perm&0222 != 0
}

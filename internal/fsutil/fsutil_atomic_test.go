package fsutil

// fsutil_atomic_test.go 覆盖 WriteFileAtomic 的 symlink、目录目标与失败语义
// （跨平台部分）。umask 与权限位断言见 fsutil_atomic_posix_test.go（!windows）。

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteFileAtomicCreatesDanglingSymlinkTarget：目标是悬空文件 symlink 且
// referent 父目录存在时，必须创建 referent 并保留原 symlink（与旧 os.WriteFile 一致）。
func TestWriteFileAtomicCreatesDanglingSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	referentDir := filepath.Join(dir, "targets")
	if err := os.MkdirAll(referentDir, 0755); err != nil {
		t.Fatalf("mkdir referent dir: %v", err)
	}
	referent := filepath.Join(referentDir, "config.json")
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(referent, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := WriteFileAtomic(link, []byte("payload"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic through dangling symlink: %v", err)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("original symlink must be preserved: info=%v err=%v", info, err)
	}
	data, err := os.ReadFile(referent)
	if err != nil || string(data) != "payload" {
		t.Fatalf("referent content = %q (err=%v), want payload", data, err)
	}
}

// TestWriteFileAtomicFollowsSymlinkChainToNewReferent：多跳 symlink 链必须解析到
// 最终 referent 并在其父目录存在时创建文件。
func TestWriteFileAtomicFollowsSymlinkChainToNewReferent(t *testing.T) {
	dir := t.TempDir()
	referentDir := filepath.Join(dir, "targets")
	if err := os.MkdirAll(referentDir, 0755); err != nil {
		t.Fatalf("mkdir referent dir: %v", err)
	}
	referent := filepath.Join(referentDir, "final.json")
	middle := filepath.Join(dir, "middle.json")
	first := filepath.Join(dir, "first.json")
	if err := os.Symlink(middle, first); err != nil {
		t.Fatalf("symlink first: %v", err)
	}
	if err := os.Symlink(referent, middle); err != nil {
		t.Fatalf("symlink middle: %v", err)
	}

	if err := WriteFileAtomic(first, []byte("chain"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic through symlink chain: %v", err)
	}
	data, err := os.ReadFile(referent)
	if err != nil || string(data) != "chain" {
		t.Fatalf("final referent content = %q (err=%v), want chain", data, err)
	}
}

// TestWriteFileAtomicReportsSymlinkLoop：symlink 环必须返回明确错误，
// 且不破坏任何链接。
func TestWriteFileAtomicReportsSymlinkLoop(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	if err := os.Symlink(b, a); err != nil {
		t.Fatalf("symlink a: %v", err)
	}
	if err := os.Symlink(a, b); err != nil {
		t.Fatalf("symlink b: %v", err)
	}
	err := WriteFileAtomic(a, []byte("x"), 0644)
	if err == nil {
		t.Fatal("expected error for symlink loop")
	}
	for _, link := range []string{a, b} {
		info, statErr := os.Lstat(link)
		if statErr != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("symlink %s must stay intact: info=%v err=%v", link, info, statErr)
		}
	}
}

// TestWriteFileAtomicDanglingSymlinkMissingParent：（回归）referent 父目录不存在时
// 必须报错且不创建任何目录、不改动链接。
func TestWriteFileAtomicDanglingSymlinkMissingParent(t *testing.T) {
	dir := t.TempDir()
	referent := filepath.Join(dir, "missing-dir", "config.json")
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(referent, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := WriteFileAtomic(link, []byte("x"), 0644); err == nil {
		t.Fatal("expected error when referent parent is missing")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "missing-dir")); !os.IsNotExist(statErr) {
		t.Errorf("must not create referent parent dir, got %v", statErr)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link must stay intact: info=%v err=%v", info, err)
	}
}

// TestAtomicWritePreservesModeAndFailure：替换已有文件时写入新内容、不遗留临时
// 文件；父目录缺失时报错。（权限位断言在 posix 文件，见 TestAtomicWritePreservesExistingMode。）
func TestAtomicWritePreservesModeAndFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := WriteFileAtomic(path, []byte("new"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "new" {
		t.Fatalf("content = %q (err=%v), want new", data, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("temporary files left behind: %v", entries)
	}
	if err := WriteFileAtomic(filepath.Join(dir, "missing", "x.json"), []byte("x"), 0644); err == nil {
		t.Error("expected error for missing parent directory")
	}
}

// TestAtomicWritePreservesSymlinkTarget：目标是 symlink 时写其指向的真实文件，
// 不替换 symlink 本身。
func TestAtomicWritePreservesSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	realPath := filepath.Join(dir, "real.json")
	if err := os.WriteFile(realPath, []byte("old"), 0644); err != nil {
		t.Fatalf("seed real file: %v", err)
	}
	linkPath := filepath.Join(dir, "link.json")
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := WriteFileAtomic(linkPath, []byte("new"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic through symlink: %v", err)
	}
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("Lstat link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink was replaced by a regular file")
	}
	data, err := os.ReadFile(realPath)
	if err != nil || string(data) != "new" {
		t.Fatalf("real file content = %q (err=%v), want new", data, err)
	}
}

// TestAtomicWriteRejectsDirectoryTarget：目标为目录时明确报错且不破坏目录。
func TestAtomicWriteRejectsDirectoryTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "adir")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := WriteFileAtomic(target, []byte("x"), 0644); err == nil {
		t.Fatal("expected error when target is a directory")
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		t.Fatalf("directory target damaged: info=%v err=%v", info, err)
	}
}

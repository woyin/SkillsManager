package fsutil

// fsutil_atomic_test.go 覆盖 WriteFileAtomic 的权限、symlink 与只读语义。

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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

// TestWriteFileAtomicNewFileHonorsUmask：新建文件的 perm 必须由内核应用 umask
// （与 os.WriteFile 一致），不能靠 chmod 把新文件固定成 0644。
// umask 是进程级状态，因此用子进程隔离，不改进程全局 umask。
func TestWriteFileAtomicNewFileHonorsUmask(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("umask/permission semantics are not enforced for root")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "new.json")
	mode := runUmaskChild(t, target)
	if mode != 0600 {
		t.Errorf("new file mode = %04o, want 0600 (umask 077)", mode)
	}
}

// TestWriteFileAtomicExistingFileKeepsModeUnderUmask：既有文件权限被精确保留，
// umask 只影响新建文件。
func TestWriteFileAtomicExistingFileKeepsModeUnderUmask(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("umask/permission semantics are not enforced for root")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "existing.json")
	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := os.Chmod(target, 0644); err != nil {
		t.Fatalf("chmod seed file: %v", err)
	}
	mode := runUmaskChild(t, target)
	if mode != 0644 {
		t.Errorf("existing file mode = %04o, want 0644 (umask must not shrink it)", mode)
	}
}

// TestWriteFileAtomicReadOnlyErrorIsPermission：只读目标错误必须可被
// errors.Is(err, os.ErrPermission) 识别（与旧 os.WriteFile 的 EACCES 一致）。
func TestWriteFileAtomicReadOnlyErrorIsPermission(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission semantics are not enforced for root")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "readonly.json")
	if err := os.WriteFile(target, []byte("keep"), 0644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := os.Chmod(target, 0444); err != nil {
		t.Fatalf("chmod read-only: %v", err)
	}
	err := WriteFileAtomic(target, []byte("replace"), 0644)
	if err == nil {
		t.Fatal("expected error for read-only target")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("error must wrap os.ErrPermission, got: %v", err)
	}
}

// runUmaskChild 在设置了 umask 077 的子进程中执行一次原子写，返回结果 mode。
func runUmaskChild(t *testing.T, target string) os.FileMode {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperUmaskProcess") //nolint:gosec // 测试自身二进制
	cmd.Env = append(os.Environ(), "SM_UMASK_HELPER=1", "SM_UMASK_TARGET="+target)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("umask helper failed: %v\n%s", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "MODE=") {
			continue
		}
		value, parseErr := strconv.ParseUint(strings.TrimPrefix(line, "MODE="), 8, 32)
		if parseErr != nil {
			t.Fatalf("parse mode from %q: %v", line, parseErr)
		}
		return os.FileMode(value)
	}
	t.Fatalf("helper output missing MODE line: %s", out)
	return 0
}

// TestHelperUmaskProcess 是 umask 子进程入口；仅在 SM_UMASK_HELPER 下执行。
func TestHelperUmaskProcess(t *testing.T) {
	if os.Getenv("SM_UMASK_HELPER") == "" {
		t.Skip("helper process only")
	}
	syscall.Umask(0077)
	target := os.Getenv("SM_UMASK_TARGET")
	if err := WriteFileAtomic(target, []byte("written"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic in helper: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat in helper: %v", err)
	}
	fmt.Printf("MODE=%04o\n", info.Mode().Perm())
}

// TestAtomicWritePreservesModeAndFailure：替换已有文件时保留原权限；
// 写入失败时旧内容保持不变。
func TestAtomicWritePreservesModeAndFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := WriteFileAtomic(path, []byte("new"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("mode = %v, want 0600 (existing mode preserved)", perm)
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

// TestAtomicWriteRejectsReadOnlyTarget：只读目标必须报错且内容不变，
// 不能借 rename 绕过既有权限语义。
func TestAtomicWriteRejectsReadOnlyTarget(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission semantics are not enforced for root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "readonly.json")
	if err := os.WriteFile(path, []byte("keep"), 0444); err != nil {
		t.Fatalf("seed read-only file: %v", err)
	}
	if err := WriteFileAtomic(path, []byte("replace"), 0644); err == nil {
		t.Fatal("expected permission error for read-only target")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "keep" {
		t.Fatalf("read-only content = %q (err=%v), want keep", data, err)
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

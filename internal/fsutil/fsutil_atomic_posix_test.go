//go:build !windows

package fsutil

// fsutil_atomic_posix_test.go 承载真正 POSIX 语义的断言（syscall.Umask、权限位、
// 只读 EACCES）。Windows 不套用这些语义，因此整文件排除在 Windows 构建之外。

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

// TestAtomicWritePreservesExistingMode：替换已有文件时精确保留原权限位。
func TestAtomicWritePreservesExistingMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatalf("chmod seed file: %v", err)
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

// TestCopyDirPreservesExecutableMode：复制保留可执行权限位（POSIX 语义）。
func TestCopyDirPreservesExecutableMode(t *testing.T) {
	src := t.TempDir()
	executable := filepath.Join(src, "bin", "run")
	if err := os.MkdirAll(filepath.Dir(executable), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "out")
	if err := CopyDir(src, dest); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	info, err := os.Stat(filepath.Join(dest, "bin", "run"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("copied mode = %o, want 0755", info.Mode().Perm())
	}
}

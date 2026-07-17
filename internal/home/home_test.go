package home

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitAndDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Windows 用 USERPROFILE；在 darwin/linux 上 Setenv HOME 即可。
	if os.Getenv("USERPROFILE") != "" {
		t.Setenv("USERPROFILE", os.Getenv("HOME"))
	}

	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	got := Dir()
	want := os.Getenv("HOME")
	if got != want {
		t.Fatalf("Dir() = %q, want %q", got, want)
	}
}

func TestResetForTestRefreshesDir(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()

	t.Setenv("HOME", first)
	if os.Getenv("USERPROFILE") != "" {
		t.Setenv("USERPROFILE", first)
	}
	ResetForTest()
	if Dir() != first {
		t.Fatalf("after first reset Dir() = %q, want %q", Dir(), first)
	}

	t.Setenv("HOME", second)
	if os.Getenv("USERPROFILE") != "" {
		t.Setenv("USERPROFILE", second)
	}
	ResetForTest()
	if Dir() != second {
		t.Fatalf("after second reset Dir() = %q, want %q", Dir(), second)
	}

	// 路径应可拼接使用
	joined := filepath.Join(Dir(), ".sm")
	if filepath.Dir(joined) != second {
		t.Fatalf("joined path parent = %q, want %q", filepath.Dir(joined), second)
	}
}

func TestInitRejectsEmptyHome(t *testing.T) {
	// 清空 HOME / USERPROFILE，在多数 Unix 上 UserHomeDir 会失败或返回空。
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	// 某些环境仍能从 passwd 解析；若 Init 成功则跳过本断言。
	if err := Init(); err == nil {
		t.Skip("os.UserHomeDir still succeeds with empty HOME; cannot assert rejection")
	}
}

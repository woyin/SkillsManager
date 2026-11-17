package project

// project_atomic_test.go 验证 .sm.json 以原子方式保存：保留既有权限、
// 不残留临时文件、只读目标仍报错。

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSavePreservesExistingModeAndLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".sm.json")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatalf("seed .sm.json: %v", err)
	}
	mgr := NewManager(dir)
	if err := mgr.Save(&Config{Profile: "web"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf(".sm.json mode = %v, want 0600", perm)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("temporary files left behind: %v", entries)
	}
	loaded, err := mgr.Load()
	if err != nil || loaded.Profile != "web" {
		t.Fatalf("Load after Save = %+v (err=%v)", loaded, err)
	}
}

func TestSaveRejectsReadOnlyTarget(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission semantics are not enforced for root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".sm.json")
	original := []byte("{\"profile\":\"old\"}")
	if err := os.WriteFile(path, original, 0444); err != nil {
		t.Fatalf("seed read-only .sm.json: %v", err)
	}
	mgr := NewManager(dir)
	if err := mgr.Save(&Config{Profile: "new"}); err == nil {
		t.Fatal("expected permission error for read-only .sm.json")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(original) {
		t.Fatalf(".sm.json content = %q (err=%v), want unchanged", data, err)
	}
}

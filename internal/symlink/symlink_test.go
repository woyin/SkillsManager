// internal/symlink/symlink_test.go
package symlink

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "registry", "skills", "cloudflare", "my-skill")
	dst := filepath.Join(dir, "codex", "skills", "my-skill")

	os.MkdirAll(src, 0755)
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("# skill"), 0644)
	os.MkdirAll(filepath.Dir(dst), 0755)

	err := Create(src, dst)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("Readlink failed: %v", err)
	}
	if target != src {
		t.Errorf("Expected symlink to %s, got %s", src, target)
	}
}

func TestIsSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.MkdirAll(src, 0755)

	if IsSymlink(dst) {
		t.Error("Expected false for non-existent path")
	}

	Create(src, dst)
	if !IsSymlink(dst) {
		t.Error("Expected true for symlink")
	}
}

func TestVerifySymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.MkdirAll(src, 0755)
	Create(src, dst)

	if !Verify(dst) {
		t.Error("Expected valid symlink")
	}

	os.RemoveAll(src)
	if Verify(dst) {
		t.Error("Expected broken symlink to be invalid")
	}
}

func TestCleanupBroken(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.MkdirAll(src, 0755)
	Create(src, dst)
	os.RemoveAll(src)

	removed, err := RemoveIfBroken(dst)
	if err != nil {
		t.Fatalf("RemoveIfBroken failed: %v", err)
	}
	if !removed {
		t.Error("Expected broken symlink to be removed")
	}
	if _, err := os.Lstat(dst); !os.IsNotExist(err) {
		t.Error("Expected symlink to be gone")
	}
}

func TestFindSymlinksPointingTo(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst1 := filepath.Join(dir, "link1")
	dst2 := filepath.Join(dir, "link2")
	os.MkdirAll(src, 0755)
	Create(src, dst1)
	Create(src, dst2)

	links, err := FindPointingTo(dir, src)
	if err != nil {
		t.Fatalf("FindPointingTo failed: %v", err)
	}
	if len(links) != 2 {
		t.Errorf("Expected 2 symlinks, got %d", len(links))
	}
}

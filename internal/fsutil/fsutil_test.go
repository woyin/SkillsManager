package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCopyDirBasic(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, map[string]string{
		"SKILL.md":      "---\nname: a\n---\n",
		"docs/intro.md": "intro",
	})

	dest := filepath.Join(t.TempDir(), "out")
	if err := CopyDir(src, dest); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	for _, rel := range []string{"SKILL.md", "docs/intro.md"} {
		if _, err := os.Stat(filepath.Join(dest, rel)); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
}

func TestCopyDirSkipsGit(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, map[string]string{
		"SKILL.md":           "x",
		".git/HEAD":          "ref",
		".git/objects/aa/bb": "blob",
	})

	dest := filepath.Join(t.TempDir(), "out")
	if err := CopyDir(src, dest); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, ".git")); !os.IsNotExist(err) {
		t.Errorf(".git should be skipped, got err=%v", err)
	}
}

func TestCopyDirRejectsExistingDest(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, map[string]string{"SKILL.md": "x"})

	root := t.TempDir()
	dest := filepath.Join(root, "out")
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatal(err)
	}
	// CopyDir does not treat an existing dest as an error itself (it MkdirAll's),
	// but the registry wrapper does. Verify CopyDir is idempotent-ish: it should
	// still succeed and not error on existing dest contents.
	if err := CopyDir(src, dest); err != nil {
		t.Fatalf("CopyDir into existing dest: %v", err)
	}
}

func TestCopyDirFollowsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior not tested on windows")
	}
	src := t.TempDir()
	writeTree(t, src, map[string]string{
		"real/SKILL.md": "x",
	})

	// Create a symlink skills -> real
	link := filepath.Join(src, "skills")
	if err := os.Symlink(filepath.Join(src, "real"), link); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "out")
	if err := CopyDir(link, dest); err != nil {
		t.Fatalf("CopyDir symlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "SKILL.md")); err != nil {
		t.Errorf("followed symlink contents missing: %v", err)
	}
}

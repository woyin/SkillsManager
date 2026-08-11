package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeTree(t testing.TB, root string, files map[string]string) {
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

func TestCopyDirPreservesFileModesAndSkipsEveryExcludedDirectory(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, map[string]string{
		"SKILL.md":               "skill",
		".git/config":            "git",
		"node_modules/lib/a.js":  "dependency",
		"dist/bundle.js":         "build output",
		"build/output":           "build output",
		"__pycache__/module.pyc": "bytecode",
	})
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
	for _, name := range []string{".git", "node_modules", "dist", "build", "__pycache__"} {
		if _, err := os.Stat(filepath.Join(dest, name)); !os.IsNotExist(err) {
			t.Errorf("%s should be skipped, stat error = %v", name, err)
		}
	}
}

func TestCopyDirFollowsRelativeFileSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior not tested on windows")
	}
	src := t.TempDir()
	writeTree(t, src, map[string]string{"assets/real.txt": "content"})
	if err := os.Symlink(filepath.Join("assets", "real.txt"), filepath.Join(src, "linked.txt")); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "out")
	if err := CopyDir(src, dest); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "linked.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "content" {
		t.Fatalf("copied linked file = %q", data)
	}
}

func TestCopyDirRejectsBrokenSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior not tested on windows")
	}
	src := t.TempDir()
	if err := os.Symlink("missing.txt", filepath.Join(src, "broken.txt")); err != nil {
		t.Fatal(err)
	}
	if err := CopyDir(src, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("CopyDir should reject a broken entry symlink")
	}
}

func TestCopyDirAndHelpersReportInvalidPaths(t *testing.T) {
	if err := CopyDir(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("CopyDir should reject a missing source")
	}
	if _, err := resolveSymlink(filepath.Join(t.TempDir(), "missing-link")); err == nil {
		t.Fatal("resolveSymlink should reject a missing link")
	}

	src := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(src, []byte("source"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(filepath.Join(t.TempDir(), "missing.txt"), filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("copyFile should reject a missing source")
	}
	if err := copyFile(src, filepath.Join(t.TempDir(), "missing-parent", "out")); err == nil {
		t.Fatal("copyFile should reject a missing destination parent")
	}
	if err := copyFile(t.TempDir(), filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("copyFile should reject a directory source")
	}

	notDirectory := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(notDirectory, []byte("file"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CopyDir(src, notDirectory); err == nil {
		t.Fatal("CopyDir should reject a destination that is a file")
	}
	if err := CopyDir(src, filepath.Join(t.TempDir(), "source-file-out")); err == nil {
		t.Fatal("CopyDir should reject a file source")
	}
}

func BenchmarkCopyDirRecursive(b *testing.B) {
	src := b.TempDir()
	files := make(map[string]string, 80)
	for i := 0; i < 80; i++ {
		files[filepath.Join("nested", fmt.Sprintf("skill-%03d.md", i))] = "skill content"
	}
	writeTree(b, src, files)

	destRoot := b.TempDir()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := CopyDir(src, filepath.Join(destRoot, fmt.Sprintf("copy-%d", i))); err != nil {
			b.Fatal(err)
		}
	}
}

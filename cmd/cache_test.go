package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/tool"
)

func TestSourceCachesProtectsReferencedAndPrunesUnused(t *testing.T) {
	oldData := DataDir
	DataDir = t.TempDir()
	t.Cleanup(func() { DataDir = oldData })
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()

	makeCacheRepo := func(name string) string {
		t.Helper()
		repo := filepath.Join(DataDir, "sources", name)
		if err := os.MkdirAll(repo, 0755); err != nil {
			t.Fatal(err)
		}
		gitRun(t, "", "init", "-q", repo)
		gitRun(t, repo, "config", "user.name", "test")
		gitRun(t, repo, "config", "user.email", "test@example.com")
		writeUpdateSkill(t, repo, "cache\n")
		gitRun(t, repo, "add", ".")
		gitRun(t, repo, "commit", "-qm", "initial")
		return repo
	}
	used, unused := makeCacheRepo("used"), makeCacheRepo("unused")
	linkDir := filepath.Join(tmpHome, tool.Codex.SkillDir)
	if err := os.MkdirAll(linkDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(used, filepath.Join(linkDir, "used")); err != nil {
		t.Fatal(err)
	}

	items, err := sourceCaches()
	if err != nil || len(items) != 2 {
		t.Fatalf("items = %+v, %v", items, err)
	}
	for _, item := range items {
		if item.Path == used && item.Refs != 1 {
			t.Fatalf("used refs = %d", item.Refs)
		}
	}
	removed, _, err := pruneSourceCaches(items)
	if err != nil || removed != 1 {
		t.Fatalf("prune = %d, %v", removed, err)
	}
	if _, err := os.Stat(used); err != nil {
		t.Fatalf("used cache removed: %v", err)
	}
	if _, err := os.Stat(unused); !os.IsNotExist(err) {
		t.Fatalf("unused cache still exists: %v", err)
	}
}

func TestWriteSourceCachesShowsModeRefsAndSize(t *testing.T) {
	var out bytes.Buffer
	err := writeSourceCaches(&out, []sourceCache{{Source: "repo", Commit: "abc", Pinned: true, Refs: 2, Size: 2048}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SOURCE", "repo", "pinned", "2", "2.0 KiB"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q: %s", want, out.String())
		}
	}
}

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/lockfile"
	"github.com/woyin/skills-manager/internal/tool"
)

// setupRmGlobals 还原 rm 的包级 flag。
func setupRmGlobals(t *testing.T) {
	t.Helper()
	oldProject, oldDir := rmProject, rmDir
	t.Cleanup(func() { rmProject, rmDir = oldProject, oldDir })
}

// TestRmScanDirsDefaultIncludesProjectAndGlobal 默认扫全局 + 项目。
func TestRmScanDirsDefaultIncludesProjectAndGlobal(t *testing.T) {
	setupRmGlobals(t)
	rmProject = false
	rmDir = t.TempDir()
	dirs := rmScanDirs(tool.Claude)
	if len(dirs) != 2 {
		t.Fatalf("got %d dirs, want 2 (global + project)", len(dirs))
	}
}

// TestRmScanDirsProjectOnly 验证 --project 仅项目级。
func TestRmScanDirsProjectOnly(t *testing.T) {
	setupRmGlobals(t)
	projectDir := t.TempDir()
	rmProject = true
	rmDir = projectDir
	dirs := rmScanDirs(tool.Claude)
	if len(dirs) != 1 {
		t.Fatalf("got %d dirs, want 1 (project only)", len(dirs))
	}
	want := filepath.Join(projectDir, tool.Claude.ProjectSkillDir)
	if dirs[0] != want {
		t.Fatalf("project dir = %s, want %s", dirs[0], want)
	}
}

// TestRmFromAgentsCleansProjectScope 验证 rm --agent --project 清项目级 symlinks。
func TestRmFromAgentsCleansProjectScope(t *testing.T) {
	setupRmGlobals(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()

	registry := filepath.Join(t.TempDir(), "registry")
	oldRegistry := RegistryDir
	RegistryDir = registry
	t.Cleanup(func() { RegistryDir = oldRegistry })

	skill := makeRegistrySkill(t, registry, "global", "target")
	projLink := filepath.Join(projectDir, tool.Claude.ProjectSkillDir, "target")
	makeLink(t, projLink, skill)

	rmAgents = []string{"claude"}
	rmSkills = []string{"target"}
	rmProject = true
	rmDir = projectDir
	if err := removeFromAgents(nil); err != nil {
		t.Fatalf("removeFromAgents: %v", err)
	}
	assertGone(t, projLink)

	// 全局无此链接，确保不报错
	if _, err := os.Lstat(filepath.Join(home, tool.Claude.SkillDir, "target")); !os.IsNotExist(err) {
		t.Fatalf("global link unexpectedly exists")
	}
}

// TestRemoveFromProjectLock verifies that removing a skill also deletes its
// entry from the project skills-lock.json, keeping the lockfile in sync.
func TestRemoveFromProjectLock(t *testing.T) {
	setupRmGlobals(t)
	projectDir := t.TempDir()
	rmDir = projectDir

	// Seed a lockfile with two skills.
	lm := lockfile.NewManager(projectDir)
	lock := &lockfile.LocalLock{Skills: map[string]*lockfile.SkillEntry{
		"keep":  {Source: "owner/repo", SourceType: "github", ComputedHash: "h1"},
		"purge": {Source: "owner/repo", SourceType: "github", ComputedHash: "h2"},
	}}
	if err := lm.Save(lock); err != nil {
		t.Fatal(err)
	}

	removeFromProjectLock("purge")

	got, err := lm.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Skills["purge"]; ok {
		t.Error("purge should have been removed from lockfile")
	}
	if _, ok := got.Skills["keep"]; !ok {
		t.Error("keep should remain in lockfile")
	}
}

// TestRemoveFromProjectLockNoLockfile verifies no error when no lockfile exists.
func TestRemoveFromProjectLockNoLockfile(t *testing.T) {
	setupRmGlobals(t)
	rmDir = t.TempDir()
	// Should be a no-op without error.
	removeFromProjectLock("anything")
}

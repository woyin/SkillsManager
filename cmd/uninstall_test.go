package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/tool"
)

func TestUninstallCanRemoveOneSkillFromOneGlobalAgent(t *testing.T) {
	home := t.TempDir()
	registry := filepath.Join(t.TempDir(), "registry")
	oldRegistry := RegistryDir
	RegistryDir = registry
	t.Cleanup(func() { RegistryDir = oldRegistry })

	codexSkill := makeRegistrySkill(t, registry, "global", "safe")
	claudeSkill := makeRegistrySkill(t, registry, "global", "safe")
	otherSkill := makeRegistrySkill(t, registry, "global", "other")
	makeLink(t, filepath.Join(home, tool.Codex.SkillDir, "safe"), codexSkill)
	makeLink(t, filepath.Join(home, tool.Claude.SkillDir, "safe"), claudeSkill)
	makeLink(t, filepath.Join(home, tool.Codex.SkillDir, "other"), otherSkill)

	removed, err := removeInstalledSymlinks(uninstallOptions{homeDir: home, agents: []string{"codex"}, skills: []string{"safe"}})
	if err != nil {
		t.Fatalf("removeInstalledSymlinks failed: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed %d links, want 1", removed)
	}
	assertGone(t, filepath.Join(home, tool.Codex.SkillDir, "safe"))
	assertExists(t, filepath.Join(home, tool.Claude.SkillDir, "safe"))
	assertExists(t, filepath.Join(home, tool.Codex.SkillDir, "other"))
}

func TestUninstallCanRemoveOnlyCurrentProjectLinks(t *testing.T) {
	home := t.TempDir()
	projectDir := t.TempDir()
	registry := filepath.Join(t.TempDir(), "registry")
	oldRegistry := RegistryDir
	RegistryDir = registry
	t.Cleanup(func() { RegistryDir = oldRegistry })

	skill := makeRegistrySkill(t, registry, "global", "safe")
	globalLink := filepath.Join(home, tool.Codex.SkillDir, "safe")
	projectLink := filepath.Join(projectDir, tool.Codex.ProjectSkillDir, "safe")
	makeLink(t, globalLink, skill)
	makeLink(t, projectLink, skill)

	removed, err := removeInstalledSymlinks(uninstallOptions{homeDir: home, projectDir: projectDir, projectOnly: true})
	if err != nil {
		t.Fatalf("removeInstalledSymlinks failed: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed %d links, want 1", removed)
	}
	assertExists(t, globalLink)
	assertGone(t, projectLink)
}

func makeRegistrySkill(t *testing.T, registry, category, name string) string {
	t.Helper()
	path := filepath.Join(registry, "skills", category, name)
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("creating registry skill: %v", err)
	}
	return path
}

func makeLink(t *testing.T, link, target string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		t.Fatalf("creating link parent: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("creating symlink: %v", err)
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("%s should exist: %v", path, err)
	}
}

func assertGone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%s should be gone, stat err %v", path, err)
	}
}

func TestUninstallSkipsAgentsWithoutGlobalSkillDir(t *testing.T) {
	home := t.TempDir()
	registry := filepath.Join(t.TempDir(), "registry")
	oldRegistry := RegistryDir
	RegistryDir = registry
	t.Cleanup(func() { RegistryDir = oldRegistry })

	skill := makeRegistrySkill(t, registry, "global", "danger")
	homeRootLink := filepath.Join(home, "danger")
	makeLink(t, homeRootLink, skill)

	removed, err := removeInstalledSymlinks(uninstallOptions{homeDir: home, agents: []string{"promptscript"}})
	if err != nil {
		t.Fatalf("removeInstalledSymlinks failed: %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed %d links, want 0", removed)
	}
	assertExists(t, homeRootLink)
}

func TestUninstallDefaultRemovesProjectAndGlobal(t *testing.T) {
	home := t.TempDir()
	projectDir := t.TempDir()
	registry := filepath.Join(t.TempDir(), "registry")
	oldRegistry := RegistryDir
	RegistryDir = registry
	t.Cleanup(func() { RegistryDir = oldRegistry })

	skill := makeRegistrySkill(t, registry, "global", "safe")
	globalLink := filepath.Join(home, tool.Codex.SkillDir, "safe")
	projectLink := filepath.Join(projectDir, tool.Codex.ProjectSkillDir, "safe")
	makeLink(t, globalLink, skill)
	makeLink(t, projectLink, skill)

	removed, err := removeInstalledSymlinks(uninstallOptions{homeDir: home, projectDir: projectDir, agents: []string{"codex"}, skills: []string{"safe"}})
	if err != nil {
		t.Fatalf("removeInstalledSymlinks: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed %d, want 2 (project+global)", removed)
	}
	assertGone(t, globalLink)
	assertGone(t, projectLink)
}

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/home"
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

// TestEveSubagentSkillDirsDiscoversSubagentDirs verifies the Eve subagent
// directory discovery used by rm to clean agent/subagents/<name>/skills.
func TestEveSubagentSkillDirsDiscoversSubagentDirs(t *testing.T) {
	setupRmGlobals(t)
	projectDir := t.TempDir()
	rmDir = projectDir

	// Two subagents: one with a skills dir, one without.
	resSkills := filepath.Join(projectDir, "agent", "subagents", "researcher", "skills")
	emptySub := filepath.Join(projectDir, "agent", "subagents", "empty")
	if err := os.MkdirAll(resSkills, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(emptySub, 0755); err != nil {
		t.Fatal(err)
	}

	dirs := eveSubagentSkillDirs()
	if len(dirs) != 1 {
		t.Fatalf("got %d dirs, want 1 (only subagents with a skills dir)", len(dirs))
	}
	if dirs[0] != resSkills {
		t.Errorf("dir = %s, want %s", dirs[0], resSkills)
	}
}

// TestRemoveSkillCleansEveSubagentDir verifies that rm removes a skill
// installed into an Eve subagent directory (agent/subagents/<name>/skills),
// which is outside the per-tool skill-dir scan. Mirrors npx skills remove,
// which scans getEveSubagentSkillsDir for each subagent.
func TestRemoveSkillCleansEveSubagentDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	withTestRegistry(t)

	setupRmGlobals(t)
	projectDir := t.TempDir()
	rmDir = projectDir

	// Place a skill directly in an Eve subagent skills dir (no registry
	// original) and ensure rm removes it.
	subSkillDir := filepath.Join(projectDir, "agent", "subagents", "researcher", "skills", "alpha")
	if err := os.MkdirAll(subSkillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subSkillDir, "SKILL.md"), []byte("# alpha"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := removeSkill("alpha", []string{"alpha"}); err != nil {
		t.Fatalf("removeSkill: %v", err)
	}
	if _, err := os.Lstat(subSkillDir); !os.IsNotExist(err) {
		t.Fatalf("eve subagent skill dir still exists after rm: %v", err)
	}
}

func TestRemoveMCPAndAllRegistrySkills(t *testing.T) {
	setupRmGlobals(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	registryDir := t.TempDir()
	oldRegistry, oldYes := RegistryDir, rmYes
	RegistryDir, rmYes = registryDir, true
	t.Cleanup(func() { RegistryDir, rmYes = oldRegistry, oldYes })

	if err := os.MkdirAll(filepath.Join(registryDir, "mcp"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(registryDir, "mcp", "demo.json"), []byte(`{"mcpServers":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := removeMCP("demo"); err != nil {
		t.Fatalf("removeMCP: %v", err)
	}
	if _, err := os.Stat(filepath.Join(registryDir, "mcp", "demo.json")); !os.IsNotExist(err) {
		t.Fatalf("MCP file should be removed: %v", err)
	}

	for _, name := range []string{"first", "second"} {
		dir := filepath.Join(registryDir, "skills", "global", name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := removeAll(); err != nil {
		t.Fatalf("removeAll: %v", err)
	}
	for _, name := range []string{"first", "second"} {
		if _, err := os.Stat(filepath.Join(registryDir, "skills", "global", name)); !os.IsNotExist(err) {
			t.Fatalf("skill %s should be removed: %v", name, err)
		}
	}
}

func TestCountReferencesToSeesGlobalAndProjectInstall(t *testing.T) {
	setupRmGlobals(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	projectDir := t.TempDir()
	rmDir = projectDir
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	for _, link := range []string{
		filepath.Join(tmpHome, tool.Claude.SkillDir, "linked"),
		filepath.Join(projectDir, tool.Claude.ProjectSkillDir, "linked"),
	} {
		if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(source, link); err != nil {
			t.Fatal(err)
		}
	}
	if got := countReferencesTo(source, "linked"); got < 2 {
		t.Fatalf("countReferencesTo = %d, want at least 2", got)
	}
}

func TestRemoveSkillLegacyRemovesInstallsAndRegistryOriginal(t *testing.T) {
	setupRmGlobals(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	registryDir := t.TempDir()
	oldRegistry := RegistryDir
	RegistryDir = registryDir
	t.Cleanup(func() { RegistryDir = oldRegistry })
	projectDir := t.TempDir()
	rmDir, rmProject = projectDir, false

	source := makeRegistrySkill(t, registryDir, "global", "legacy")
	for _, link := range []string{
		filepath.Join(tmpHome, tool.Claude.SkillDir, "legacy"),
		filepath.Join(projectDir, tool.Claude.ProjectSkillDir, "legacy"),
	} {
		if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(source, link); err != nil {
			t.Fatal(err)
		}
	}
	if err := removeSkillLegacy("legacy", []string{"legacy", "global"}); err != nil {
		t.Fatalf("removeSkillLegacy: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("registry original should be removed: %v", err)
	}
}

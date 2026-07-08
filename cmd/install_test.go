package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/tool"
)

// makeLocalSkillSource 在 dir 下造一个含 SKILL.md 的本地技能源目录，
// 返回源根目录（可直接传给 installSkillsToAgents 的 source 参数）。
func makeLocalSkillSource(t *testing.T, dir, name string) string {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("creating local skill source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: test\n---\n# "+name+"\n"), 0644); err != nil {
		t.Fatalf("writing SKILL.md: %v", err)
	}
	return dir
}

// TestInstallProjectScopeWritesProjectDir 验证 --project 把技能装到
// projectDir/<ProjectSkillDir>/<name> 而非全局 ~/SkillDir/<name>。
func TestInstallProjectScopeWritesProjectDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	source := t.TempDir()
	makeLocalSkillSource(t, source, "alpha")

	err := installSkillsToAgents(source, []string{"claude"}, []string{"*"}, false, true, projectDir)
	if err != nil {
		t.Fatalf("installSkillsToAgents project: %v", err)
	}

	want := filepath.Join(projectDir, tool.Claude.ProjectSkillDir, "alpha")
	assertExists(t, want)

	// 全局目录不应出现
	globalLink := filepath.Join(home, tool.Claude.SkillDir, "alpha")
	if _, err := os.Lstat(globalLink); !os.IsNotExist(err) {
		t.Fatalf("global link should not exist, got %v", err)
	}
}

// TestInstallGlobalScopeWritesHomeDir 验证默认（project=false）仍落全局目录。
func TestInstallGlobalScopeWritesHomeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	source := t.TempDir()
	makeLocalSkillSource(t, source, "beta")

	err := installSkillsToAgents(source, []string{"claude"}, []string{"*"}, false, false, "")
	if err != nil {
		t.Fatalf("installSkillsToAgents global: %v", err)
	}

	want := filepath.Join(home, tool.Claude.SkillDir, "beta")
	assertExists(t, want)
}
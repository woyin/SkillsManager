// cmd/add_test.go 验证 `sm add`（Register）的 Registry-first 契约：
// 默认 global、本地单文件物化、本地集合 --skill/--all、同名同源刷新、
// 同名不同源失败与 --force、写入前验证拒绝。
package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/registry"
)

func setupAddTest(t *testing.T) string {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	return withTestRegistry(t)
}

// makeValidSkillDir 在 dir 下造一个名为 name、含合法 frontmatter 的技能目录。
func makeValidSkillDir(t *testing.T, dir, name string) string {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\nname: "+name+"\ndescription: a valid test skill\n---\n# "+name+"\n"), 0644)
	return skillDir
}

func TestAddLocalSkillDefaultsToGlobal(t *testing.T) {
	regDir := setupAddTest(t)
	src := t.TempDir()
	makeValidSkillDir(t, src, "one-skill")

	err := registerFromSource(registry.New(regDir), src, "", nil, false, "", false)
	if err != nil {
		t.Fatalf("registerFromSource: %v", err)
	}
	dest := filepath.Join(regDir, "skills", "global", "one-skill")
	if _, err := os.Stat(filepath.Join(dest, "SKILL.md")); err != nil {
		t.Errorf("skill not in registry/global: %v", err)
	}
}

func TestAddSingleSkillMDMaterializesToDir(t *testing.T) {
	regDir := setupAddTest(t)
	src := t.TempDir()
	file := filepath.Join(src, "SKILL.md")
	os.WriteFile(file, []byte("---\nname: file-skill\ndescription: from a single file\n---\n# body"), 0644)

	err := registerFromSource(registry.New(regDir), file, "", nil, false, "", false)
	if err != nil {
		t.Fatalf("single file add: %v", err)
	}
	want := filepath.Join(regDir, "skills", "global", "file-skill", "SKILL.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("single file not materialized: %v", err)
	}
}

func TestAddRejectsNonSkillMDFile(t *testing.T) {
	regDir := setupAddTest(t)
	src := t.TempDir()
	file := filepath.Join(src, "README.md")
	os.WriteFile(file, []byte("# not a skill"), 0644)

	err := registerFromSource(registry.New(regDir), file, "", nil, false, "", false)
	if err == nil {
		t.Fatal("expected error for non-SKILL.md single file")
	}
}

func TestAddLocalCollectionNonInteractiveFailsWithoutSelector(t *testing.T) {
	regDir := setupAddTest(t)
	src := t.TempDir()
	// 集合：两个技能。
	makeValidSkillDir(t, filepath.Join(src, "skills"), "a")
	makeValidSkillDir(t, filepath.Join(src, "skills"), "b")

	err := registerFromSource(registry.New(regDir), src, "", nil, false, "", false)
	if err == nil {
		t.Fatal("expected failure for multi-skill source without selector in non-TTY")
	}
}

func TestAddLocalCollectionWithSkillSelector(t *testing.T) {
	regDir := setupAddTest(t)
	src := t.TempDir()
	makeValidSkillDir(t, filepath.Join(src, "skills"), "a")
	makeValidSkillDir(t, filepath.Join(src, "skills"), "b")

	err := registerFromSource(registry.New(regDir), src, "", []string{"a"}, false, "", false)
	if err != nil {
		t.Fatalf("select a: %v", err)
	}
	if _, err := os.Stat(filepath.Join(regDir, "skills", "global", "a", "SKILL.md")); err != nil {
		t.Errorf("skill a not registered: %v", err)
	}
	if _, err := os.Stat(filepath.Join(regDir, "skills", "global", "b", "SKILL.md")); !os.IsNotExist(err) {
		t.Error("skill b should NOT be registered")
	}
}

func TestAddLocalCollectionWithAll(t *testing.T) {
	regDir := setupAddTest(t)
	src := t.TempDir()
	makeValidSkillDir(t, filepath.Join(src, "skills"), "a")
	makeValidSkillDir(t, filepath.Join(src, "skills"), "b")

	err := registerFromSource(registry.New(regDir), src, "", []string{"*"}, false, "", false)
	if err != nil {
		t.Fatalf("select all: %v", err)
	}
	for _, n := range []string{"a", "b"} {
		if _, err := os.Stat(filepath.Join(regDir, "skills", "global", n, "SKILL.md")); err != nil {
			t.Errorf("skill %s not registered: %v", n, err)
		}
	}
}

func TestAddSameSourceRefreshes(t *testing.T) {
	regDir := setupAddTest(t)
	src := t.TempDir()
	makeValidSkillDir(t, src, "dup")
	reg := registry.New(regDir)

	if err := registerFromSource(reg, src, "", nil, false, "", false); err != nil {
		t.Fatalf("first: %v", err)
	}
	// 第二次同源：刷新，不报错。
	if err := registerFromSource(reg, src, "", nil, false, "", false); err != nil {
		t.Fatalf("refresh: %v", err)
	}
}

func TestAddDifferentSourceFailsWithoutForce(t *testing.T) {
	regDir := setupAddTest(t)
	src1 := t.TempDir()
	src2 := t.TempDir()
	makeValidSkillDir(t, src1, "shared-name")
	makeValidSkillDir(t, src2, "shared-name")
	reg := registry.New(regDir)

	if err := registerFromSource(reg, src1, "", nil, false, "", false); err != nil {
		t.Fatalf("first: %v", err)
	}
	err := registerFromSource(reg, src2, "", nil, false, "", false)
	if err == nil {
		t.Fatal("expected cross-source failure without --force")
	}
}

func TestAddDifferentSourceForceReplaces(t *testing.T) {
	regDir := setupAddTest(t)
	src1 := t.TempDir()
	src2 := t.TempDir()
	makeValidSkillDir(t, src1, "shared-name")
	// 给 src2 不同的内容以便区分
	os.WriteFile(filepath.Join(src2, "shared-name", "SKILL.md"),
		[]byte("---\nname: shared-name\ndescription: replaced content\n---\n# new"), 0644)
	os.MkdirAll(src2, 0755)
	makeValidSkillDir(t, src2, "shared-name")
	reg := registry.New(regDir)

	if err := registerFromSource(reg, src1, "", nil, false, "", false); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := registerFromSource(reg, src2, "", nil, true, "", false); err != nil {
		t.Fatalf("force replace: %v", err)
	}
}

func TestAddRejectsInvalidNameBeforeWrite(t *testing.T) {
	regDir := setupAddTest(t)
	src := t.TempDir()
	skillDir := filepath.Join(src, "bad")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\nname: Bad_Name\ndescription: x\n---\n# x"), 0644)

	err := registerFromSource(registry.New(regDir), src, "", nil, false, "", false)
	if err == nil {
		t.Fatal("expected validation error for invalid name")
	}
	// Registry 必须为空。
	if _, err := os.Stat(filepath.Join(regDir, "skills", "global", "bad")); !os.IsNotExist(err) {
		t.Error("invalid skill must not be written")
	}
}

func TestAddRejectsMissingDescriptionBeforeWrite(t *testing.T) {
	regDir := setupAddTest(t)
	src := t.TempDir()
	skillDir := filepath.Join(src, "nodesc")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\nname: nodesc\n---\n# x"), 0644)

	err := registerFromSource(registry.New(regDir), src, "", nil, false, "", false)
	if err == nil {
		t.Fatal("expected validation error for missing description")
	}
}

func TestAddCrossCategorySameNameFails(t *testing.T) {
	regDir := setupAddTest(t)
	src := t.TempDir()
	makeValidSkillDir(t, src, "x-skill")
	reg := registry.New(regDir)

	if err := registerFromSource(reg, src, "global", nil, false, "", false); err != nil {
		t.Fatalf("global: %v", err)
	}
	err := registerFromSource(reg, src, "codex-only", nil, false, "", false)
	if err == nil {
		t.Fatal("expected cross-category same-name failure")
	}
}

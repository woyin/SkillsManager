package curation

// 本文件固定 bootstrap 应用的专用语义：无显式目标拒绝、ownership 校验失败时
// 撤销本次安装、配置单点写盘失败时保持原字节、仅 MCP 也必须开事务，
// 以及 baseline.Skills 只写 --skill 额外项（profile 展开不落顶层）。

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/tool"
)

// TestBootstrapOwnershipFailureRollsBack：bootstrap 的 ownership 校验失败时安装被撤销。
func TestBootstrapOwnershipFailureRollsBack(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("bar")
	plan := &Plan{
		Project:     env.projectDir,
		Baseline:    &Baseline{},
		Bootstrap:   true,
		registryDir: env.registryDir,
	}
	foreign := filepath.Join(t.TempDir(), "bar")
	_ = os.Symlink(filepath.Join(env.registryDir, "skills", "global", "bar"), foreign)
	tx := &fakeTx{
		cfg:     &project.Config{},
		added:   []AddedEntity{{Agent: "", Skill: "bar", Path: foreign, Symlink: true}},
		created: []string{foreign},
	}
	opts := ApplyOptions{
		ApplyRemovals:  true,
		ExplicitTarget: &ExplicitTarget{Skills: []string{"bar"}},
		BeginAdditions: func([]string) (AdditionTx, error) { return tx, nil },
	}
	_, err := plan.Apply(opts)
	if err == nil {
		t.Fatal("expected ownership validation failure")
	}
	if _, statErr := os.Lstat(foreign); statErr == nil {
		t.Errorf("ownership 校验失败后安装 %s 未撤销", foreign)
	}
	if !tx.closed {
		t.Errorf("事务未被 Rollback")
	}
}

// TestBootstrapFailureLeavesConfigUntouched：bootstrap 失败时 .sm.json 保持原字节（单点写盘）。
func TestBootstrapFailureLeavesConfigUntouched(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("bar")
	cfgPath := filepath.Join(env.projectDir, ".sm.json")
	before := []byte("{\"curation\":{\"managed\":{\"claude\":[\"old\"]}}}")
	if err := os.WriteFile(cfgPath, before, 0644); err != nil {
		t.Fatal(err)
	}
	plan := &Plan{Project: env.projectDir, Baseline: &Baseline{}, Bootstrap: true, registryDir: env.registryDir}
	tx := newFakeTx(env, "bar")
	tx.stageErr = errors.New("stage boom")
	opts := ApplyOptions{
		ApplyRemovals:  true,
		ExplicitTarget: &ExplicitTarget{Skills: []string{"bar"}},
		BeginAdditions: func([]string) (AdditionTx, error) { return tx, nil },
	}
	if _, err := plan.Apply(opts); err == nil {
		t.Fatal("expected stage failure")
	}
	after, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf(".sm.json 不应被部分写入：before=%s after=%s", before, after)
	}
	barPath := filepath.Join(env.projectDir, tool.Claude.ProjectSkillDir, "bar")
	if _, statErr := os.Lstat(barPath); statErr == nil {
		t.Errorf("bootstrap 失败后 %s 未撤销", barPath)
	}
}

// TestApplyBootstrapMCPOnlyStartsTransaction：bootstrap 只有 MCP 没有技能时
// 仍必须开启安装事务，否则 Profile 的 MCP 漏装。
func TestApplyBootstrapMCPOnlyStartsTransaction(t *testing.T) {
	env := newTestEnv(t)
	plan := &Plan{Project: env.projectDir, Baseline: &Baseline{}, Bootstrap: true, registryDir: env.registryDir}
	begun := 0
	opts := ApplyOptions{
		ApplyRemovals:  true,
		ExplicitTarget: &ExplicitTarget{Profile: "mcp-only"},
		BeginAdditions: func(skills []string) (AdditionTx, error) {
			begun++
			tx := &fakeTx{cfg: &project.Config{}, saveDir: env.projectDir}
			return tx, nil
		},
	}
	if _, err := plan.Apply(opts); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if begun != 1 {
		t.Errorf("bootstrap（MCP-only）必须调用 BeginAdditions 一次，实际 %d", begun)
	}
}

// TestBootstrapBaselineKeepsTopLevelSkills：baseline.Skills 只写 --skill 额外项，
// profile 展开成员只用于本次安装（旧 recordBootstrapOwnership 语义）。
func TestBootstrapBaselineKeepsTopLevelSkills(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("foo")
	env.mkRegistrySkill("bar")
	if err := os.MkdirAll(env.profilesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.profilesDir, "web.json"), []byte(`{"skills":["foo"],"mcp":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	plan := &Plan{Project: env.projectDir, Baseline: &Baseline{}, Bootstrap: true, registryDir: env.registryDir}
	opts := ApplyOptions{
		ApplyRemovals: true,
		ExplicitTarget: &ExplicitTarget{
			Profile:        "web",
			Skills:         []string{"foo", "bar"},
			TopLevelSkills: []string{"bar"},
		},
		BeginAdditions: func(skills []string) (AdditionTx, error) {
			return newFakeTx(env, skills...), nil
		},
	}
	if _, err := plan.Apply(opts); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	cfg, err := project.NewManager(env.projectDir).Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Curation == nil || cfg.Curation.Baseline == nil {
		t.Fatal("baseline 未写入")
	}
	if cfg.Curation.Baseline.Profile != "web" {
		t.Errorf("baseline.Profile 应为 web，得到 %q", cfg.Curation.Baseline.Profile)
	}
	if len(cfg.Curation.Baseline.Skills) != 1 || cfg.Curation.Baseline.Skills[0] != "bar" {
		t.Errorf("baseline.Skills 只应含 --skill 额外项 [bar]，得到 %v", cfg.Curation.Baseline.Skills)
	}
	if len(cfg.Skills) != 1 || cfg.Skills[0] != "bar" {
		t.Errorf("顶层 skills 应为 [bar]，得到 %v", cfg.Skills)
	}
}

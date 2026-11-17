package curation

// fix-A2 的 RED 用例：W1 结果准确性、W2 no-op 不写配置、W4 仅移除保存失败恢复、
// bootstrap baseline 保持“Profile + 额外 skills”旧语义。

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/tool"
)

// TestFixA2RemovedClearedWhenRestored：移除后因 ownership/Commit 失败而恢复时，
// 结果不得声称已删除。
func TestFixA2RemovedClearedWhenRestored(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("bar")
	env.mkRegistrySkill("foo")
	env.symlinkAgentSkill("claude", "foo")
	env.writeConfig(&project.Config{Curation: &project.Curation{
		Baseline: &project.Baseline{Skills: []string{"bar"}},
		Managed:  map[string][]string{"claude": {"foo"}},
	}})
	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	opts := ApplyOptions{ApplyRemovals: true}
	opts.BeginAdditions = func(skills []string) (AdditionTx, error) {
		tx := newFakeTx(env, skills...)
		tx.stageErr = errors.New("stage boom")
		return tx, nil
	}
	res, applyErr := plan.Apply(opts)
	if applyErr == nil {
		t.Fatal("expected failure")
	}
	fooPath := filepath.Join(env.projectDir, tool.Claude.ProjectSkillDir, "foo")
	if _, statErr := os.Lstat(fooPath); statErr != nil {
		t.Fatalf("已删链接应被恢复: %v", statErr)
	}
	if len(res.Removed) != 0 {
		t.Errorf("链接已恢复时不得报告 Removed，得到 %v", res.Removed)
	}
}

// TestFixA2NoopApplyLeavesConfigBytes：无新增且无实际移除时不得重写 .sm.json。
func TestFixA2NoopApplyLeavesConfigBytes(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("foo")
	env.symlinkAgentSkill("claude", "foo")
	// 无 curation 块的旧配置：no-op 时不得凭空写入空 curation 块。
	cfgPath := filepath.Join(env.projectDir, ".sm.json")
	if err := os.WriteFile(cfgPath, []byte("{\n  \"skills\": [\n    \"foo\"\n  ]\n}"), 0644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	for _, applyRemovals := range []bool{false, true} {
		if _, err := plan.Apply(ApplyOptions{ApplyRemovals: applyRemovals}); err != nil {
			t.Fatalf("Apply(removals=%v): %v", applyRemovals, err)
		}
		after, err := os.ReadFile(cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(before) {
			t.Errorf("no-op apply(removals=%v) 不得改写 .sm.json:\nbefore=%s\nafter=%s", applyRemovals, before, after)
		}
	}
}

// TestFixA2OnlyRemovalSaveFailureRestoresLink：仅移除（无句柄）时末次保存失败必须恢复链接。
func TestFixA2OnlyRemovalSaveFailureRestoresLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX 权限语义")
	}
	if os.Geteuid() == 0 {
		t.Skip("root 不受目录权限限制")
	}
	env := newTestEnv(t)
	env.mkRegistrySkill("foo")
	env.symlinkAgentSkill("claude", "foo")
	env.writeConfig(&project.Config{Curation: &project.Curation{
		Managed: map[string][]string{"claude": {"foo"}},
	}})
	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	fooPath := filepath.Join(env.projectDir, tool.Claude.ProjectSkillDir, "foo")
	if err := os.Chmod(env.projectDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(env.projectDir, 0755) })

	_, applyErr := plan.Apply(ApplyOptions{ApplyRemovals: true})
	if applyErr == nil {
		t.Fatal("保存失败必须返回错误")
	}
	if _, statErr := os.Lstat(fooPath); statErr != nil {
		t.Errorf("保存失败后已删链接必须恢复: %v", statErr)
	}
}

// TestFixA2BootstrapBaselineKeepsTopLevelSkills：baseline.Skills 只写 --skill 额外项，
// profile 展开成员只用于本次安装（旧 recordBootstrapOwnership 语义）。
func TestFixA2BootstrapBaselineKeepsTopLevelSkills(t *testing.T) {
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

// TestFixA2CleanupCommittedReportsAddedAndRemoved：cleanup 已提交分支
// （内容已写盘，仅备份清理失败）必须同时报告已新增与已移除，且错误保留完整。
func TestFixA2CleanupCommittedReportsAddedAndRemoved(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("foo")
	env.mkRegistrySkill("bar")
	env.symlinkAgentSkill("claude", "foo")
	env.writeConfig(&project.Config{Curation: &project.Curation{
		Baseline: &project.Baseline{Skills: []string{"bar"}},
		Managed:  map[string][]string{"claude": {"foo"}},
	}})
	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	opts := ApplyOptions{ApplyRemovals: true}
	opts.BeginAdditions = func(skills []string) (AdditionTx, error) {
		tx := newFakeTx(env, skills...)
		return &cleanupTx{fakeTx: *tx, paths: []string{"/tmp/backup"}}, nil
	}
	res, applyErr := plan.Apply(opts)
	if applyErr == nil {
		t.Fatal("cleanup 失败必须返回错误")
	}
	if !strings.Contains(applyErr.Error(), "cleanup incomplete") {
		t.Errorf("错误应保留 cleanup 原文，得到 %v", applyErr)
	}
	if len(res.Added) != 1 || res.Added[0] != "bar" {
		t.Errorf("cleanup 已提交时应报告已新增 bar，得到 %v", res.Added)
	}
	if len(res.Removed) != 1 || res.Removed[0] != "claude/foo" {
		t.Errorf("cleanup 已提交时应报告已移除 claude/foo，得到 %v", res.Removed)
	}
	// 内容已提交：实体与 managed 都必须保持已落盘状态。
	if _, statErr := os.Lstat(filepath.Join(env.projectDir, tool.Claude.ProjectSkillDir, "foo")); !os.IsNotExist(statErr) {
		t.Errorf("cleanup 已提交后不得恢复已移除链接")
	}
	cfg, err := project.NewManager(env.projectDir).Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Curation == nil || !cfg.Curation.IsOwned("claude", "bar") {
		t.Errorf("cleanup 已提交后应保留新增 ownership，得到 %+v", cfg.Curation)
	}
}

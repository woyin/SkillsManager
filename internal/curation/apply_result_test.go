package curation

// 本文件固定 ApplyResult 的报告语义：只报实际落地实体，失败路径不报告
// 已撤销的动作，内容已提交的清理失败仍报告已落地结果。

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/tool"
)

// TestApplyReportsActualInstalls：ApplyResult.Added 只反映实际落地实体，不反映计划名。
func TestApplyReportsActualInstalls(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("bar")
	env.mkRegistrySkill("baz")
	env.writeConfig(&project.Config{Curation: &project.Curation{Baseline: &project.Baseline{Skills: []string{"bar"}}}})
	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	opts := ApplyOptions{ApplyRemovals: true}
	opts.BeginAdditions = func(skills []string) (AdditionTx, error) {
		// 句柄实际只落地 baz（bar 被 registry 侧替换/跳过）。
		return newFakeTx(env, "baz"), nil
	}
	res, err := plan.Apply(opts)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Added) != 1 || res.Added[0] != "baz" {
		t.Errorf("Added 应为实际落地 baz，得到 %v", res.Added)
	}
	cfg, err := project.NewManager(env.projectDir).Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.Curation.IsOwned("claude", "baz") || cfg.Curation.IsOwned("claude", "bar") {
		t.Errorf("ownership 应只含实际落地实体，得到 %+v", cfg.Curation.Managed)
	}
}

// TestApplyFailureReportsNoAdded：失败路径不得报告任何已新增实体。
func TestApplyFailureReportsNoAdded(t *testing.T) {
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
	if len(res.Added) != 0 {
		t.Errorf("失败时不得报告 Added，得到 %v", res.Added)
	}
}

// TestApplyStageFailureReportsNoRemoval：移除后因 ownership/Commit 失败而恢复时，
// 结果不得声称已删除。
func TestApplyStageFailureReportsNoRemoval(t *testing.T) {
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

// TestApplyCleanupCommittedReportsAddedAndRemoved：cleanup 已提交分支
// （内容已写盘，仅备份清理失败）必须同时报告已新增与已移除，且错误保留完整。
func TestApplyCleanupCommittedReportsAddedAndRemoved(t *testing.T) {
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

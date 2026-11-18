package curation

// 本文件固定 Apply 的事务语义：失败时逆序补偿（新增撤销、移除恢复）、
// rollback 错误必须可见、内容已提交后不得撤销、preflight 在副作用前拒绝、
// 以及“无可提交内容时不写盘”。ownership 登记见 apply_ownership_test.go，
// 结果报告见 apply_result_test.go，bootstrap 专用语义见 apply_bootstrap_test.go。

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/tool"
)

// TestApplyRemovalFailureUndoesAdditions：新增落地后移除阶段失败，本次新增必须被撤销。
func TestApplyRemovalFailureUndoesAdditions(t *testing.T) {
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

	fooPath := filepath.Join(env.projectDir, tool.Claude.ProjectSkillDir, "foo")
	opts := ApplyOptions{ApplyRemovals: true}
	opts.BeginAdditions = func(skills []string) (AdditionTx, error) {
		tx := newFakeTx(env, skills...)
		// 制造移除阶段失败：链接在 preflight 后消失。
		_ = os.Remove(fooPath)
		return tx, nil
	}

	if _, err := plan.Apply(opts); err == nil {
		t.Fatal("expected removal failure")
	}
	barPath := filepath.Join(env.projectDir, tool.Claude.ProjectSkillDir, "bar")
	if _, statErr := os.Lstat(barPath); statErr == nil {
		t.Errorf("新增 %s 未随移除失败撤销", barPath)
	}
}

// TestApplyStageFailureRestoresAdditionsAndRemovals：配置暂存失败时新增与移除都恢复。
func TestApplyStageFailureRestoresAdditionsAndRemovals(t *testing.T) {
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
		tx.stageErr = fmt.Errorf("boom staging")
		return tx, nil
	}
	if _, err := plan.Apply(opts); err == nil || !strings.Contains(err.Error(), "boom staging") {
		t.Fatalf("expected staging error, got %v", err)
	}
	barPath := filepath.Join(env.projectDir, tool.Claude.ProjectSkillDir, "bar")
	if _, statErr := os.Lstat(barPath); statErr == nil {
		t.Errorf("暂存失败后新增 %s 未撤销", barPath)
	}
	fooPath := filepath.Join(env.projectDir, tool.Claude.ProjectSkillDir, "foo")
	if _, statErr := os.Lstat(fooPath); statErr != nil {
		t.Errorf("暂存失败后已移除的 %s 未恢复: %v", fooPath, statErr)
	}
}

// TestApplyRollbackFailureIsReported：回滚自身失败必须出现在返回错误里。
func TestApplyRollbackFailureIsReported(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("bar")
	env.writeConfig(&project.Config{Curation: &project.Curation{Baseline: &project.Baseline{Skills: []string{"bar"}}}})
	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	opts := ApplyOptions{ApplyRemovals: true}
	opts.BeginAdditions = func(skills []string) (AdditionTx, error) {
		tx := newFakeTx(env, skills...)
		tx.commitErr = fmt.Errorf("boom commit")
		tx.rollbackErr = errors.New("rollback failed to restore")
		return tx, nil
	}
	_, applyErr := plan.Apply(opts)
	if applyErr == nil || !strings.Contains(applyErr.Error(), "rollback failed to restore") {
		t.Fatalf("expected visible rollback failure, got %v", applyErr)
	}
}

// TestApplyCleanupFailureKeepsRemovals：CommitCleanupError 表示内容已提交，绝不恢复已移除链接。
func TestApplyCleanupFailureKeepsRemovals(t *testing.T) {
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
	_, applyErr := plan.Apply(opts)
	if applyErr == nil {
		t.Fatal("expected cleanup error to surface")
	}
	fooPath := filepath.Join(env.projectDir, tool.Claude.ProjectSkillDir, "foo")
	if _, statErr := os.Lstat(fooPath); !os.IsNotExist(statErr) {
		t.Errorf("内容已提交后不得恢复已移除的 %s", fooPath)
	}
}

// TestApplyPreflightRejectsBeforeSideEffects：移除目标缺失时在安装前失败。
func TestApplyPreflightRejectsBeforeSideEffects(t *testing.T) {
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
	// 生成计划后删除待移除链接：preflight 必须在安装前拒绝。
	if err := os.Remove(filepath.Join(env.projectDir, tool.Claude.ProjectSkillDir, "foo")); err != nil {
		t.Fatal(err)
	}
	started := false
	opts := ApplyOptions{ApplyRemovals: true, BeginAdditions: func(skills []string) (AdditionTx, error) {
		started = true
		return newFakeTx(env, skills...), nil
	}}
	if _, err := plan.Apply(opts); err == nil {
		t.Fatal("目标缺失必须在副作用前失败")
	}
	if started {
		t.Error("preflight 失败时不得开始安装事务")
	}
}

// TestApplyBeginFailureIsZeroSideEffect：BeginAdditions 返回 (nil, error) 时
// 不产生任何副作用，错误原样上报。
func TestApplyBeginFailureIsZeroSideEffect(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("bar")
	env.writeConfig(&project.Config{Curation: &project.Curation{Baseline: &project.Baseline{Skills: []string{"bar"}}}})
	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	opts := ApplyOptions{ApplyRemovals: true, BeginAdditions: func([]string) (AdditionTx, error) {
		return nil, errors.New("begin boom")
	}}
	res, applyErr := plan.Apply(opts)
	if applyErr == nil {
		t.Fatal("expected begin failure")
	}
	if len(res.Added) != 0 || len(res.Removed) != 0 {
		t.Errorf("begin 失败不得产生结果: %+v", res)
	}
}

// TestApplyBeginReturnsTxWithErrorCompensates：若 BeginAdditions 违约地同时
// 返回句柄与错误，curation 必须补偿该句柄（真实 installer 返回 nil 事务，
// 此用例只固定契约，不替代真实路径验证）。
func TestApplyBeginReturnsTxWithErrorCompensates(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("bar")
	env.writeConfig(&project.Config{Curation: &project.Curation{Baseline: &project.Baseline{Skills: []string{"bar"}}}})
	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	tx := newFakeTx(env, "bar")
	opts := ApplyOptions{ApplyRemovals: true, BeginAdditions: func([]string) (AdditionTx, error) {
		return tx, errors.New("begin boom with handle")
	}}
	if _, err := plan.Apply(opts); err == nil {
		t.Fatal("expected begin failure")
	}
	if !tx.closed {
		t.Errorf("Begin 同时返回句柄与错误时必须补偿该句柄")
	}
	if _, statErr := os.Lstat(filepath.Join(env.projectDir, tool.Claude.ProjectSkillDir, "bar")); statErr == nil {
		t.Errorf("违约句柄补偿后不应残留实体")
	}
}

// TestApplyRemovalOnlySaveFailureRestoresLink：仅移除（无句柄）时末次保存失败必须恢复链接。
func TestApplyRemovalOnlySaveFailureRestoresLink(t *testing.T) {
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

// TestApplyNoopLeavesConfigBytes：无新增且无实际移除时不得重写 .sm.json。
func TestApplyNoopLeavesConfigBytes(t *testing.T) {
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

package curation

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/tool"
)

// cleanupTx 模拟“内容已提交但清理未完成”的句柄：Commit 置 committed 并返回错误。
type cleanupTx struct {
	fakeTx
	paths []string
}

// Commit 模拟“内容已写盘、仅备份清理失败”：先落盘暂存配置，再返回清理错误。
func (c *cleanupTx) Commit() error {
	if c.staged != nil && c.saveDir != "" {
		if err := project.NewManager(c.saveDir).Save(c.staged); err != nil {
			return err
		}
	}
	c.committed = true
	return &fakeCleanupError{paths: c.paths}
}

type fakeCleanupError struct{ paths []string }

func (e *fakeCleanupError) Error() string { return "cleanup incomplete: " + e.paths[0] }

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

// TestApplyOwnershipRejectsForeignPath：指向项目目录之外或非 registry 的实体不得登记 owned。
func TestApplyOwnershipRejectsForeignPath(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("bar")
	env.writeConfig(&project.Config{Curation: &project.Curation{Baseline: &project.Baseline{Skills: []string{"bar"}}}})
	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "bar")
	if err := os.Symlink(filepath.Join(env.registryDir, "skills", "global", "bar"), outside); err != nil {
		t.Fatal(err)
	}
	tx := &fakeTx{added: []AddedEntity{{Agent: tool.Claude.Name, Skill: "bar", Path: outside, Symlink: true}}, created: []string{outside}}
	opts := ApplyOptions{ApplyRemovals: true, BeginAdditions: func([]string) (AdditionTx, error) { return tx, nil }}
	if _, err := plan.Apply(opts); err == nil {
		t.Fatal("项目目录之外的路径不得登记 owned")
	}

	// 真身不在 registry 内（指向任意目录）同样拒绝。
	insideDir := filepath.Join(env.projectDir, tool.Claude.ProjectSkillDir)
	if err := os.MkdirAll(insideDir, 0755); err != nil {
		t.Fatal(err)
	}
	foreignTarget := t.TempDir()
	insidePath := filepath.Join(insideDir, "bar")
	if err := os.Symlink(foreignTarget, insidePath); err != nil {
		t.Fatal(err)
	}
	tx2 := &fakeTx{added: []AddedEntity{{Agent: tool.Claude.Name, Skill: "bar", Path: insidePath, Symlink: true}}, created: []string{insidePath}}
	opts2 := ApplyOptions{ApplyRemovals: true, BeginAdditions: func([]string) (AdditionTx, error) { return tx2, nil }}
	if _, err := plan.Apply(opts2); err == nil {
		t.Fatal("指向 registry 之外的链接不得登记 owned")
	}
	if err := os.Remove(insidePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Logf("cleanup: %v", err)
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

// TestApplyNoRemovalsKeepsManaged：ApplyRemovals=false 时不得改动 managed
// （链接仍在磁盘上，删除 ownership 记录会丢失所有权）。
func TestApplyNoRemovalsKeepsManaged(t *testing.T) {
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
	if _, err := plan.Apply(ApplyOptions{ApplyRemovals: false}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	cfg, err := project.NewManager(env.projectDir).Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Curation == nil || !cfg.Curation.IsOwned("claude", "foo") {
		t.Errorf("未移除链接时不得删除 managed 记录，得到 %+v", cfg.Curation)
	}
	if _, statErr := os.Lstat(filepath.Join(env.projectDir, tool.Claude.ProjectSkillDir, "foo")); statErr != nil {
		t.Errorf("链接应保留: %v", statErr)
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

// TestApplySharedDirClearsAllManagedAgents：共享物理路径移除后，所有共享代理的
// managed 记录都要清理，不能只删被去重保留的首个代理。
func TestApplySharedDirClearsAllManagedAgents(t *testing.T) {
	reg := filepath.Join(t.TempDir(), "registry")
	proj := t.TempDir()
	env := &testEnv{registryDir: reg, profilesDir: filepath.Join(t.TempDir(), "profiles"), projectDir: proj,
		tools: []tool.Tool{tool.Codex, tool.OpenCode, tool.Zed}}
	env.mkRegistrySkill("foo")
	shared := filepath.Join(proj, ".agents", "skills")
	if err := os.MkdirAll(shared, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(reg, "skills", "global", "foo"), filepath.Join(shared, "foo")); err != nil {
		t.Fatal(err)
	}
	env.writeConfig(&project.Config{Curation: &project.Curation{
		Managed: map[string][]string{"codex": {"foo"}, "opencode": {"foo"}, "zed": {"foo"}},
	}})
	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	if _, err := plan.Apply(ApplyOptions{ApplyRemovals: true}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	cfg, err := project.NewManager(proj).Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, agent := range []string{"codex", "opencode", "zed"} {
		if cfg.Curation != nil && cfg.Curation.IsOwned(agent, "foo") {
			t.Errorf("共享路径移除后 %s 的 managed 记录应清理，得到 %+v", agent, cfg.Curation.Managed)
		}
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

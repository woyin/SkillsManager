package curation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/tool"
)

// fakeTx 是 AdditionTx 的测试实现：只补偿自己创建的文件。
type fakeTx struct {
	added       []AddedEntity
	cfg         *project.Config
	staged      *project.Config
	created     []string
	committed   bool
	closed      bool
	commitErr   error
	stageErr    error
	rollbackErr error
	onRollback  func()
	saveDir     string
}

func (f *fakeTx) Added() []AddedEntity           { return f.added }
func (f *fakeTx) DesiredConfig() *project.Config { return f.cfg }
func (f *fakeTx) Committed() bool                { return f.committed }
func (f *fakeTx) StageProjectConfig(c *project.Config) error {
	if f.stageErr != nil {
		return f.stageErr
	}
	f.staged = c
	return nil
}
func (f *fakeTx) Commit() error {
	if f.commitErr != nil {
		return f.commitErr
	}
	if f.staged != nil && f.saveDir != "" {
		if err := project.NewManager(f.saveDir).Save(f.staged); err != nil {
			return err
		}
	}
	f.committed = true
	return nil
}
func (f *fakeTx) Rollback() error {
	if f.closed {
		return nil
	}
	f.closed = true
	if f.onRollback != nil {
		f.onRollback()
	}
	for i := len(f.created) - 1; i >= 0; i-- {
		_ = os.Remove(f.created[i])
	}
	return f.rollbackErr
}

func newFakeTx(env *testEnv, skills ...string) *fakeTx {
	linkHost := filepath.Join(env.projectDir, tool.Claude.ProjectSkillDir)
	tx := &fakeTx{cfg: &project.Config{}, saveDir: env.projectDir}
	for _, s := range skills {
		_ = os.MkdirAll(linkHost, 0755)
		path := filepath.Join(linkHost, s)
		if err := os.Symlink(filepath.Join(env.registryDir, "skills", "global", s), path); err != nil {
			panic(err)
		}
		tx.created = append(tx.created, path)
		tx.added = append(tx.added, AddedEntity{Agent: tool.Claude.Name, Skill: s, Path: path, Symlink: true})
	}
	return tx
}

// TestApplyAdditionRolledBackOnRemovalFailure：新增落地后移除阶段失败，本次新增必须被撤销。
func TestApplyAdditionRolledBackOnRemovalFailure(t *testing.T) {
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

// TestApplyAdditionRolledBackOnOwnershipFailure：配置暂存失败时新增与移除都恢复。
func TestApplyAdditionRolledBackOnOwnershipFailure(t *testing.T) {
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

// TestApplyDoesNotOwnExistingLink：只有 SymlinkMode 的 changed 实体才可登记 owned。
func TestApplyDoesNotOwnExistingLink(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("bar")
	env.writeConfig(&project.Config{Curation: &project.Curation{Baseline: &project.Baseline{Skills: []string{"bar"}}}})
	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	dir := filepath.Join(env.projectDir, tool.Claude.ProjectSkillDir)
	_ = os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, "bar")
	tx := &fakeTx{cfg: &project.Config{}, added: []AddedEntity{{Agent: tool.Claude.Name, Skill: "bar", Path: path, Symlink: false}}}
	opts := ApplyOptions{ApplyRemovals: true, BeginAdditions: func([]string) (AdditionTx, error) { return tx, nil }}
	if _, err := plan.Apply(opts); err == nil || !strings.Contains(err.Error(), "not a Link Install") {
		t.Fatalf("expected Link Install rejection, got %v", err)
	}
}

// TestApplyRollbackFailureIsVisible：回滚自身失败必须出现在返回错误里。
func TestApplyRollbackFailureIsVisible(t *testing.T) {
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

// TestApplySharedDirRemovesOnce：共享项目目录只移除一次。
func TestApplySharedDirRemovesOnce(t *testing.T) {
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
		Baseline: &project.Baseline{
			Skills: []string{"bar"},
		},
		Managed: map[string][]string{"codex": {"foo"}, "opencode": {"foo"}, "zed": {"foo"}},
	}})
	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	removes := 0
	for _, pr := range plan.Proposals {
		if pr.Action == ActionRemove {
			removes++
		}
	}
	if removes != 1 {
		t.Errorf("共享目录应只产生 1 条移除提案，得到 %d", removes)
	}
	res, err := plan.Apply(ApplyOptions{ApplyRemovals: true})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Removed) != 1 {
		t.Errorf("expected 1 removal, got %v", res.Removed)
	}
	if _, statErr := os.Lstat(filepath.Join(shared, "foo")); !os.IsNotExist(statErr) {
		t.Errorf("foo 应被移除, stat err=%v", statErr)
	}
}

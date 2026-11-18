package curation

// 本文件固定 ownership 登记与清理语义：只有本次 changed 的 Link Install 才可
// 登记 owned；路径必须在项目目录内且真身位于 registry；未移除链接时不得删除
// managed 记录；共享物理路径只产生一条移除提案，并清理所有共享代理的记录。
// 补偿与提交语义见 apply_transaction_test.go。

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/tool"
)

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

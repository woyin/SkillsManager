package installer

// transaction_test.go 覆盖 A1 事务句柄契约（plan v4『接口协议 v2』）。
// 落地路径统一由 TargetDirectories(inst.tools, projectDir, ProjectScope) 推出。

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/project"
)

// TestRegistryStrictPartialFailure：BeginRegistryInstall 全量预检，任一名字失败即零写入。
func TestRegistryStrictPartialFailure(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	inst, dirs := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)

	tx, beginErr := inst.BeginRegistryInstall([]string{"cf-skill", "nope"}, "")
	if beginErr == nil {
		if tx != nil {
			_ = tx.Rollback()
		}
		t.Fatal("expected strict preflight error for unknown skill name")
	}
	if tx != nil {
		t.Error("expected nil transaction on preflight failure")
	}
	assertNoSkillLink(t, dirs, "cf-skill")
}

// TestBeginInstallMergesMCPInMemoryOnly：Begin 阶段不写 .mcp.json/.sm.json，Commit 才写。
func TestBeginInstallMergesMCPInMemoryOnly(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	inst, _ := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)

	tx, err := inst.BeginInstall(projectDir, "", []string{"global-skill"}, []string{"test"})
	if err != nil {
		t.Fatalf("BeginInstall: %v", err)
	}
	mcpPath := filepath.Join(projectDir, ".mcp.json")
	if _, statErr := os.Stat(mcpPath); !os.IsNotExist(statErr) {
		t.Fatalf("BeginInstall must not write .mcp.json, got %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(projectDir, ".sm.json")); !os.IsNotExist(statErr) {
		t.Fatalf("BeginInstall must not write .sm.json, got %v", statErr)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	data, readErr := os.ReadFile(mcpPath)
	if readErr != nil {
		t.Fatalf("read .mcp.json after Commit: %v", readErr)
	}
	if !strings.Contains(string(data), "mcpServers") {
		t.Errorf(".mcp.json should contain merged servers, got %q", data)
	}
}

// TestChangedPlacementsReportAgentAndMode：ChangedPlacements 必须是实际 changed 的
// (agent, skill, path, mode)，供 curation 记 ownership。
func TestChangedPlacementsReportAgentAndMode(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	inst, dirs := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)

	tx, err := inst.BeginInstall(projectDir, "", []string{"global-skill"}, nil)
	if err != nil {
		t.Fatalf("BeginInstall: %v", err)
	}
	changes := tx.ChangedPlacements()
	if len(changes) != len(dirs) {
		t.Fatalf("expected %d changed placements (one per project target), got %d: %+v", len(dirs), len(changes), changes)
	}
	agents := map[string]bool{}
	destinations := map[string]bool{}
	for _, change := range changes {
		agents[change.Agent] = true
		destinations[change.Destination] = true
		if change.Skill != "global-skill" {
			t.Errorf("Skill = %q, want global-skill", change.Skill)
		}
		if change.ActualMode != SymlinkMode {
			t.Errorf("ActualMode = %v, want SymlinkMode", change.ActualMode)
		}
		if filepath.Base(change.Destination) != "global-skill" {
			t.Errorf("Destination = %q, want basename global-skill", change.Destination)
		}
		if change.Source == "" {
			t.Error("Source must be set for ownership validation")
		}
	}
	for _, dir := range dirs {
		if !destinations[filepath.Join(dir, "global-skill")] {
			t.Errorf("missing changed destination under %s: %+v", dir, changes)
		}
	}
	if !agents["alpha"] || !agents["beta"] {
		t.Errorf("expected alpha and beta agents, got %v", agents)
	}
	if got := tx.ChangedSkills(); len(got) != 1 || got[0] != "global-skill" {
		t.Errorf("ChangedSkills = %v, want [global-skill]", got)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	assertNoSkillLink(t, dirs, "global-skill")
}

// TestTransactionConfigFailureCompensates：skills 与 MCP 均已成功暂存后，
// Commit 步骤 2（.sm.json 末次写）失败必须补偿本次实体与已写文件。
func TestTransactionConfigFailureCompensates(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	inst, dirs := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)

	tx, err := inst.BeginInstall(projectDir, "", []string{"global-skill"}, []string{"test"})
	if err != nil {
		t.Fatalf("BeginInstall: %v", err)
	}
	// 前置：skills 与 MCP 暂存成功。
	if changes := tx.ChangedPlacements(); len(changes) != len(dirs) {
		t.Fatalf("expected staged changes before Commit, got %d", len(changes))
	}
	mcpPath := filepath.Join(projectDir, ".mcp.json")
	if _, statErr := os.Stat(mcpPath); !os.IsNotExist(statErr) {
		t.Fatalf(".mcp.json must not exist before Commit, got %v", statErr)
	}
	if stageErr := tx.StageProjectConfig(&project.Config{Skills: []string{"global-skill"}}); stageErr != nil {
		t.Fatalf("StageProjectConfig: %v", stageErr)
	}
	// Stage 成功后才让写入失败：目标变成目录，Commit 无法写出。
	if mkErr := os.Mkdir(filepath.Join(projectDir, ".sm.json"), 0755); mkErr != nil {
		t.Fatalf("prepare .sm.json dir: %v", mkErr)
	}

	commitErr := tx.Commit()
	if commitErr == nil {
		t.Fatal("expected commit error when .sm.json cannot be written")
	}
	var cleanupErr *CommitCleanupError
	if errors.As(commitErr, &cleanupErr) {
		t.Fatalf("config write failure must not be reported as cleanup error: %v", commitErr)
	}
	if !strings.Contains(commitErr.Error(), ".sm.json") {
		t.Errorf("commit error should name .sm.json, got: %v", commitErr)
	}
	// 步骤 1 已写 .mcp.json，补偿后必须恢复为“不存在”。
	if _, statErr := os.Stat(mcpPath); !os.IsNotExist(statErr) {
		t.Errorf(".mcp.json must be restored after commit compensation, got %v", statErr)
	}
	assertNoSkillLink(t, dirs, "global-skill")
	if rollbackErr := tx.Rollback(); rollbackErr != nil {
		t.Errorf("Rollback after internal compensation must be a no-op, got %v", rollbackErr)
	}
	if commitAgain := tx.Commit(); !errors.Is(commitAgain, ErrTransactionRolledBack) {
		t.Errorf("Commit after rollback = %v, want ErrTransactionRolledBack", commitAgain)
	}
}

// TestCommitMCPWriteFailureCompensates：Commit 步骤 1（单次 MCP 写）失败时
// 本次 skills 被撤销，且既有 .mcp.json 目标不被破坏。
func TestCommitMCPWriteFailureCompensates(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	inst, dirs := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)

	tx, err := inst.BeginInstall(projectDir, "", []string{"global-skill"}, []string{"test"})
	if err != nil {
		t.Fatalf("BeginInstall: %v", err)
	}
	mcpPath := filepath.Join(projectDir, ".mcp.json")
	if mkErr := os.Mkdir(mcpPath, 0755); mkErr != nil {
		t.Fatalf("occupy .mcp.json: %v", mkErr)
	}

	commitErr := tx.Commit()
	if commitErr == nil {
		t.Fatal("expected commit error when .mcp.json cannot be written")
	}
	var cleanupErr *CommitCleanupError
	if errors.As(commitErr, &cleanupErr) {
		t.Fatalf("MCP write failure must not be reported as cleanup error: %v", commitErr)
	}
	info, statErr := os.Stat(mcpPath)
	if statErr != nil || !info.IsDir() {
		t.Errorf("existing .mcp.json target must stay untouched: info=%v err=%v", info, statErr)
	}
	assertNoSkillLink(t, dirs, "global-skill")
	if rollbackErr := tx.Rollback(); rollbackErr != nil {
		t.Errorf("Rollback after internal compensation = %v, want nil", rollbackErr)
	}
}

// TestTransactionIdempotentTerminalStates：Commit/Rollback 幂等且终态可判定。
func TestTransactionIdempotentTerminalStates(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	inst, dirs := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)

	tx, err := inst.BeginRegistryInstall([]string{"cf-skill"}, "")
	if err != nil {
		t.Fatalf("BeginRegistryInstall: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Errorf("second Commit on committed tx = %v, want nil", err)
	}
	if rollbackErr := tx.Rollback(); !errors.Is(rollbackErr, ErrTransactionCommitted) {
		t.Errorf("Rollback on committed tx = %v, want ErrTransactionCommitted", rollbackErr)
	}
	for _, dir := range dirs {
		if _, statErr := os.Lstat(filepath.Join(dir, "cf-skill")); statErr != nil {
			t.Errorf("committed content must stay in %s: %v", dir, statErr)
		}
	}

	tx2, err := inst.BeginRegistryInstall([]string{"cf-skill"}, "")
	if err != nil {
		t.Fatalf("BeginRegistryInstall #2: %v", err)
	}
	if err := tx2.Rollback(); err != nil {
		t.Errorf("first Rollback = %v, want nil", err)
	}
	if err := tx2.Rollback(); err != nil {
		t.Errorf("second Rollback = %v, want nil (idempotent)", err)
	}
	if commitErr := tx2.Commit(); !errors.Is(commitErr, ErrTransactionRolledBack) {
		t.Errorf("Commit on rolled-back tx = %v, want ErrTransactionRolledBack", commitErr)
	}
}

// TestCommitCleanupFailurePreservesCommittedContent：清理失败 = 内容已提交，
// 必须返回 CommitCleanupError 且不撤销已安装内容。
func TestCommitCleanupFailurePreservesCommittedContent(t *testing.T) {
	requirePermissionSemantics(t)
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	inst, dirs := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)
	inst.SetCopyMode(true)

	// 预置既有实体，使 placement 产生备份目录。
	existing := filepath.Join(dirs[0], "cf-skill")
	if mkErr := os.MkdirAll(existing, 0755); mkErr != nil {
		t.Fatalf("prepare existing entity: %v", mkErr)
	}
	if wErr := os.WriteFile(filepath.Join(existing, "old.txt"), []byte("old"), 0644); wErr != nil {
		t.Fatalf("seed old content: %v", wErr)
	}

	tx, err := inst.BeginRegistryInstall([]string{"cf-skill"}, "")
	if err != nil {
		t.Fatalf("BeginRegistryInstall: %v", err)
	}
	backups, globErr := filepath.Glob(filepath.Join(dirs[0], ".sm-placement-backup-*"))
	if globErr != nil || len(backups) == 0 {
		t.Fatalf("expected placement backup dirs under %s, got %v (err=%v)", dirs[0], backups, globErr)
	}
	for _, dir := range backups {
		if chmodErr := os.Chmod(dir, 0555); chmodErr != nil {
			t.Fatalf("chmod backup dir: %v", chmodErr)
		}
		defer os.Chmod(dir, 0755)
	}

	commitErr := tx.Commit()
	if commitErr == nil {
		t.Fatal("expected cleanup error")
	}
	var cleanupErr *CommitCleanupError
	if !errors.As(commitErr, &cleanupErr) {
		t.Fatalf("Commit error = %v, want *CommitCleanupError", commitErr)
	}
	if len(cleanupErr.Paths) == 0 {
		t.Error("CommitCleanupError must report preserved backup paths")
	}
	if _, statErr := os.Stat(filepath.Join(existing, "SKILL.md")); statErr != nil {
		t.Errorf("committed content must stay in place, got %v", statErr)
	}
	if rollbackErr := tx.Rollback(); !errors.Is(rollbackErr, ErrTransactionCommitted) {
		t.Errorf("Rollback after cleanup failure = %v, want ErrTransactionCommitted", rollbackErr)
	}
}

// TestInstallKeepsExistingLinkOnFailure：本次失败不得删除既有幂等链接。
func TestInstallKeepsExistingLinkOnFailure(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	if err := os.WriteFile(filepath.Join(registryDir, "mcp", "broken.json"), []byte("{not json"), 0644); err != nil {
		t.Fatalf("write broken mcp: %v", err)
	}
	inst, dirs := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)

	existingLink := filepath.Join(dirs[0], "global-skill")
	if err := os.MkdirAll(dirs[0], 0755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	target := filepath.Join(registryDir, "skills", "global", "global-skill")
	if err := os.Symlink(target, existingLink); err != nil {
		t.Fatalf("seed existing link: %v", err)
	}

	if _, installErr := inst.Install(projectDir, "", []string{"global-skill"}, []string{"broken"}); installErr == nil {
		t.Fatal("expected install error for broken MCP")
	}
	got, readErr := os.Readlink(existingLink)
	if readErr != nil {
		t.Fatalf("existing link must survive failure, got %v", readErr)
	}
	if got != target {
		t.Errorf("existing link target = %q, want %q", got, target)
	}
}

// TestInstallFromRegistryPartialFailureKeepsCompat：普通入口保持 warning/continue
// 与 nil error 的部分失败契约。
func TestInstallFromRegistryPartialFailureKeepsCompat(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	inst, dirs := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)

	result, installErr := inst.InstallFromRegistry([]string{"cf-skill", "nope"}, "")
	if installErr != nil {
		t.Fatalf("legacy registry install must keep nil error on partial failure, got %v", installErr)
	}
	if result == nil || len(result.Skills) == 0 {
		t.Fatal("expected the resolvable skill to be installed")
	}
	if len(result.Failures) != 1 || result.Failures[0].Name != "nope" {
		t.Errorf("expected one recorded failure for nope, got %+v", result.Failures)
	}
	if _, statErr := os.Lstat(filepath.Join(dirs[0], "cf-skill")); statErr != nil {
		t.Errorf("cf-skill link missing under %s: %v", dirs[0], statErr)
	}
}

package installer

// installer_atomic_test.go 覆盖 A1 失败恢复场景（plan v4 测试名）。
// 所有落地路径都由 TargetDirectories(inst.tools, projectDir, ProjectScope) 推出，
// 避免固定 base 路径与真实项目 scope 目标不一致导致空断言。

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/tool"
)

// requirePermissionSemantics 统一权限类 fixture 的前置探针：root 或文件系统
// 不强制只读语义时跳过，避免假绿/假红。
func requirePermissionSemantics(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("permission semantics are not enforced for root")
	}
	probe := filepath.Join(t.TempDir(), "probe")
	if err := os.WriteFile(probe, []byte("x"), 0444); err != nil {
		t.Fatalf("prepare probe: %v", err)
	}
	if err := os.WriteFile(probe, []byte("y"), 0644); err == nil {
		t.Skip("filesystem does not enforce read-only file semantics")
	}
}

// projectScopeTools 使用相对 ProjectSkillDir，与真实工具定义一致，
// 使 ProjectScope 目标确定地落在 projectDir 之下。
func projectScopeTools() []tool.Tool {
	return []tool.Tool{
		{Name: "alpha", ProjectSkillDir: filepath.Join(".agents", "skills")},
		{Name: "beta", ProjectSkillDir: filepath.Join(".beta", "skills")},
	}
}

// newProjectScopeInstaller 返回项目 scope 安装器与它的实际目标目录列表。
func newProjectScopeInstaller(t *testing.T, registryDir, profilesDir, projectDir string) (*Installer, []string) {
	t.Helper()
	inst, err := New(registryDir, profilesDir, projectScopeTools())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inst.SetScope(projectDir, false)
	targets := TargetDirectories(inst.tools, projectDir, ProjectScope)
	dirs := make([]string, 0, len(targets))
	for _, target := range targets {
		dirs = append(dirs, target.Directory)
	}
	if len(dirs) == 0 {
		t.Fatal("fixture must resolve at least one project scope target")
	}
	return inst, dirs
}

// TestInstallRestoresMCPOnConfigFailure：skills 与 MCP 都已成功暂存，
// .sm.json 在 Commit 末次写失败时，本次链接与 .mcp.json 必须完全恢复。
func TestInstallRestoresMCPOnConfigFailure(t *testing.T) {
	requirePermissionSemantics(t)
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	inst, dirs := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)

	// 只读的既有 .sm.json：Stage 阶段可读快照，Commit 写入必然失败。
	smPath := filepath.Join(projectDir, ".sm.json")
	if err := os.WriteFile(smPath, []byte("{}"), 0444); err != nil {
		t.Fatalf("prepare read-only .sm.json: %v", err)
	}

	_, installErr := inst.Install(projectDir, "", []string{"global-skill"}, []string{"test"})
	if installErr == nil {
		t.Fatal("expected install error when .sm.json cannot be written")
	}
	if !strings.Contains(installErr.Error(), ".sm.json") {
		t.Errorf("failure must come from the .sm.json write, got: %v", installErr)
	}
	// 步骤 1 已写入 .mcp.json，补偿后必须不存在（原本不存在）。
	if _, statErr := os.Stat(filepath.Join(projectDir, ".mcp.json")); !os.IsNotExist(statErr) {
		t.Errorf(".mcp.json must be restored/absent after config failure, got %v", statErr)
	}
	assertNoSkillLink(t, dirs, "global-skill")
	assertNoSkillLink(t, []string{filepath.Join(projectDir, ".claude", "skills")}, "global-skill")
}

// TestInstallRestoresMCPOnLaterMCPMergeFailure：后续 MCP 合并失败时，
// 先前 MCP 的合并结果也不得落盘，本次 skills 必须撤销。
func TestInstallRestoresMCPOnLaterMCPMergeFailure(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	if err := os.WriteFile(filepath.Join(registryDir, "mcp", "broken.json"), []byte("{not json"), 0644); err != nil {
		t.Fatalf("write broken mcp: %v", err)
	}
	inst, dirs := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)

	if _, installErr := inst.Install(projectDir, "", []string{"global-skill"}, []string{"test", "broken"}); installErr == nil {
		t.Fatal("expected install error when a later MCP merge fails")
	}
	if _, statErr := os.Stat(filepath.Join(projectDir, ".mcp.json")); !os.IsNotExist(statErr) {
		t.Errorf(".mcp.json must not exist after failed MCP merge, got %v", statErr)
	}
	assertNoSkillLink(t, dirs, "global-skill")
}

// TestInstallFailsOnUnparsableMCPAndKeepsBytes：既有 .mcp.json 非法 JSON 时
// 必须失败且原字节不变（不得用空 map 覆盖）。
func TestInstallFailsOnUnparsableMCPAndKeepsBytes(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	mcpPath := filepath.Join(projectDir, ".mcp.json")
	original := []byte("{\n  \"mcpServers\": {oops\n")
	if err := os.WriteFile(mcpPath, original, 0644); err != nil {
		t.Fatalf("prepare .mcp.json: %v", err)
	}
	inst, dirs := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)

	_, installErr := inst.Install(projectDir, "", []string{"global-skill"}, []string{"test"})
	if installErr == nil {
		t.Fatal("expected install error for unparsable existing .mcp.json")
	}
	if !strings.Contains(installErr.Error(), ".mcp.json") {
		t.Errorf("error should name .mcp.json, got: %v", installErr)
	}
	after, readErr := os.ReadFile(mcpPath)
	if readErr != nil {
		t.Fatalf("read .mcp.json: %v", readErr)
	}
	if string(after) != string(original) {
		t.Errorf(".mcp.json bytes changed: got %q want %q", after, original)
	}
	assertNoSkillLink(t, dirs, "global-skill")
}

// TestProfileCategoryMemberFailureAborts：category 安装成员失败必须让整体失败，
// 不是 warning 跳过（ADR 0012）。
func TestProfileCategoryMemberFailureAborts(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	inst, dirs := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)

	// 在第二个目标目录占用成员名，使 placement 必然失败。
	occupied := filepath.Join(dirs[len(dirs)-1], "cf-skill")
	if err := os.MkdirAll(filepath.Dir(occupied), 0755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	if err := os.WriteFile(occupied, []byte("occupied"), 0644); err != nil {
		t.Fatalf("occupy destination: %v", err)
	}

	if _, installErr := inst.Install(projectDir, "", []string{"cloudflare"}, nil); installErr == nil {
		t.Fatal("expected install error when a category member cannot be placed")
	}
	// 回滚必须撤掉先成功目标的本次链接，而先存在的占位文件保持原样。
	if _, statErr := os.Lstat(filepath.Join(dirs[0], "cf-skill")); !os.IsNotExist(statErr) {
		t.Errorf("first target must not keep a link after rollback (err=%v)", statErr)
	}
	info, statErr := os.Lstat(occupied)
	if statErr != nil {
		t.Fatalf("occupied placeholder must survive: %v", statErr)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("occupied placeholder must not be replaced by a symlink")
	}
}

// TestInstallJoinsStageFailureWithRollback：StageProjectConfig 失败发生在
// skills 已落地之后，Install 必须撤销本次实体并把回滚错误一起上报
// （不得忽略 tx.Rollback() 的错误）。
func TestInstallJoinsStageFailureWithRollback(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	inst, dirs := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)

	// .sm.json 是目录：Stage 的写前快照读取失败，此时 links 已落地。
	if mkErr := os.Mkdir(filepath.Join(projectDir, ".sm.json"), 0755); mkErr != nil {
		t.Fatalf("prepare .sm.json dir: %v", mkErr)
	}

	_, installErr := inst.Install(projectDir, "", []string{"global-skill"}, nil)
	if installErr == nil {
		t.Fatal("expected install error when staging .sm.json fails")
	}
	if !strings.Contains(installErr.Error(), "staging .sm.json") {
		t.Errorf("error must name the staging step, got: %v", installErr)
	}
	// 回滚必须真的执行：本次链接全部消失。
	assertNoSkillLink(t, dirs, "global-skill")
}

// TestInstallRestoresExistingMCPBytesAndMode：既有 .mcp.json（内容 + 0600）在
// 末次配置写失败后，字节与权限都必须恢复。
func TestInstallRestoresExistingMCPBytesAndMode(t *testing.T) {
	requirePermissionSemantics(t)
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	inst, dirs := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)

	mcpPath := filepath.Join(projectDir, ".mcp.json")
	original := []byte("{\n  \"mcpServers\": {\n    \"keep\": {\"command\": \"keep\"}\n  }\n}")
	if err := os.WriteFile(mcpPath, original, 0600); err != nil {
		t.Fatalf("seed .mcp.json: %v", err)
	}
	if err := os.Chmod(mcpPath, 0600); err != nil {
		t.Fatalf("chmod .mcp.json: %v", err)
	}
	// 只读 .sm.json：Stage 快照可读，Commit 写入被拒。
	if err := os.WriteFile(filepath.Join(projectDir, ".sm.json"), []byte("{}"), 0444); err != nil {
		t.Fatalf("seed read-only .sm.json: %v", err)
	}

	_, installErr := inst.Install(projectDir, "", []string{"global-skill"}, []string{"test"})
	if installErr == nil {
		t.Fatal("expected install error when .sm.json cannot be written")
	}
	after, readErr := os.ReadFile(mcpPath)
	if readErr != nil {
		t.Fatalf("read .mcp.json: %v", readErr)
	}
	if !bytes.Equal(after, original) {
		t.Errorf(".mcp.json bytes must be restored, got %q want %q", after, original)
	}
	info, statErr := os.Stat(mcpPath)
	if statErr != nil {
		t.Fatalf("Stat .mcp.json: %v", statErr)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf(".mcp.json mode = %04o, want 0600", perm)
	}
	assertNoSkillLink(t, dirs, "global-skill")
}

// TestRollbackReportsFailure：恢复旧实体失败时必须保留备份并给出路径，
// 不得删除唯一恢复副本。
func TestRollbackReportsFailure(t *testing.T) {
	requirePermissionSemantics(t)
	base := t.TempDir()
	src := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("# new"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	skillsDir := filepath.Join(base, "skills")
	dest := filepath.Join(skillsDir, "demo")
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dest, "old.txt"), []byte("old"), 0644); err != nil {
		t.Fatalf("seed old content: %v", err)
	}

	placer := NewPlacement(PlacementOptions{Mode: SymlinkMode, Conflict: ReplaceOnConflict, RejectOverlap: true})
	result, err := placer.Place(src, dest)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected a changed placement")
	}
	backupDir := result.state.backupDir
	if backupDir == "" {
		t.Fatal("expected a placement backup for replaced destination")
	}

	if chmodErr := os.Chmod(skillsDir, 0555); chmodErr != nil {
		t.Fatalf("chmod skills dir: %v", chmodErr)
	}
	defer os.Chmod(skillsDir, 0755)

	rollbackErr := result.Rollback()
	if rollbackErr == nil {
		t.Fatal("expected rollback error when the destination cannot be removed")
	}
	if !strings.Contains(rollbackErr.Error(), "previous content preserved at") &&
		!strings.Contains(rollbackErr.Error(), "removing replacement") {
		t.Errorf("rollback error should expose the cause and preserved path, got: %v", rollbackErr)
	}
	if _, statErr := os.Stat(filepath.Join(backupDir, "demo", "old.txt")); statErr != nil {
		t.Fatalf("previous content must be preserved in backup dir: %v", statErr)
	}
}

func assertNoSkillLink(t *testing.T, dirs []string, name string) {
	t.Helper()
	for _, dir := range dirs {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("skill link %s must not exist in %s (err=%v)", name, dir, err)
		}
	}
}

package installer

// transaction_symlink_test.go 覆盖悬空 symlink 的写入与补偿语义：
// .mcp.json / .sm.json 是悬空链接时，写入应创建 referent 并保留链接；
// Commit 后续失败时只删除本次创建的 referent，原链接保持悬空。
// 本文件先于实现加入，用于证明真实失败。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/project"
)

// danglingMCPLink 在 projectDir 下建立 .mcp.json -> <referentDir>/servers.json
// 的悬空链接，返回 (link, referent)。
func danglingMCPLink(t *testing.T, projectDir string) (link, referent string) {
	t.Helper()
	referentDir := filepath.Join(projectDir, "mcp-target")
	if err := os.MkdirAll(referentDir, 0755); err != nil {
		t.Fatalf("mkdir referent dir: %v", err)
	}
	referent = filepath.Join(referentDir, "servers.json")
	link = filepath.Join(projectDir, ".mcp.json")
	if err := os.Symlink(referent, link); err != nil {
		t.Fatalf("symlink .mcp.json: %v", err)
	}
	return link, referent
}

// TestCommitWritesMCPThroughDanglingSymlink：MCP 通过既有悬空链接写入时，
// referent 被创建、链接保留。
func TestCommitWritesMCPThroughDanglingSymlink(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	inst, _ := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)
	link, referent := danglingMCPLink(t, projectDir)

	tx, err := inst.BeginInstall(projectDir, "", []string{"global-skill"}, []string{"test"})
	if err != nil {
		t.Fatalf("BeginInstall: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit through dangling .mcp.json symlink: %v", err)
	}
	info, statErr := os.Lstat(link)
	if statErr != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("original .mcp.json symlink must be preserved: info=%v err=%v", info, statErr)
	}
	data, readErr := os.ReadFile(referent)
	if readErr != nil {
		t.Fatalf("referent must be created: %v", readErr)
	}
	if !strings.Contains(string(data), "mcpServers") {
		t.Errorf("referent content = %q, want merged mcpServers", data)
	}
}

// TestCommitWritesConfigThroughDanglingSymlink：.sm.json 通过既有悬空链接写入时，
// referent 被创建、链接保留。
func TestCommitWritesConfigThroughDanglingSymlink(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	inst, _ := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)
	referentDir := filepath.Join(projectDir, "sm-target")
	if err := os.MkdirAll(referentDir, 0755); err != nil {
		t.Fatalf("mkdir referent dir: %v", err)
	}
	referent := filepath.Join(referentDir, "config.json")
	link := filepath.Join(projectDir, ".sm.json")
	if err := os.Symlink(referent, link); err != nil {
		t.Fatalf("symlink .sm.json: %v", err)
	}

	tx, err := inst.BeginInstall(projectDir, "", []string{"global-skill"}, nil)
	if err != nil {
		t.Fatalf("BeginInstall: %v", err)
	}
	if stageErr := tx.StageProjectConfig(&project.Config{Skills: []string{"global-skill"}}); stageErr != nil {
		t.Fatalf("StageProjectConfig through dangling symlink: %v", stageErr)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit through dangling .sm.json symlink: %v", err)
	}
	info, statErr := os.Lstat(link)
	if statErr != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("original .sm.json symlink must be preserved: info=%v err=%v", info, statErr)
	}
	if _, readErr := os.Stat(referent); readErr != nil {
		t.Fatalf("referent must be created: %v", readErr)
	}
}

// TestCommitConfigFailureRestoresDanglingMCPLink：MCP 通过悬空链接写成功后，
// .sm.json 末次写失败 → 只删除本次创建的 referent，原悬空链接保留。
func TestCommitConfigFailureRestoresDanglingMCPLink(t *testing.T) {
	requirePermissionSemantics(t)
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	inst, dirs := newProjectScopeInstaller(t, registryDir, profilesDir, projectDir)
	link, referent := danglingMCPLink(t, projectDir)
	// 只读 .sm.json：Stage 快照可读，Commit 步骤 2 失败。
	if err := os.WriteFile(filepath.Join(projectDir, ".sm.json"), []byte("{}"), 0444); err != nil {
		t.Fatalf("seed read-only .sm.json: %v", err)
	}

	_, installErr := inst.Install(projectDir, "", []string{"global-skill"}, []string{"test"})
	if installErr == nil {
		t.Fatal("expected install error when .sm.json cannot be written")
	}
	if !strings.Contains(installErr.Error(), ".sm.json") {
		t.Fatalf("failure must come from the .sm.json write (MCP write must succeed through the dangling link), got: %v", installErr)
	}
	if _, statErr := os.Stat(referent); !os.IsNotExist(statErr) {
		t.Errorf("referent created by this transaction must be removed, got %v", statErr)
	}
	target, readErr := os.Readlink(link)
	if readErr != nil {
		t.Fatalf("original .mcp.json symlink must be preserved, got %v", readErr)
	}
	if target != referent {
		t.Errorf("link target = %q, want %q", target, referent)
	}
	assertNoSkillLink(t, dirs, "global-skill")
}

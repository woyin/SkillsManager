package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

// countProjectIndexReads 安装计数 seam：返回读取项目索引的累计次数。
// projectIndexReadHook 只在 knownProjectPaths 内触发，cleanup 恢复原值。
func countProjectIndexReads(t *testing.T) func() int {
	t.Helper()
	old := projectIndexReadHook
	reads := 0
	projectIndexReadHook = func() { reads++ }
	t.Cleanup(func() { projectIndexReadHook = old })
	return func() int { return reads }
}

// registerRmIndexSkill 在临时 Registry 注册一件技能，返回其原件路径。
func registerRmIndexSkill(t *testing.T, regDir, name string) string {
	t.Helper()
	src := t.TempDir()
	makeValidSkillDir(t, src, name)
	reg := registry.New(regDir)
	res, err := reg.Register(filepath.Join(src, name), "",
		registry.SkillOrigin{SourceKind: registry.SourceLocalSnapshot, Source: src}, false)
	if err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
	return res.Path
}

// linkRmInstall 在 projectDir 的 Claude 项目技能目录建一条同名安装链接。
func linkRmInstall(t *testing.T, projectDir, name, original string) string {
	t.Helper()
	dir := filepath.Join(projectDir, tool.Claude.ProjectSkillDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, name)
	if err := os.Symlink(original, link); err != nil {
		t.Fatal(err)
	}
	return link
}

// TestRmForceReadsProjectIndexOnce 验证 --force 删除流程只读取一次项目索引：
// 损坏索引只产生一条“引用可能不完整”告警，而不是每个阶段的重复告警。
func TestRmForceReadsProjectIndexOnce(t *testing.T) {
	setupRmGlobals(t)
	oldForce := rmForce
	t.Cleanup(func() { rmForce = oldForce })
	setTestHome(t, t.TempDir())
	regDir := withTestRegistry(t)

	oldData := DataDir
	DataDir = t.TempDir()
	t.Cleanup(func() { DataDir = oldData })

	projectDir := t.TempDir()
	rmProject, rmDir, rmForce = false, projectDir, true

	original := registerRmIndexSkill(t, regDir, "once-index")
	linkRmInstall(t, projectDir, "once-index", original)

	if err := os.WriteFile(dbPath(), []byte("not a sqlite database"), 0644); err != nil {
		t.Fatal(err)
	}

	reads := countProjectIndexReads(t)
	var rmErr error
	out := captureRmStderr(t, func() { rmErr = removeRegistryOriginal("once-index") })

	if rmErr != nil {
		t.Fatalf("force remove with corrupt index: %v", rmErr)
	}
	if got := reads(); got != 1 {
		t.Fatalf("knownProjectPaths reads = %d, want 1", got)
	}
	if got := strings.Count(out, "reference list may be incomplete"); got != 1 {
		t.Fatalf("incomplete-reference warnings = %d, want 1; stderr=%q", got, out)
	}
	// 索引不可用不改变删除策略：原件仍被删除。
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("registry original should be removed: %v", err)
	}
}

// TestRmForceReusesIndexForCleanupAndReport 验证可访问项目的清理与不可访问历史
// 项目的报告共用同一份项目索引：只读一次，且两种效果都成立。
func TestRmForceReusesIndexForCleanupAndReport(t *testing.T) {
	setupRmGlobals(t)
	oldForce := rmForce
	t.Cleanup(func() { rmForce = oldForce })
	setTestHome(t, t.TempDir())
	regDir := withTestRegistry(t)

	dataDir := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	oldData := DataDir
	DataDir = dataDir
	t.Cleanup(func() { DataDir = oldData })

	// 当前项目目录不放安装：清理与报告都只能依赖索引中的已知项目。
	rmProject, rmDir, rmForce = false, t.TempDir(), true

	original := registerRmIndexSkill(t, regDir, "reuse-index")
	accessible := t.TempDir()
	link := linkRmInstall(t, accessible, "reuse-index", original)
	gone := filepath.Join(t.TempDir(), "gone-project")

	db, err := openDB()
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	if err := db.UpsertProject(accessible, "dev", []string{"reuse-index"}, nil); err != nil {
		t.Fatalf("UpsertProject accessible: %v", err)
	}
	if err := db.UpsertProject(gone, "dev", []string{"reuse-index"}, nil); err != nil {
		t.Fatalf("UpsertProject gone: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reads := countProjectIndexReads(t)
	var rmErr error
	out := captureRmStderr(t, func() { rmErr = removeRegistryOriginal("reuse-index") })

	if rmErr != nil {
		t.Fatalf("force remove: %v", rmErr)
	}
	if got := reads(); got != 1 {
		t.Fatalf("knownProjectPaths reads = %d, want 1", got)
	}
	// 清理使用了同一份索引：索引中的可访问项目安装被删除。
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("known project install should be cleaned: %v", err)
	}
	// 报告使用了同一份索引：不可访问历史项目被报告。
	if !strings.Contains(out, "inaccessible") || !strings.Contains(out, "gone-project") {
		t.Fatalf("expected inaccessible historical project report, got %q", out)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("registry original should be removed: %v", err)
	}
}

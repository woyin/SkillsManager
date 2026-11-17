package lifecycle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/tool"
)

func claudeTool() tool.Tool {
	return tool.Tool{Name: "claude", SkillDir: ".claude/skills", ProjectSkillDir: ".claude/skills"}
}

// TestScanDirsGlobalAndProject 验证扫描目录显式范围：可选全局 + 项目级。
func TestScanDirsGlobalAndProject(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	project := t.TempDir()

	got := ScanDirs(claudeTool(), ScanOptions{ProjectDir: project, IncludeGlobal: true})
	want := []string{filepath.Join(tmpHome, ".claude", "skills"), filepath.Join(project, ".claude", "skills")}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("ScanDirs = %v, want %v", got, want)
	}
	if got := ScanDirs(claudeTool(), ScanOptions{ProjectDir: project}); len(got) != 1 || got[0] != want[1] {
		t.Fatalf("project-only ScanDirs = %v", got)
	}
}

// TestCollectReferencesFindsGlobalAndProjectLinks 验证同名链接在全局与项目
// 目录都被收集，且健康扫描不报错。
func TestCollectReferencesFindsGlobalAndProjectLinks(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	project := t.TempDir()
	skillPath := filepath.Join(t.TempDir(), "alpha")
	if err := os.MkdirAll(skillPath, 0755); err != nil {
		t.Fatal(err)
	}
	globalDir := filepath.Join(tmpHome, ".claude", "skills")
	projectDir := filepath.Join(project, ".claude", "skills")
	for _, dir := range []string{globalDir, projectDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(skillPath, filepath.Join(dir, "alpha")); err != nil {
			t.Fatal(err)
		}
	}

	set, errs := CollectReferences("alpha", skillPath, CollectOptions{
		ProjectDir: project,
		Tools:      []tool.Tool{claudeTool()},
	})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(set.Paths) != 2 {
		t.Fatalf("refs = %v, want global+project", set.Paths)
	}
}

// TestCollectReferencesReportsInaccessibleProject 验证不可访问项目返回错误
// 并记入 Inaccessible，不能当成“无引用”。
func TestCollectReferencesReportsInaccessibleProject(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(blocker, "project")

	set, errs := CollectReferences("alpha", "", CollectOptions{
		ProjectDir:    t.TempDir(),
		Tools:         []tool.Tool{claudeTool()},
		KnownProjects: []string{project},
	})
	if len(errs) == 0 {
		t.Fatalf("expected scan errors for inaccessible project, set = %+v", set)
	}
	if len(set.Inaccessible) == 0 {
		t.Fatalf("expected inaccessible paths, got %+v", set)
	}
}

// TestCleanupInstallsReportsInaccessibleRemoval 验证删除失败被记录，
// 不静默丢弃且 Removed 计数准确。
func TestCleanupInstallsReportsInaccessibleRemoval(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	project := t.TempDir()
	projectSkills := filepath.Join(project, ".claude", "skills")
	if err := os.MkdirAll(projectSkills, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "alpha")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(projectSkills, "alpha")); err != nil {
		t.Fatal(err)
	}

	// 探针：只读目录是否真的阻止删除；root/非 POSIX 权限则跳过。
	if err := os.Chmod(projectSkills, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(projectSkills, 0755) })
	probe := filepath.Join(projectSkills, "probe")
	if err := os.WriteFile(probe, []byte("x"), 0644); err == nil {
		_ = os.Remove(probe)
		t.Skip("directory stays writable (root or non-POSIX permissions)")
	}

	res := CleanupInstalls("alpha", CleanupOptions{
		ProjectDir: project,
		Tools:      []tool.Tool{claudeTool()},
	})
	if res.Removed != 0 {
		t.Fatalf("Removed = %d, want 0", res.Removed)
	}
	if len(res.Failures) == 0 && len(res.Inaccessible) == 0 {
		t.Fatal("cleanup failure must be observable")
	}
}

// TestCleanupInstallsGlobalWithEmptyProjectDir 验证 rm --dir 场景（无项目路径）
// 仍清理全局安装，与迁移前语义一致。
func TestCleanupInstallsGlobalWithEmptyProjectDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	globalDir := filepath.Join(tmpHome, ".claude", "skills")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "alpha")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(globalDir, "alpha")); err != nil {
		t.Fatal(err)
	}

	res := CleanupInstalls("alpha", CleanupOptions{
		IncludeGlobal: true,
		Tools:         []tool.Tool{claudeTool()},
	})
	if res.Removed != 1 {
		t.Fatalf("Removed = %d, want 1 (global cleanup must still happen when ProjectDir is empty)", res.Removed)
	}
	if _, err := os.Lstat(filepath.Join(globalDir, "alpha")); !os.IsNotExist(err) {
		t.Fatalf("global install still present: %v", err)
	}
}

// TestRemoveLockEntryReportsCorruptLock 验证坏锁文件错误被返回。
func TestRemoveLockEntryReportsCorruptLock(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "skills-lock.json"), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RemoveLockEntry(project, "alpha"); err == nil {
		t.Fatal("expected an error for a corrupt skills-lock.json")
	}
}

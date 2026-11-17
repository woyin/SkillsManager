package health

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestScanSharedFixture 用一份共享目录 fixture 锁定四类判定：
// broken、outside（orphaned）、inside（健康）、普通目录（忽略），并验证多个
// Agent 共享同一目录时不会重复计数。
func TestScanSharedFixture(t *testing.T) {
	root := t.TempDir()
	registryDir := filepath.Join(root, "registry")
	managed := filepath.Join(registryDir, "skills", "global", "managed")
	if err := os.MkdirAll(managed, 0755); err != nil {
		t.Fatal(err)
	}

	shared := filepath.Join(root, "home", ".agents", "skills")
	if err := os.MkdirAll(shared, 0755); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(shared, "broken")
	if err := os.Symlink(filepath.Join(root, "missing-target"), broken); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(root, "external")
	if err := os.MkdirAll(external, 0755); err != nil {
		t.Fatal(err)
	}
	orphaned := filepath.Join(shared, "orphaned")
	if err := os.Symlink(external, orphaned); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(shared, "managed")
	if err := os.Symlink(managed, inside); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(shared, "plain-dir"), 0755); err != nil {
		t.Fatal(err)
	}

	report, err := Scan([]string{shared, shared, filepath.Join(root, "absent-dir")}, registryDir, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	want := []Issue{
		{Type: IssueBrokenSymlink, Path: broken},
		{Type: IssueOrphanedSymlink, Path: orphaned},
	}
	if !reflect.DeepEqual(report.Issues, want) {
		t.Fatalf("issues = %+v, want %+v", report.Issues, want)
	}
}

// TestScanReportsInaccessibleTargetAsError 验证链接目标不可访问（EACCES）时
// Scan 返回错误，而不是把它当成 broken——否则 --fix 会删掉仍有效的链接。
func TestScanReportsInaccessibleTargetAsError(t *testing.T) {
	root := t.TempDir()
	scanned := filepath.Join(root, "skills")
	if err := os.MkdirAll(scanned, 0755); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(root, "locked")
	if err := os.MkdirAll(filepath.Join(locked, "skill"), 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(scanned, "locked-link")
	if err := os.Symlink(filepath.Join(locked, "skill"), link); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0755) })
	if _, err := os.Stat(link); err == nil {
		t.Skip("target stays statable (root or non-POSIX permissions)")
	}

	if _, err := Scan([]string{scanned}, filepath.Join(root, "registry"), nil); err == nil {
		t.Fatal("Scan must report an IO error when the link target is inaccessible")
	}
}

// TestScanReportsSymlinkLoopAsError 验证循环链接（ELOOP）返回错误，
// 而不是被归类为 broken。
func TestScanReportsSymlinkLoopAsError(t *testing.T) {
	root := t.TempDir()
	scanned := filepath.Join(root, "skills")
	if err := os.MkdirAll(scanned, 0755); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(scanned, "a")
	b := filepath.Join(scanned, "b")
	if err := os.Symlink("b", a); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a", b); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a); err == nil {
		t.Skip("platform resolves symlink loops")
	}

	if _, err := Scan([]string{scanned}, filepath.Join(root, "registry"), nil); err == nil {
		t.Fatal("Scan must report an IO error for a symlink loop")
	}
}

// TestScanFailsOnNonDirectoryPath 验证“技能目录”实际是文件时报 IO 错误，
// 而不是静默返回健康。
func TestScanFailsOnNonDirectoryPath(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(filepath.Dir(blocker), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocker, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := Scan([]string{blocker}, filepath.Join(root, "registry"), nil); err == nil {
		t.Fatal("Scan must report an IO error when the skill directory is a file")
	}
}

// TestScanReportsMissingProject 验证数据库记录的项目目录缺失被分类为
// missing_project，而不是 IO 错误。
func TestScanReportsMissingProject(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "missing-project")

	report, err := Scan(nil, filepath.Join(root, "registry"), []string{missing, missing})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	want := []Issue{{Type: IssueMissingProject, Path: missing}}
	if !reflect.DeepEqual(report.Issues, want) {
		t.Fatalf("issues = %+v, want %+v", report.Issues, want)
	}
}

// TestScanReportsProjectStatError 验证项目路径不可访问时返回错误，
// 不能把“读不到”当成“项目仍在”或“项目缺失”。用普通文件当父路径触发
// ENOTDIR，避免依赖权限位（root/Windows 下 chmod 语义不同）。
func TestScanReportsProjectStatError(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}

	project := filepath.Join(blocker, "project")
	if _, err := Scan(nil, filepath.Join(root, "registry"), []string{project}); err == nil {
		t.Fatal("Scan must report a Stat error for an inaccessible project path")
	}
}

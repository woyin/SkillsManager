package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
	"github.com/woyin/skills-manager/web"
)

func TestCheckSymlinksReportsAndFixesBrokenAndOrphanedLinks(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	oldRegistry, oldFix := RegistryDir, checkFix
	RegistryDir, checkFix = t.TempDir(), false
	t.Cleanup(func() { RegistryDir, checkFix = oldRegistry, oldFix })

	dir := filepath.Join(tmpHome, tool.Claude.SkillDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), filepath.Join(dir, "broken")); err != nil {
		t.Fatal(err)
	}
	orphanTarget := filepath.Join(t.TempDir(), "external-skill")
	if err := os.MkdirAll(orphanTarget, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(orphanTarget, filepath.Join(dir, "orphan")); err != nil {
		t.Fatal(err)
	}

	if issues, err := checkSymlinks(); err != nil || issues != 2 {
		t.Fatalf("checkSymlinks = %d, %v", issues, err)
	}
	checkFix = true
	if issues, err := checkSymlinks(); err != nil || issues != 2 {
		t.Fatalf("fixed checkSymlinks = %d, %v", issues, err)
	}
	for _, name := range []string{"broken", "orphan"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s should be removed: %v", name, err)
		}
	}
}

func TestCheckProjectRecordsReportsAndFixesMissingProject(t *testing.T) {
	oldData, oldFix := DataDir, checkFix
	DataDir, checkFix = t.TempDir(), false
	t.Cleanup(func() { DataDir, checkFix = oldData, oldFix })
	database, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing-project")
	if err := database.UpsertProject(missing, "", nil, nil); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	if issues, err := checkProjectRecords(); err != nil || issues != 1 {
		t.Fatalf("checkProjectRecords = %d, %v", issues, err)
	}
	checkFix = true
	if issues, err := checkProjectRecords(); err != nil || issues != 1 {
		t.Fatalf("fixed checkProjectRecords = %d, %v", issues, err)
	}
	database, err = openDB()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	projects, err := database.GetAllProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Fatalf("stale project remained: %+v", projects)
	}
}

// TestCheckSymlinksIOErrorIsNotHealthy 验证读目录遇到真实 IO 错误时 checkSymlinks
// 返回错误，而不是把不可读目录当成健康（与 Web /api/check 的 500 对齐）。
func TestCheckSymlinksIOErrorIsNotHealthy(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	oldRegistry := RegistryDir
	RegistryDir = t.TempDir()
	t.Cleanup(func() { RegistryDir = oldRegistry })

	blocker := filepath.Join(tmpHome, tool.Claude.SkillDir)
	if err := os.MkdirAll(filepath.Dir(blocker), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocker, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := checkSymlinks(); err == nil {
		t.Fatal("checkSymlinks must report an IO error when the skill directory is a file")
	}
}

// TestCheckUsesGlobalSkillDir 锁定 CLI 使用 tool.GetGlobalSkillDir 解析目录：
// 链接放在该目录下时必被发现，且输出路径位于该目录内。
func TestCheckUsesGlobalSkillDir(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	oldRegistry := RegistryDir
	RegistryDir = t.TempDir()
	t.Cleanup(func() { RegistryDir = oldRegistry })

	dir := tool.GetGlobalSkillDir(tool.Claude)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "broken")
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), link); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = writer
	issues, checkErr := checkSymlinks()
	writer.Close()
	os.Stdout = oldStdout
	io.Copy(&buf, reader)

	if checkErr != nil {
		t.Fatalf("checkSymlinks: %v", checkErr)
	}
	if issues != 1 {
		t.Fatalf("issues = %d, want 1; output: %s", issues, buf.String())
	}
	if !strings.Contains(buf.String(), link) {
		t.Fatalf("expected scanned path %s in output: %s", link, buf.String())
	}
}

// parseCheckOutputSet 把 CLI check 的 ⚠ 行解析为 "type path" 集合，
// 覆盖 broken/orphaned/missing_project 三类，供与 Web JSON 逐项比较。
func parseCheckOutputSet(t *testing.T, out string) map[string]bool {
	t.Helper()
	set := make(map[string]bool)
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "⚠ Broken symlink: "):
			set["broken_symlink "+strings.TrimPrefix(line, "⚠ Broken symlink: ")] = true
		case strings.HasPrefix(line, "⚠ Orphaned symlink outside registry: "):
			set["orphaned_symlink "+strings.TrimPrefix(line, "⚠ Orphaned symlink outside registry: ")] = true
		case strings.HasPrefix(line, "⚠ Project directory missing: "):
			set["missing_project "+strings.TrimPrefix(line, "⚠ Project directory missing: ")] = true
		}
	}
	return set
}

// checkIssueSetPayload 是 /api/check 响应的最小解码结构（web.checkResponse 未导出）。
type checkIssueSetPayload struct {
	Status string `json:"status"`
	Issues []struct {
		Type string `json:"type"`
		Path string `json:"path"`
	} `json:"issues"`
}

// TestCheckCLIAndWebShareIssueSetOnSameFixture 用同一 HOME/Registry/DB fixture
// 同时跑 CLI（checkSymlinks + checkProjectRecords）与 Web /api/check，断言
// broken_symlink、orphaned_symlink、missing_project 三类问题集合完全一致。
func TestCheckCLIAndWebShareIssueSetOnSameFixture(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	oldRegistry, oldData, oldFix := RegistryDir, DataDir, checkFix
	RegistryDir, DataDir, checkFix = t.TempDir(), t.TempDir(), false
	t.Cleanup(func() { RegistryDir, DataDir, checkFix = oldRegistry, oldData, oldFix })

	dir := tool.GetGlobalSkillDir(tool.Claude)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(dir, "broken")
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), broken); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "external")
	if err := os.MkdirAll(external, 0755); err != nil {
		t.Fatal(err)
	}
	orphaned := filepath.Join(dir, "orphaned")
	if err := os.Symlink(external, orphaned); err != nil {
		t.Fatal(err)
	}
	managed := filepath.Join(RegistryDir, "skills", "global", "managed")
	if err := os.MkdirAll(managed, 0755); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(dir, "managed")
	if err := os.Symlink(managed, inside); err != nil {
		t.Fatal(err)
	}

	// 同一个临时 DB 记录一个已删除项目：CLI 与 Web 读到同一份项目记录。
	missingProject := filepath.Join(t.TempDir(), "missing-project")
	seed, err := openDB()
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	if err := seed.UpsertProject(missingProject, "", nil, nil); err != nil {
		seed.Close()
		t.Fatalf("UpsertProject: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	// CLI：checkSymlinks + checkProjectRecords 汇总到同一集合。
	var cliOut bytes.Buffer
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = writer
	symlinkIssues, symlinkErr := checkSymlinks()
	projectIssues, projectErr := checkProjectRecords()
	writer.Close()
	os.Stdout = oldStdout
	io.Copy(&cliOut, reader)
	if symlinkErr != nil {
		t.Fatalf("checkSymlinks: %v", symlinkErr)
	}
	if projectErr != nil {
		t.Fatalf("checkProjectRecords: %v", projectErr)
	}
	if symlinkIssues != 2 || projectIssues != 1 {
		t.Fatalf("CLI issues = symlink:%d project:%d, want 2/1; output: %s", symlinkIssues, projectIssues, cliOut.String())
	}
	cliSet := parseCheckOutputSet(t, cliOut.String())
	if len(cliSet) != 3 {
		t.Fatalf("CLI set = %v, want 3 entries", cliSet)
	}

	// Web：同一 Registry/HOME/DB，直接路由请求，不启动服务。
	database, err := openDB()
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	mux := http.NewServeMux()
	web.NewHandler(registry.New(RegistryDir), database).RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/check", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("web status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var payload checkIssueSetPayload
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Status != "issues" {
		t.Fatalf("web status field = %q, want issues", payload.Status)
	}
	webSet := make(map[string]bool, len(payload.Issues))
	for _, issue := range payload.Issues {
		webSet[issue.Type+" "+issue.Path] = true
	}

	if !reflect.DeepEqual(cliSet, webSet) {
		t.Fatalf("CLI/Web issue sets differ:\nCLI=%v\nWeb=%v", cliSet, webSet)
	}
	for _, kind := range []string{"broken_symlink", "orphaned_symlink", "missing_project"} {
		found := false
		for issue := range cliSet {
			if strings.HasPrefix(issue, kind+" ") {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected %s in shared issue set, got %v", kind, cliSet)
		}
	}
}

// TestCheckCLIAndWebAgreeOnIOError 同一 IO 失败 fixture 上，CLI 返回 error，
// Web 返回现有 500 错误响应，二者都不把不可读目录当健康。
func TestCheckCLIAndWebAgreeOnIOError(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	oldRegistry, oldData := RegistryDir, DataDir
	RegistryDir, DataDir = t.TempDir(), t.TempDir()
	t.Cleanup(func() { RegistryDir, DataDir = oldRegistry, oldData })

	blocker := tool.GetGlobalSkillDir(tool.Claude)
	if err := os.MkdirAll(filepath.Dir(blocker), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocker, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := checkSymlinks(); err == nil {
		t.Fatal("CLI must report an IO error for an unreadable skill directory")
	}

	mux := http.NewServeMux()
	web.NewHandler(registry.New(RegistryDir), nil).RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/check", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("web status = %d, want 500; body: %s", rec.Code, rec.Body.String())
	}
	var payload map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload["error"] == "" {
		t.Fatalf("expected error message, got %+v", payload)
	}
}

// TestCheckFixDoesNotClaimRemovedOnFailure 验证 --fix 删除失败时不打印 Removed，
// 也不减少问题计数（不能假报已修复）。
func TestCheckFixDoesNotClaimRemovedOnFailure(t *testing.T) {
	// 只读目录是否真的阻止删除依赖平台与运行身份：先用探针确认，
	// 不依赖 os.Geteuid（Windows 无此函数）。
	probe := t.TempDir()
	probeFile := filepath.Join(probe, "probe")
	if err := os.WriteFile(probeFile, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(probe, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(probe, 0755) })
	if err := os.Remove(probeFile); err == nil {
		t.Skip("removal inside a read-only directory succeeded (root or non-POSIX permissions)")
	}

	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	oldRegistry, oldFix := RegistryDir, checkFix
	RegistryDir, checkFix = t.TempDir(), true
	t.Cleanup(func() { RegistryDir, checkFix = oldRegistry, oldFix })

	dir := tool.GetGlobalSkillDir(tool.Claude)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "broken")
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), link); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	var buf bytes.Buffer
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = writer
	issues, checkErr := checkSymlinks()
	writer.Close()
	os.Stdout = oldStdout
	io.Copy(&buf, reader)

	if checkErr != nil {
		t.Fatalf("checkSymlinks: %v", checkErr)
	}
	if issues != 1 {
		t.Fatalf("issue count must stay 1 when removal fails, got %d", issues)
	}
	out := buf.String()
	if strings.Contains(out, "→ Removed") {
		t.Fatalf("must not claim removal: %s", out)
	}
	if !strings.Contains(out, "Remove failed") {
		t.Fatalf("expected removal failure notice, got: %s", out)
	}
}

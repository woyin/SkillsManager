package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/db"
	"github.com/woyin/skills-manager/internal/health"
	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

func TestCheckReportsMissingProjects(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	home.ResetForTest()

	database, err := db.Open(filepath.Join(dir, "data", "sm.db"))
	if err != nil {
		t.Fatalf("Open db failed: %v", err)
	}
	defer database.Close()

	missingProject := filepath.Join(dir, "missing-project")
	if err := database.UpsertProject(missingProject, "cloudflare", nil, nil); err != nil {
		t.Fatalf("UpsertProject failed: %v", err)
	}

	handler := NewHandler(registry.New(filepath.Join(dir, "registry")), database)
	req := httptest.NewRequest(http.MethodGet, "/api/check", nil)
	rec := httptest.NewRecorder()

	handler.handleCheck(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	var resp checkResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response failed: %v", err)
	}
	if resp.Status != "issues" {
		t.Fatalf("Expected issues status, got %+v", resp)
	}
	if len(resp.Issues) != 1 {
		t.Fatalf("Expected 1 issue, got %+v", resp)
	}
	if resp.Issues[0].Type != "missing_project" || resp.Issues[0].Path != missingProject {
		t.Fatalf("Unexpected issue: %+v", resp.Issues[0])
	}
}

func TestCheckHealthyReturnsEmptyIssueArray(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	home.ResetForTest()

	handler := NewHandler(registry.New(filepath.Join(dir, "registry")), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/check", nil)
	rec := httptest.NewRecorder()

	handler.handleCheck(rec, req)

	var raw map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("Decode response failed: %v", err)
	}
	if raw["status"] != "ok" {
		t.Fatalf("Expected ok status, got %+v", raw)
	}
	if _, ok := raw["issues"].([]any); !ok {
		t.Fatalf("Expected issues to be an array, got %+v", raw["issues"])
	}
}

func TestCheckReportsOrphanedSymlinkOutsideRegistry(t *testing.T) {
	dir := t.TempDir()
	tmpHome := filepath.Join(dir, "home")
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()

	externalTarget := filepath.Join(dir, "external", "skill")
	if err := os.MkdirAll(externalTarget, 0755); err != nil {
		t.Fatalf("creating external target: %v", err)
	}
	linkPath := filepath.Join(tmpHome, ".codex", "skills", "external-skill")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		t.Fatalf("creating link parent: %v", err)
	}
	if err := os.Symlink(externalTarget, linkPath); err != nil {
		t.Fatalf("creating symlink: %v", err)
	}

	handler := NewHandler(registry.New(filepath.Join(dir, "registry")), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/check", nil)
	rec := httptest.NewRecorder()

	handler.handleCheck(rec, req)

	var resp checkResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response failed: %v", err)
	}
	if len(resp.Issues) != 1 {
		t.Fatalf("Expected 1 issue, got %+v", resp)
	}
	if resp.Issues[0].Type != "orphaned_symlink" || resp.Issues[0].Path != linkPath {
		t.Fatalf("Unexpected issue: %+v", resp.Issues[0])
	}
}

// TestCheckIOErrorIsNotHealthy 验证扫描目录遇到真实 IO 错误时返回现有错误
// 响应，而不是把不可读目录当成“健康”。用普通文件占位触发 ENOTDIR，避免
// 依赖权限位（root 下 chmod 无效）。
func TestCheckIOErrorIsNotHealthy(t *testing.T) {
	dir := t.TempDir()
	tmpHome := filepath.Join(dir, "home")
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()

	blocker := filepath.Join(tmpHome, ".claude", "skills")
	if err := os.MkdirAll(filepath.Dir(blocker), 0755); err != nil {
		t.Fatalf("creating parent: %v", err)
	}
	if err := os.WriteFile(blocker, []byte("not a directory"), 0644); err != nil {
		t.Fatalf("creating blocker file: %v", err)
	}

	handler := NewHandler(registry.New(filepath.Join(dir, "registry")), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/check", nil)
	rec := httptest.NewRecorder()

	handler.handleCheck(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for unreadable skill dir, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if resp["error"] == "" {
		t.Fatalf("expected error message, got %+v", resp)
	}
}

// TestCheckMatchesHealthScanOnSameFixture 验证 Web /api/check 与共享扫描在同一
// fixture 上给出完全一致的问题集合（类型与顺序），即 CLI/Web 共用判据而非各写一份。
func TestCheckMatchesHealthScanOnSameFixture(t *testing.T) {
	dir := t.TempDir()
	tmpHome := filepath.Join(dir, "home")
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()

	registryDir := filepath.Join(dir, "registry")
	managed := filepath.Join(registryDir, "skills", "global", "managed")
	if err := os.MkdirAll(managed, 0755); err != nil {
		t.Fatal(err)
	}
	skills := filepath.Join(tmpHome, ".claude", "skills")
	if err := os.MkdirAll(skills, 0755); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(skills, "broken")
	if err := os.Symlink(filepath.Join(dir, "missing-target"), broken); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(dir, "external")
	if err := os.MkdirAll(external, 0755); err != nil {
		t.Fatal(err)
	}
	orphaned := filepath.Join(skills, "orphaned")
	if err := os.Symlink(external, orphaned); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(skills, "managed")
	if err := os.Symlink(managed, inside); err != nil {
		t.Fatal(err)
	}

	database, err := db.Open(filepath.Join(dir, "data", "sm.db"))
	if err != nil {
		t.Fatalf("Open db failed: %v", err)
	}
	defer database.Close()
	missingProject := filepath.Join(dir, "missing-project")
	if err := database.UpsertProject(missingProject, "", nil, nil); err != nil {
		t.Fatalf("UpsertProject failed: %v", err)
	}

	var dirs []string
	for _, tl := range tool.AllTools() {
		dirs = append(dirs, tool.GetGlobalSkillDir(tl))
	}
	report, err := health.Scan(dirs, registryDir, []string{missingProject})
	if err != nil {
		t.Fatalf("health.Scan: %v", err)
	}
	if len(report.Issues) != 3 {
		t.Fatalf("expected 3 issues from shared scan, got %+v", report.Issues)
	}

	handler := NewHandler(registry.New(registryDir), database)
	req := httptest.NewRequest(http.MethodGet, "/api/check", nil)
	rec := httptest.NewRecorder()
	handler.handleCheck(rec, req)

	var resp checkResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response failed: %v", err)
	}
	if len(resp.Issues) != len(report.Issues) {
		t.Fatalf("handler issues = %+v, shared scan = %+v", resp.Issues, report.Issues)
	}
	for i, issue := range report.Issues {
		if resp.Issues[i].Type != string(issue.Type) || resp.Issues[i].Path != issue.Path {
			t.Fatalf("issue %d differs: handler=%+v scan=%+v", i, resp.Issues[i], issue)
		}
	}
}

func TestRegistryAPIIncludesItemDetails(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "registry", "skills", "global", "demo-skill")
	if err := os.MkdirAll(filepath.Join(skillDir, ".git"), 0755); err != nil {
		t.Fatalf("creating skill dir: %v", err)
	}
	gitConfig := "[remote \"origin\"]\n\turl = https://github.com/user/demo-skill.git\n"
	if err := os.WriteFile(filepath.Join(skillDir, ".git", "config"), []byte(gitConfig), 0644); err != nil {
		t.Fatalf("writing git config: %v", err)
	}
	mcpDir := filepath.Join(dir, "registry", "mcp")
	if err := os.MkdirAll(mcpDir, 0755); err != nil {
		t.Fatalf("creating mcp dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mcpDir, "github.json"), []byte(`{"mcpServers":{"github":{"type":"stdio","command":"github"}}}`), 0644); err != nil {
		t.Fatalf("writing mcp: %v", err)
	}

	handler := NewHandler(registry.New(filepath.Join(dir, "registry")), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/registry", nil)
	rec := httptest.NewRecorder()

	handler.handleRegistry(rec, req)

	var raw map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("Decode response failed: %v", err)
	}
	if _, ok := raw["skill_details"].(map[string]any); !ok {
		t.Fatalf("Expected skill_details object, got %+v", raw)
	}
	if _, ok := raw["mcp_details"].([]any); !ok {
		t.Fatalf("Expected mcp_details array, got %+v", raw)
	}

	details := raw["skill_details"].(map[string]any)
	global := details["global"].([]any)
	first := global[0].(map[string]any)
	if first["source_url"] != "https://github.com/user/demo-skill.git" || first["last_updated"] == "" {
		t.Fatalf("Expected registry metadata, got %+v", first)
	}
}

func TestHandleIndex(t *testing.T) {
	dir := t.TempDir()
	handler := NewHandler(registry.New(filepath.Join(dir, "registry")), nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.handleIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html" {
		t.Errorf("Expected text/html, got %s", ct)
	}
}

func TestHandleIndexNotFound(t *testing.T) {
	dir := t.TempDir()
	handler := NewHandler(registry.New(filepath.Join(dir, "registry")), nil)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()
	handler.handleIndex(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("Expected 404, got %d", rec.Code)
	}
}

func TestHandleProjectsWithoutDB(t *testing.T) {
	dir := t.TempDir()
	handler := NewHandler(registry.New(filepath.Join(dir, "registry")), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	rec := httptest.NewRecorder()
	handler.handleProjects(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec.Code)
	}

	var projects []any
	if err := json.NewDecoder(rec.Body).Decode(&projects); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("Expected empty array, got %d items", len(projects))
	}
}

func TestHandleProjectsWithDB(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "data", "sm.db"))
	if err != nil {
		t.Fatalf("Open db failed: %v", err)
	}
	defer database.Close()

	database.UpsertProject("/test/project", "cloudflare", []string{"skill-1"}, nil)

	handler := NewHandler(registry.New(filepath.Join(dir, "registry")), database)
	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	rec := httptest.NewRecorder()
	handler.handleProjects(rec, req)

	var projects []db.Project
	if err := json.NewDecoder(rec.Body).Decode(&projects); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("Expected 1 project, got %d", len(projects))
	}
	if projects[0].Profile != "cloudflare" {
		t.Errorf("Expected profile 'cloudflare', got '%s'", projects[0].Profile)
	}
}

func TestHandleHistoryWithoutDB(t *testing.T) {
	dir := t.TempDir()
	handler := NewHandler(registry.New(filepath.Join(dir, "registry")), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/history", nil)
	rec := httptest.NewRecorder()
	handler.handleHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec.Code)
	}

	var history []any
	if err := json.NewDecoder(rec.Body).Decode(&history); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("Expected empty array, got %d items", len(history))
	}
}

func TestHandleHistoryWithDB(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "data", "sm.db"))
	if err != nil {
		t.Fatalf("Open db failed: %v", err)
	}
	defer database.Close()

	database.RecordInstallation("/test/project", "default", []string{"skill-a"}, []string{"mcp-a"})

	handler := NewHandler(registry.New(filepath.Join(dir, "registry")), database)
	req := httptest.NewRequest(http.MethodGet, "/api/history", nil)
	rec := httptest.NewRecorder()
	handler.handleHistory(rec, req)

	var installs []db.Installation
	if err := json.NewDecoder(rec.Body).Decode(&installs); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if len(installs) != 1 {
		t.Fatalf("Expected 1 installation, got %d", len(installs))
	}
}

func TestHandleTools(t *testing.T) {
	dir := t.TempDir()
	handler := NewHandler(registry.New(filepath.Join(dir, "registry")), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/tools", nil)
	rec := httptest.NewRecorder()
	handler.handleTools(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec.Code)
	}

	var tools []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&tools); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if len(tools) < 6 {
		t.Errorf("Expected at least 6 tools, got %d", len(tools))
	}

	// Verify structure
	for _, tool := range tools {
		if _, ok := tool["name"]; !ok {
			t.Error("Tool missing 'name' field")
		}
		if _, ok := tool["installed"]; !ok {
			t.Error("Tool missing 'installed' field")
		}
		if _, ok := tool["has_skill_dir"]; !ok {
			t.Error("Tool missing 'has_skill_dir' field")
		}
	}
}

func TestHandleAivo(t *testing.T) {
	dir := t.TempDir()
	handler := NewHandler(registry.New(filepath.Join(dir, "registry")), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/aivo", nil)
	rec := httptest.NewRecorder()
	handler.handleAivo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if _, ok := resp["installed"]; !ok {
		t.Error("Response missing 'installed' field")
	}
}

func TestRegisterRoutes(t *testing.T) {
	dir := t.TempDir()
	handler := NewHandler(registry.New(filepath.Join(dir, "registry")), nil)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Test that routes are registered by making requests
	tests := []struct {
		path   string
		method string
	}{
		{"/api/registry", http.MethodGet},
		{"/api/projects", http.MethodGet},
		{"/api/history", http.MethodGet},
		{"/api/tools", http.MethodGet},
		{"/api/aivo", http.MethodGet},
		{"/api/check", http.MethodGet},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code == http.StatusNotFound {
				t.Errorf("Route %s not registered (404)", tt.path)
			}
		})
	}
}

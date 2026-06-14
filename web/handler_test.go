package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/db"
	"github.com/woyin/skills-manager/internal/registry"
)

func TestCheckReportsMissingProjects(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

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
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)

	externalTarget := filepath.Join(dir, "external", "skill")
	if err := os.MkdirAll(externalTarget, 0755); err != nil {
		t.Fatalf("creating external target: %v", err)
	}
	linkPath := filepath.Join(home, ".codex", "skills", "external-skill")
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

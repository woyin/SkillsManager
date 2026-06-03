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

	var raw map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("Decode response failed: %v", err)
	}
	if raw["status"] != "ok" {
		t.Fatalf("Expected ok status, got %+v", raw)
	}
	if _, ok := raw["issues"].([]interface{}); !ok {
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

	var raw map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("Decode response failed: %v", err)
	}
	if _, ok := raw["skill_details"].(map[string]interface{}); !ok {
		t.Fatalf("Expected skill_details object, got %+v", raw)
	}
	if _, ok := raw["mcp_details"].([]interface{}); !ok {
		t.Fatalf("Expected mcp_details array, got %+v", raw)
	}

	details := raw["skill_details"].(map[string]interface{})
	global := details["global"].([]interface{})
	first := global[0].(map[string]interface{})
	if first["source_url"] != "https://github.com/user/demo-skill.git" || first["last_updated"] == "" {
		t.Fatalf("Expected registry metadata, got %+v", first)
	}
}

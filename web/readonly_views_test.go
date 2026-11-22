package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/db"
	"github.com/woyin/skills-manager/internal/profile"
	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

func writeTestSkill(t *testing.T, regDir, category, name, body string) string {
	t.Helper()
	dir := filepath.Join(regDir, "skills", category, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	return dir
}

func TestRegistryAPIIncludesSkillQualitySummaries(t *testing.T) {
	dir := t.TempDir()
	regDir := filepath.Join(dir, "registry")
	skillDir := writeTestSkill(t, regDir, "global", "demo-skill", `---
name: demo-skill
description: A thorough skill description that exceeds twenty characters for agents.
---

## Overview
Detailed guidance for testing registry scoring.

## Usage
Run the workflow step by step.

## Verification
Check all expected outputs before finishing.
`)
	reg := registry.New(regDir)
	if err := reg.WriteOrigin(skillDir, registry.SkillOrigin{
		SourceKind: registry.SourceGit,
		Source:     "https://github.com/example/demo-skill.git",
		Ref:        "v1.0.0",
		RefKind:    registry.RefTag,
		Commit:     "abcdef1234567890",
	}); err != nil {
		t.Fatalf("WriteOrigin: %v", err)
	}

	handler := NewHandler(reg, nil)
	rec := httptest.NewRecorder()
	handler.handleRegistry(rec, httptest.NewRequest(http.MethodGet, "/api/registry", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	var resp registryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	summary, ok := resp.SkillQuality["global/demo-skill"]
	if !ok {
		t.Fatalf("missing skill_quality entry for global/demo-skill: %+v", resp.SkillQuality)
	}
	if summary.Score <= 0 {
		t.Errorf("expected positive score, got %+v", summary)
	}
	if summary.Errors != 0 {
		t.Errorf("expected 0 lint errors, got %+v", summary)
	}
	if summary.UpdateClass != string(registry.ClassPinned) {
		t.Errorf("update_class = %q, want %q", summary.UpdateClass, registry.ClassPinned)
	}
}

func TestHandleProfilesListsAndValidatesProfiles(t *testing.T) {
	dir := t.TempDir()
	regDir := filepath.Join(dir, "registry")
	profilesDir := filepath.Join(dir, "profiles")
	writeTestSkill(t, regDir, "global", "good-skill", "---\nname: good-skill\ndescription: A valid registered skill for profile validation.\n---\n")
	mcpDir := filepath.Join(regDir, "mcp")
	if err := os.MkdirAll(mcpDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mcpDir, "github.json"), []byte(`{"mcpServers":{}}`), 0644); err != nil {
		t.Fatal(err)
	}

	loader := profile.NewLoader(profilesDir)
	if err := loader.Save("valid-prof", &profile.Profile{
		Skills: []string{"good-skill"},
		MCP:    []string{"github"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := loader.Save("broken-prof", &profile.Profile{
		Skills: []string{"missing-skill"},
		MCP:    []string{"missing-mcp"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "corrupt.json"), []byte(`{not-json`), 0644); err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(registry.New(regDir), nil).WithProfilesDir(profilesDir)
	rec := httptest.NewRecorder()
	handler.handleProfiles(rec, httptest.NewRequest(http.MethodGet, "/api/profiles", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	var items []profileItem
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d profiles, want 3: %+v", len(items), items)
	}
	// Sorted alphabetically: broken-prof, corrupt, valid-prof.
	if items[0].Name != "broken-prof" || items[0].Valid || len(items[0].Issues) != 2 {
		t.Errorf("broken-prof = %+v, want invalid with 2 issues", items[0])
	}
	if items[1].Name != "corrupt" || items[1].Valid || len(items[1].Issues) != 1 {
		t.Errorf("corrupt = %+v, want invalid with parse issue", items[1])
	}
	if items[2].Name != "valid-prof" || !items[2].Valid || len(items[2].Issues) != 0 {
		t.Errorf("valid-prof = %+v, want valid with 0 issues", items[2])
	}

	// Missing profiles directory returns empty array instead of error.
	emptyHandler := NewHandler(registry.New(regDir), nil).WithProfilesDir(filepath.Join(dir, "absent"))
	emptyRec := httptest.NewRecorder()
	emptyHandler.handleProfiles(emptyRec, httptest.NewRequest(http.MethodGet, "/api/profiles", nil))
	if emptyRec.Code != http.StatusOK || strings.TrimSpace(emptyRec.Body.String()) != "[]" {
		t.Fatalf("absent profilesDir response = %d %q, want 200 []", emptyRec.Code, emptyRec.Body.String())
	}
}

func TestHandleSkillDetailReturnsScoreLintOriginAndContent(t *testing.T) {
	dir := t.TempDir()
	regDir := filepath.Join(dir, "registry")
	content := "---\nname: inspect-me\ndescription: short\n---\n\n# Body with <tag>\n"
	skillDir := writeTestSkill(t, regDir, "global", "inspect-me", content)
	reg := registry.New(regDir)
	if err := reg.WriteOrigin(skillDir, registry.SkillOrigin{
		SourceKind: registry.SourceLocalSnapshot,
		Source:     "/local/source",
	}); err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(reg, nil)

	// Missing name -> 400.
	rec := httptest.NewRecorder()
	handler.handleSkill(rec, httptest.NewRequest(http.MethodGet, "/api/skill", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing name status = %d, want 400", rec.Code)
	}

	// Path traversal attempt -> 404 (never reads outside registry).
	rec = httptest.NewRecorder()
	handler.handleSkill(rec, httptest.NewRequest(http.MethodGet, "/api/skill?category=../../etc&name=passwd", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("traversal status = %d, want 404", rec.Code)
	}

	// Valid skill lookup.
	rec = httptest.NewRecorder()
	handler.handleSkill(rec, httptest.NewRequest(http.MethodGet, "/api/skill?category=global&name=inspect-me", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("valid skill status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var detail skillDetailResponse
	if err := json.NewDecoder(rec.Body).Decode(&detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.Name != "inspect-me" || detail.Category != "global" {
		t.Errorf("unexpected identity: %+v", detail)
	}
	if detail.Content != content {
		t.Errorf("content mismatch: got %q, want %q", detail.Content, content)
	}
	if detail.Origin.UpdateClass != string(registry.ClassSnapshot) || !detail.Origin.HasFile {
		t.Errorf("unexpected origin: %+v", detail.Origin)
	}
	if detail.Lint.Warnings == 0 || detail.Lint.HasErrors {
		t.Errorf("expected short-description warning without errors, got %+v", detail.Lint)
	}

	// Cross-category duplicate without category query -> 409 Conflict.
	writeTestSkill(t, regDir, "custom", "inspect-me", content)
	rec = httptest.NewRecorder()
	handler.handleSkill(rec, httptest.NewRequest(http.MethodGet, "/api/skill?name=inspect-me", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("ambiguous skill status = %d, want 409", rec.Code)
	}
}

func TestHandlePlanPreviewsAllowedProjectAndRejectsUnknownProject(t *testing.T) {
	dir := t.TempDir()
	regDir := filepath.Join(dir, "registry")
	profilesDir := filepath.Join(dir, "profiles")
	writeTestSkill(t, regDir, "global", "wanted-skill", "---\nname: wanted-skill\ndescription: A valid skill referenced by project baseline.\n---\n")

	registeredProj := filepath.Join(dir, "registered-proj")
	if err := os.MkdirAll(registeredProj, 0755); err != nil {
		t.Fatal(err)
	}
	if err := project.NewManager(registeredProj).Save(&project.Config{
		Skills: []string{"wanted-skill"},
	}); err != nil {
		t.Fatal(err)
	}

	workspaceProj := filepath.Join(dir, "workspace-proj")
	if err := os.MkdirAll(workspaceProj, 0755); err != nil {
		t.Fatal(err)
	}

	unregisteredProj := filepath.Join(dir, "secret-proj")
	if err := os.MkdirAll(unregisteredProj, 0755); err != nil {
		t.Fatal(err)
	}

	database, err := db.Open(filepath.Join(dir, "data", "sm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.UpsertProject(registeredProj, "", []string{"wanted-skill"}, nil); err != nil {
		t.Fatal(err)
	}
	missingProj := filepath.Join(dir, "deleted-proj")
	if err := database.UpsertProject(missingProj, "", nil, nil); err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(registry.New(regDir), database).
		WithProfilesDir(profilesDir).
		WithCurrentProject(workspaceProj)
	handler.detectTools = func() []tool.Tool { return []tool.Tool{tool.Claude} }

	// 1. Registered project returns read-only plan with ADD proposal and zero filesystem mutation.
	rec := httptest.NewRecorder()
	handler.handlePlan(rec, httptest.NewRequest(http.MethodGet, "/api/plan?project="+registeredProj, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("registered project status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var plan planResponse
	if err := json.NewDecoder(rec.Body).Decode(&plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if plan.Bootstrap || plan.CheckOK {
		t.Errorf("expected curated unsatisfied plan, got bootstrap=%v check=%v", plan.Bootstrap, plan.CheckOK)
	}
	if len(plan.Proposals) != 1 || plan.Proposals[0].Action != "add" || plan.Proposals[0].Skill != "wanted-skill" {
		t.Errorf("unexpected proposals: %+v", plan.Proposals)
	}
	if len(plan.Agents) != 1 || plan.Agents[0] != "claude" {
		t.Errorf("unexpected agents: %+v", plan.Agents)
	}
	// Verify read-only: no symlink created in project.
	if _, statErr := os.Lstat(filepath.Join(registeredProj, tool.Claude.ProjectSkillDir, "wanted-skill")); !os.IsNotExist(statErr) {
		t.Errorf("plan preview must not modify disk: %v", statErr)
	}

	// 2. Default (empty query) previews current workspace (bootstrap plan).
	rec = httptest.NewRecorder()
	handler.handlePlan(rec, httptest.NewRequest(http.MethodGet, "/api/plan", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("default workspace status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var wsPlan planResponse
	if err := json.NewDecoder(rec.Body).Decode(&wsPlan); err != nil {
		t.Fatalf("decode workspace plan: %v", err)
	}
	if !wsPlan.Bootstrap {
		t.Errorf("expected bootstrap plan for workspace without .sm.json, got %+v", wsPlan)
	}

	// 3. Unregistered path -> 403 Forbidden.
	rec = httptest.NewRecorder()
	handler.handlePlan(rec, httptest.NewRequest(http.MethodGet, "/api/plan?project="+unregisteredProj, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unregistered project status = %d, want 403", rec.Code)
	}

	// 4. Registered but missing on disk -> 404 Not Found.
	rec = httptest.NewRecorder()
	handler.handlePlan(rec, httptest.NewRequest(http.MethodGet, "/api/plan?project="+missingProj, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing project status = %d, want 404", rec.Code)
	}

	// 5. No currentProject and no query param -> 400 Bad Request.
	noWS := NewHandler(registry.New(regDir), nil)
	rec = httptest.NewRecorder()
	noWS.handlePlan(rec, httptest.NewRequest(http.MethodGet, "/api/plan", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing project param status = %d, want 400", rec.Code)
	}
}

func TestAPIRejectsNonGetMethods(t *testing.T) {
	dir := t.TempDir()
	handler := NewHandler(registry.New(filepath.Join(dir, "registry")), nil)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	for _, endpoint := range []string{
		"/api/registry",
		"/api/projects",
		"/api/history",
		"/api/check",
		"/api/tools",
		"/api/aivo",
		"/api/profiles",
		"/api/skill?name=x",
		"/api/plan",
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, endpoint, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s status = %d, want 405", endpoint, rec.Code)
		}
		if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
			t.Errorf("POST %s Allow = %q, want GET", endpoint, allow)
		}
	}
}

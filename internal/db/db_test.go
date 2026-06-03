package db

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	var tableCount int
	err = database.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('installations','projects')").Scan(&tableCount)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if tableCount != 2 {
		t.Errorf("Expected 2 tables, got %d", tableCount)
	}
}

func TestOpenCreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "missing", "nested", "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	if _, err := database.db.Exec("SELECT 1"); err != nil {
		t.Fatalf("Database is not usable: %v", err)
	}
}

func TestRecordInstallation(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	err = database.RecordInstallation("/Users/test/project", "cloudflare", []string{"skill-a", "skill-b"}, []string{"mcp-x"})
	if err != nil {
		t.Fatalf("RecordInstallation failed: %v", err)
	}

	installs, err := database.GetInstallations("/Users/test/project")
	if err != nil {
		t.Fatalf("GetInstallations failed: %v", err)
	}
	if len(installs) != 1 {
		t.Fatalf("Expected 1 installation, got %d", len(installs))
	}
	if installs[0].Profile != "cloudflare" {
		t.Errorf("Expected profile 'cloudflare', got '%s'", installs[0].Profile)
	}
}

func TestUpsertProject(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	err = database.UpsertProject("/Users/test/project", "cloudflare", []string{"extra-skill"}, []string{})
	if err != nil {
		t.Fatalf("UpsertProject failed: %v", err)
	}

	projects, err := database.GetAllProjects()
	if err != nil {
		t.Fatalf("GetAllProjects failed: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("Expected 1 project, got %d", len(projects))
	}
	if projects[0].Profile != "cloudflare" {
		t.Errorf("Expected profile 'cloudflare', got '%s'", projects[0].Profile)
	}
}

func TestJSONFieldsMatchWebAPIContract(t *testing.T) {
	project := Project{
		Path:        "/Users/test/project",
		Profile:     "cloudflare",
		ExtraSkills: []string{"extra-skill"},
		ExtraMCP:    []string{"extra-mcp"},
	}
	projectJSON, err := json.Marshal(project)
	if err != nil {
		t.Fatalf("Marshal project failed: %v", err)
	}
	projectText := string(projectJSON)
	for _, field := range []string{"path", "profile", "extra_skills", "extra_mcp", "last_installed"} {
		if !strings.Contains(projectText, `"`+field+`"`) {
			t.Fatalf("Project JSON missing field %q: %s", field, projectText)
		}
	}
	if strings.Contains(projectText, "ExtraSkills") || strings.Contains(projectText, "ExtraMCP") {
		t.Fatalf("Project JSON exposed Go field names: %s", projectText)
	}

	installation := Installation{
		ProjectPath: "/Users/test/project",
		Profile:     "cloudflare",
		Skills:      []string{"skill-a"},
		MCP:         []string{"mcp-a"},
	}
	installationJSON, err := json.Marshal(installation)
	if err != nil {
		t.Fatalf("Marshal installation failed: %v", err)
	}
	installationText := string(installationJSON)
	for _, field := range []string{"id", "project_path", "profile", "skills", "mcp", "installed_at"} {
		if !strings.Contains(installationText, `"`+field+`"`) {
			t.Fatalf("Installation JSON missing field %q: %s", field, installationText)
		}
	}
	if strings.Contains(installationText, "ProjectPath") || strings.Contains(installationText, "InstalledAt") {
		t.Fatalf("Installation JSON exposed Go field names: %s", installationText)
	}
}

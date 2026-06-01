package db

import (
	"path/filepath"
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

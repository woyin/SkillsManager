package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/db"
	"github.com/woyin/skills-manager/internal/profile"
	"github.com/woyin/skills-manager/internal/prompt"
	"github.com/woyin/skills-manager/internal/registry"
)

func TestPrintImportPreview(t *testing.T) {
	data := &ExportData{
		Skills: map[string][]registry.ItemDetail{
			"global": {
				{Name: "skill-a", Path: "/path/a"},
			},
		},
		MCP: []registry.ItemDetail{
			{Name: "mcp-x", Path: "/path/x"},
		},
		Profiles: map[string]*profile.Config{
			"cloudflare": {Skills: []string{"s1"}, MCP: []string{"m1"}},
		},
		Projects: []db.Project{
			{Path: "/project/a", Profile: "cloudflare"},
		},
	}

	err := printImportPreview(data)
	if err != nil {
		t.Fatalf("printImportPreview failed: %v", err)
	}
}

func TestPrintImportPreviewEmpty(t *testing.T) {
	data := &ExportData{}

	err := printImportPreview(data)
	if err != nil {
		t.Fatalf("printImportPreview failed on empty data: %v", err)
	}
}

func TestPerformImportSkillsAndProfiles(t *testing.T) {
	dir := t.TempDir()

	// Override global paths
	oldReg, oldProf, oldData := RegistryDir, ProfilesDir, DataDir
	RegistryDir = filepath.Join(dir, "registry")
	ProfilesDir = filepath.Join(dir, "profiles")
	DataDir = filepath.Join(dir, "data")
	defer func() {
		RegistryDir, ProfilesDir, DataDir = oldReg, oldProf, oldData
	}()

	// Create a source skill to import from
	sourceDir := filepath.Join(dir, "source-skill")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("creating source: %v", err)
	}
	os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte("# Test Skill"), 0644)

	data := &ExportData{
		Profiles: map[string]*profile.Config{
			"test-prof": {Skills: []string{"s1"}, MCP: []string{"m1"}},
		},
	}

	err := performImport(data)
	if err != nil {
		t.Fatalf("performImport failed: %v", err)
	}

	// Verify profile was created
	loader := profile.NewLoader(ProfilesDir)
	p, err := loader.Load("test-prof")
	if err != nil {
		t.Fatalf("Profile should exist after import: %v", err)
	}
	if len(p.Skills) != 1 || p.Skills[0] != "s1" {
		t.Errorf("Profile skills mismatch: %v", p.Skills)
	}
}

func TestPerformImportEmptyData(t *testing.T) {
	dir := t.TempDir()

	oldReg, oldProf, oldData := RegistryDir, ProfilesDir, DataDir
	RegistryDir = filepath.Join(dir, "registry")
	ProfilesDir = filepath.Join(dir, "profiles")
	DataDir = filepath.Join(dir, "data")
	defer func() {
		RegistryDir, ProfilesDir, DataDir = oldReg, oldProf, oldData
	}()

	data := &ExportData{}
	err := performImport(data)
	if err != nil {
		t.Fatalf("performImport with empty data should succeed: %v", err)
	}
}

func TestPerformImportProjectsWithDB(t *testing.T) {
	dir := t.TempDir()

	oldReg, oldProf, oldData := RegistryDir, ProfilesDir, DataDir
	RegistryDir = filepath.Join(dir, "registry")
	ProfilesDir = filepath.Join(dir, "profiles")
	DataDir = filepath.Join(dir, "data")
	defer func() {
		RegistryDir, ProfilesDir, DataDir = oldReg, oldProf, oldData
	}()

	data := &ExportData{
		Projects: []db.Project{
			{Path: "/imported/project", Profile: "cloudflare", ExtraSkills: []string{"sk1"}},
		},
	}

	err := performImport(data)
	if err != nil {
		t.Fatalf("performImport failed: %v", err)
	}

	// Verify project was imported
	database, err := openDB()
	if err != nil {
		t.Fatalf("Open db failed: %v", err)
	}
	defer database.Close()

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

func TestPerformImportPromptSets(t *testing.T) {
	dir := t.TempDir()

	oldReg, oldProf, oldData := RegistryDir, ProfilesDir, DataDir
	RegistryDir = filepath.Join(dir, "registry")
	ProfilesDir = filepath.Join(dir, "profiles")
	DataDir = filepath.Join(dir, "data")
	defer func() {
		RegistryDir, ProfilesDir, DataDir = oldReg, oldProf, oldData
	}()

	data := &ExportData{
		Prompts: map[string]*prompt.PromptSet{
			"default": {
				Name: "default",
				Prompts: map[string]string{
					"AGENTS.md": "# Agents",
				},
			},
		},
	}

	if err := performImport(data); err != nil {
		t.Fatalf("performImport failed: %v", err)
	}

	manager := prompt.NewManager(filepath.Join(RegistryDir, "prompts"))
	loaded, err := manager.Load("default")
	if err != nil {
		t.Fatalf("prompt set should exist after import: %v", err)
	}
	if loaded.Prompts["AGENTS.md"] != "# Agents" {
		t.Fatalf("imported prompt content = %q, want %q", loaded.Prompts["AGENTS.md"], "# Agents")
	}
}

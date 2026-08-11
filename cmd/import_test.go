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

func TestPerformImportReplaceReplacesSkillProfilePromptAndProject(t *testing.T) {
	dir := t.TempDir()
	oldReg, oldProf, oldData := RegistryDir, ProfilesDir, DataDir
	oldMerge, oldReplace := importMerge, importReplace
	RegistryDir = filepath.Join(dir, "registry")
	ProfilesDir = filepath.Join(dir, "profiles")
	DataDir = filepath.Join(dir, "data")
	importMerge, importReplace = true, true
	t.Cleanup(func() {
		RegistryDir, ProfilesDir, DataDir = oldReg, oldProf, oldData
		importMerge, importReplace = oldMerge, oldReplace
	})

	oldSource := filepath.Join(dir, "old-skill")
	newSource := filepath.Join(dir, "new-skill")
	for _, source := range []string{oldSource, newSource} {
		if err := os.MkdirAll(source, 0755); err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(source)
		if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: import replace test skill\n---\n# "+name+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	reg := registry.New(RegistryDir)
	if err := reg.AddSkill(oldSource, "global", ""); err != nil {
		t.Fatalf("seeding old skill: %v", err)
	}
	oldMCP := filepath.Join(dir, "old-mcp.json")
	newMCP := filepath.Join(dir, "new-mcp.json")
	for _, path := range []string{oldMCP, newMCP} {
		if err := os.WriteFile(path, []byte(`{"mcpServers":{"demo":{"type":"http","url":"https://example.test"}}}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := reg.AddMCP(oldMCP); err != nil {
		t.Fatalf("seeding old MCP: %v", err)
	}
	if err := profile.NewLoader(ProfilesDir).Save("old", &profile.Config{}); err != nil {
		t.Fatal(err)
	}
	if err := prompt.NewManager(filepath.Join(RegistryDir, "prompts")).Save(&prompt.PromptSet{Name: "old", Prompts: map[string]string{"old.md": "old"}}); err != nil {
		t.Fatal(err)
	}
	database, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertProject(filepath.Join(dir, "old-project"), "old", nil, nil); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	data := &ExportData{
		Skills: map[string][]registry.ItemDetail{
			"global": {{Name: "new-skill", SourceURL: newSource}},
		},
		MCP:      []registry.ItemDetail{{Name: "new-mcp", SourceURL: newMCP}},
		Profiles: map[string]*profile.Config{"new": {Skills: []string{"new-skill"}}},
		Prompts:  map[string]*prompt.PromptSet{"new": {Name: "new", Prompts: map[string]string{"new.md": "new"}}},
		Projects: []db.Project{{Path: filepath.Join(dir, "new-project"), Profile: "new"}},
	}
	if err := performImport(data); err != nil {
		t.Fatalf("performImport replace: %v", err)
	}
	if old, _ := reg.FindSkillDir("old-skill"); old != "" {
		t.Fatalf("old skill was not removed: %s", old)
	}
	if added, _ := reg.FindSkillDir("new-skill"); added == "" {
		t.Fatal("new skill was not imported")
	}
	if names, _ := reg.ListMCP(); len(names) != 1 || names[0] != "new-mcp" {
		t.Fatalf("MCP after replace = %v, want [new-mcp]", names)
	}
	if _, err := profile.NewLoader(ProfilesDir).Load("old"); err == nil {
		t.Fatal("old profile was not removed")
	}
	if _, err := prompt.NewManager(filepath.Join(RegistryDir, "prompts")).Load("old"); err == nil {
		t.Fatal("old prompt set was not removed")
	}
}

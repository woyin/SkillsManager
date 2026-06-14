// internal/registry/registry_test.go
package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestRegistry(t *testing.T) string {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "skills", "global"), 0755)
	os.MkdirAll(filepath.Join(dir, "skills", "codex-only"), 0755)
	os.MkdirAll(filepath.Join(dir, "skills", "claude-only"), 0755)
	os.MkdirAll(filepath.Join(dir, "skills", "cloudflare"), 0755)
	os.MkdirAll(filepath.Join(dir, "mcp"), 0755)
	return dir
}

func TestAddSkillFromLocalPath(t *testing.T) {
	regDir := setupTestRegistry(t)
	srcDir := t.TempDir()
	os.MkdirAll(filepath.Join(srcDir, "my-skill"), 0755)
	os.WriteFile(filepath.Join(srcDir, "my-skill", "SKILL.md"), []byte("# test"), 0644)

	reg := New(regDir)
	err := reg.AddSkill(filepath.Join(srcDir, "my-skill"), "cloudflare", "")
	if err != nil {
		t.Fatalf("AddSkill failed: %v", err)
	}

	dest := filepath.Join(regDir, "skills", "cloudflare", "my-skill")
	if _, err := os.Stat(filepath.Join(dest, "SKILL.md")); err != nil {
		t.Errorf("Skill not copied: %v", err)
	}
}

func TestAddSkillGlobal(t *testing.T) {
	regDir := setupTestRegistry(t)
	srcDir := t.TempDir()
	os.MkdirAll(filepath.Join(srcDir, "global-skill"), 0755)
	os.WriteFile(filepath.Join(srcDir, "global-skill", "SKILL.md"), []byte("# global"), 0644)

	reg := New(regDir)
	err := reg.AddSkill(filepath.Join(srcDir, "global-skill"), "", "global")
	if err != nil {
		t.Fatalf("AddSkill failed: %v", err)
	}

	dest := filepath.Join(regDir, "skills", "global", "global-skill")
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("Global skill not created: %v", err)
	}
}

func TestRemoveSkill(t *testing.T) {
	regDir := setupTestRegistry(t)
	skillDir := filepath.Join(regDir, "skills", "cloudflare", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# test"), 0644)

	reg := New(regDir)
	err := reg.RemoveSkill("test-skill", "cloudflare", "")
	if err != nil {
		t.Fatalf("RemoveSkill failed: %v", err)
	}

	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Error("Skill directory should be removed")
	}
}

func TestListSkills(t *testing.T) {
	regDir := setupTestRegistry(t)
	os.MkdirAll(filepath.Join(regDir, "skills", "cloudflare", "skill-a"), 0755)
	os.MkdirAll(filepath.Join(regDir, "skills", "cloudflare", "skill-b"), 0755)
	os.MkdirAll(filepath.Join(regDir, "skills", "global", "skill-c"), 0755)

	reg := New(regDir)
	skills, err := reg.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills failed: %v", err)
	}

	if len(skills["cloudflare"]) != 2 {
		t.Errorf("Expected 2 cloudflare skills, got %d", len(skills["cloudflare"]))
	}
	if len(skills["global"]) != 1 {
		t.Errorf("Expected 1 global skill, got %d", len(skills["global"]))
	}
}

func TestListSkillDetailsIncludesSourceAndLastUpdated(t *testing.T) {
	regDir := setupTestRegistry(t)
	skillDir := filepath.Join(regDir, "skills", "cloudflare", "worker-skill")
	if err := os.MkdirAll(filepath.Join(skillDir, ".git"), 0755); err != nil {
		t.Fatalf("creating git dir: %v", err)
	}
	gitConfig := "[remote \"origin\"]\n\turl = https://github.com/user/worker-skill.git\n"
	if err := os.WriteFile(filepath.Join(skillDir, ".git", "config"), []byte(gitConfig), 0644); err != nil {
		t.Fatalf("writing git config: %v", err)
	}

	reg := New(regDir)
	details, err := reg.ListSkillDetails()
	if err != nil {
		t.Fatalf("ListSkillDetails failed: %v", err)
	}

	items := details["cloudflare"]
	if len(items) != 1 {
		t.Fatalf("Expected one cloudflare skill, got %+v", details)
	}
	item := items[0]
	if item.Name != "worker-skill" || item.Category != "cloudflare" {
		t.Fatalf("Unexpected item identity: %+v", item)
	}
	if item.SourceURL != "https://github.com/user/worker-skill.git" {
		t.Fatalf("Expected source URL, got %+v", item)
	}
	if item.LastUpdated == "" {
		t.Fatalf("Expected last updated timestamp, got %+v", item)
	}
}

func TestAddMCP(t *testing.T) {
	regDir := setupTestRegistry(t)
	srcDir := t.TempDir()
	mcpJSON := `{"mcpServers":{"test":{"type":"http","url":"https://example.com/mcp"}}}`
	os.WriteFile(filepath.Join(srcDir, "test.json"), []byte(mcpJSON), 0644)

	reg := New(regDir)
	err := reg.AddMCP(filepath.Join(srcDir, "test.json"))
	if err != nil {
		t.Fatalf("AddMCP failed: %v", err)
	}

	dest := filepath.Join(regDir, "mcp", "test.json")
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("MCP file not created: %v", err)
	}
	if string(data) != mcpJSON {
		t.Errorf("MCP content mismatch: got %s", string(data))
	}
}

func TestAddMCPCreatesRegistryDirectory(t *testing.T) {
	regDir := t.TempDir()
	srcDir := t.TempDir()
	mcpJSON := `{"mcpServers":{"test":{"type":"http","url":"https://example.com/mcp"}}}`
	os.WriteFile(filepath.Join(srcDir, "test.json"), []byte(mcpJSON), 0644)

	reg := New(regDir)
	err := reg.AddMCP(filepath.Join(srcDir, "test.json"))
	if err != nil {
		t.Fatalf("AddMCP failed: %v", err)
	}

	dest := filepath.Join(regDir, "mcp", "test.json")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("MCP file not created: %v", err)
	}
}

func TestAddMCPFromGitURLClonesAndFindsDefinition(t *testing.T) {
	regDir := setupTestRegistry(t)
	fixtureRepo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fixtureRepo, ".git"), 0755); err != nil {
		t.Fatalf("creating fixture git dir: %v", err)
	}
	mcpJSON := `{"mcpServers":{"browser":{"type":"stdio","command":"browser-mcp"}}}`
	if err := os.WriteFile(filepath.Join(fixtureRepo, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatalf("writing fixture MCP: %v", err)
	}

	binDir := t.TempDir()
	fakeGit := filepath.Join(binDir, "git")
	script := "#!/bin/sh\nif [ \"$1\" = clone ]; then cp -R " + fixtureRepo + " \"$3\"; exit 0; fi\nexit 1\n"
	if err := os.WriteFile(fakeGit, []byte(script), 0755); err != nil {
		t.Fatalf("writing fake git: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	reg := New(regDir)
	if err := reg.AddMCP("github.com/user/browser-mcp"); err != nil {
		t.Fatalf("AddMCP failed: %v", err)
	}

	repoDir := filepath.Join(regDir, "mcp", "browser-mcp")
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err != nil {
		t.Fatalf("MCP git repo should be preserved: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(repoDir, ".mcp.json"))
	if err != nil {
		t.Fatalf("MCP definition not preserved: %v", err)
	}
	if !strings.Contains(string(data), "browser-mcp") {
		t.Fatalf("Unexpected MCP content: %s", string(data))
	}
}

func TestListAndGetMCPFromRepoDirectory(t *testing.T) {
	regDir := setupTestRegistry(t)
	repoDir := filepath.Join(regDir, "mcp", "browser")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("creating repo dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".mcp.json"), []byte(`{"mcpServers":{"browser":{"type":"stdio","command":"browser"}}}`), 0644); err != nil {
		t.Fatalf("writing MCP definition: %v", err)
	}

	reg := New(regDir)
	mcps, err := reg.ListMCP()
	if err != nil {
		t.Fatalf("ListMCP failed: %v", err)
	}
	if len(mcps) != 1 || mcps[0] != "browser" {
		t.Fatalf("Expected repo MCP in list, got %v", mcps)
	}

	path := reg.GetMCPPath("browser")
	if path != filepath.Join(repoDir, ".mcp.json") {
		t.Fatalf("Expected MCP definition path, got %s", path)
	}
}

func TestListMCPDetailsIncludesRepoMetadata(t *testing.T) {
	regDir := setupTestRegistry(t)
	repoDir := filepath.Join(regDir, "mcp", "browser")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0755); err != nil {
		t.Fatalf("creating repo dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".mcp.json"), []byte(`{"mcpServers":{"browser":{"type":"stdio","command":"browser"}}}`), 0644); err != nil {
		t.Fatalf("writing MCP definition: %v", err)
	}
	gitConfig := "[remote \"origin\"]\n\turl = https://github.com/user/browser-mcp.git\n"
	if err := os.WriteFile(filepath.Join(repoDir, ".git", "config"), []byte(gitConfig), 0644); err != nil {
		t.Fatalf("writing git config: %v", err)
	}

	reg := New(regDir)
	items, err := reg.ListMCPDetails()
	if err != nil {
		t.Fatalf("ListMCPDetails failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("Expected one MCP item, got %+v", items)
	}
	if items[0].Name != "browser" || items[0].SourceURL != "https://github.com/user/browser-mcp.git" || items[0].LastUpdated == "" {
		t.Fatalf("Unexpected MCP metadata: %+v", items[0])
	}
}

func TestListMCP(t *testing.T) {
	regDir := setupTestRegistry(t)
	os.WriteFile(filepath.Join(regDir, "mcp", "github.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(regDir, "mcp", "cloudflare.json"), []byte("{}"), 0644)

	reg := New(regDir)
	mcps, err := reg.ListMCP()
	if err != nil {
		t.Fatalf("ListMCP failed: %v", err)
	}
	if len(mcps) != 2 {
		t.Errorf("Expected 2 MCP entries, got %d", len(mcps))
	}
}

func TestIsSpecialDir(t *testing.T) {
	if !IsSpecialDir(Global) {
		t.Error("Expected global to be special")
	}
	if !IsSpecialDir(CodexOnly) {
		t.Error("Expected codex-only to be special")
	}
	if IsSpecialDir("cloudflare") {
		t.Error("Expected cloudflare to not be special")
	}
	if IsSpecialDir("") {
		t.Error("Expected empty string to not be special")
	}
}

func TestDir(t *testing.T) {
	dir := setupTestRegistry(t)
	reg := New(dir)
	if reg.Dir() != dir {
		t.Errorf("Expected Dir() = %s, got %s", dir, reg.Dir())
	}
}

func TestRemoveMCPFile(t *testing.T) {
	dir := setupTestRegistry(t)
	mcpPath := filepath.Join(dir, "mcp", "test-server.json")
	os.WriteFile(mcpPath, []byte(`{"mcpServers":{"test":{"type":"stdio"}}}`), 0644)

	reg := New(dir)
	if err := reg.RemoveMCP("test-server"); err != nil {
		t.Fatalf("RemoveMCP failed: %v", err)
	}
	if _, err := os.Stat(mcpPath); !os.IsNotExist(err) {
		t.Error("Expected MCP file to be removed")
	}
}

func TestRemoveMCPDirectory(t *testing.T) {
	dir := setupTestRegistry(t)
	mcpDir := filepath.Join(dir, "mcp", "my-mcp")
	os.MkdirAll(mcpDir, 0755)
	os.WriteFile(filepath.Join(mcpDir, "mcp.json"), []byte(`{"mcpServers":{"my":{"type":"stdio"}}}`), 0644)

	reg := New(dir)
	if err := reg.RemoveMCP("my-mcp"); err != nil {
		t.Fatalf("RemoveMCP failed: %v", err)
	}
	if _, err := os.Stat(mcpDir); !os.IsNotExist(err) {
		t.Error("Expected MCP directory to be removed")
	}
}

func TestRemoveMCPNotFound(t *testing.T) {
	dir := setupTestRegistry(t)
	reg := New(dir)
	if err := reg.RemoveMCP("nonexistent"); err == nil {
		t.Error("Expected error for non-existent MCP")
	}
}

func TestGetSkillPath(t *testing.T) {
	dir := setupTestRegistry(t)
	skillDir := filepath.Join(dir, "skills", "cloudflare", "my-skill")
	os.MkdirAll(skillDir, 0755)

	reg := New(dir)

	// With category
	path, err := reg.GetSkillPath("my-skill", "cloudflare", "")
	if err != nil {
		t.Fatalf("GetSkillPath failed: %v", err)
	}
	if path != skillDir {
		t.Errorf("Expected %s, got %s", skillDir, path)
	}

	// With special
	path, err = reg.GetSkillPath("skill-c", "cloudflare", "global")
	// This should fail since skill-c is not in global
	if err == nil {
		t.Error("Expected error for skill not in special dir")
	}

	// Search all categories
	path, err = reg.GetSkillPath("my-skill", "", "")
	if err != nil {
		t.Fatalf("GetSkillPath search failed: %v", err)
	}
	if path != skillDir {
		t.Errorf("Expected %s, got %s", skillDir, path)
	}

	// Not found
	_, err = reg.GetSkillPath("nonexistent", "", "")
	if err == nil {
		t.Error("Expected error for non-existent skill")
	}
}

func TestFindSkillDir(t *testing.T) {
	dir := setupTestRegistry(t)
	skillDir := filepath.Join(dir, "skills", "cloudflare", "target-skill")
	os.MkdirAll(skillDir, 0755)

	reg := New(dir)
	found, err := reg.FindSkillDir("target-skill")
	if err != nil {
		t.Fatalf("findSkillDir failed: %v", err)
	}
	if found != skillDir {
		t.Errorf("Expected %s, got %s", skillDir, found)
	}

	found, err = reg.FindSkillDir("nonexistent")
	if err != nil {
		t.Fatalf("findSkillDir failed: %v", err)
	}
	if found != "" {
		t.Errorf("Expected empty for non-existent skill, got %s", found)
	}
}

func TestSkillNameFromPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"github.com/user/repo/my-skill", "my-skill"},
		{"github.com/user/repo/my-skill/", "my-skill"},
		{"my-skill", "my-skill"},
		{"", ""},
	}
	for _, tt := range tests {
		got := SkillNameFromPath(tt.input)
		if got != tt.want {
			t.Errorf("SkillNameFromPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsGitURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"github.com/user/repo", true},
		{"https://github.com/user/repo", true},
		{"git@github.com:user/repo.git", true},
		{"repo.git", true},
		{"./local-path", false},
		{"local-name", false},
	}
	for _, tt := range tests {
		got := IsGitURL(tt.input)
		if got != tt.want {
			t.Errorf("IsGitURL(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestAddMCPSkillDuplicate(t *testing.T) {
	dir := setupTestRegistry(t)
	srcDir := t.TempDir()
	os.MkdirAll(filepath.Join(srcDir, "dupe-skill"), 0755)
	os.WriteFile(filepath.Join(srcDir, "dupe-skill", "SKILL.md"), []byte("#"), 0644)

	reg := New(dir)
	if err := reg.AddSkill(filepath.Join(srcDir, "dupe-skill"), "cloudflare", ""); err != nil {
		t.Fatalf("first AddSkill failed: %v", err)
	}
	// Adding again should fail
	if err := reg.AddSkill(filepath.Join(srcDir, "dupe-skill"), "cloudflare", ""); err == nil {
		t.Error("Expected error for duplicate skill")
	}
}

func TestListMCPDetailsEmpty(t *testing.T) {
	dir := setupTestRegistry(t)
	reg := New(dir)
	details, err := reg.ListMCPDetails()
	if err != nil {
		t.Fatalf("ListMCPDetails failed: %v", err)
	}
	if details == nil {
		t.Error("Expected non-nil empty slice")
	}
	if len(details) != 0 {
		t.Errorf("Expected 0 details, got %d", len(details))
	}
}

func TestRemoveSkillSearchAll(t *testing.T) {
	regDir := setupTestRegistry(t)
	skillDir := filepath.Join(regDir, "skills", "cloudflare", "auto-find")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# test"), 0644)

	reg := New(regDir)
	// No category or special — should search all
	err := reg.RemoveSkill("auto-find", "", "")
	if err != nil {
		t.Fatalf("RemoveSkill search-all failed: %v", err)
	}
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Error("Skill directory should be removed")
	}
}

func TestRemoveSkillSpecialDir(t *testing.T) {
	regDir := setupTestRegistry(t)
	skillDir := filepath.Join(regDir, "skills", "codex-only", "my-skill")
	os.MkdirAll(skillDir, 0755)

	reg := New(regDir)
	err := reg.RemoveSkill("my-skill", "", "codex-only")
	if err != nil {
		t.Fatalf("RemoveSkill with special failed: %v", err)
	}
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Error("Skill directory should be removed")
	}
}

func TestRemoveSkillNotFound(t *testing.T) {
	regDir := setupTestRegistry(t)
	reg := New(regDir)

	err := reg.RemoveSkill("nonexistent", "", "")
	if err == nil {
		t.Error("Expected error for nonexistent skill")
	}
}

func TestRemoveSkillWithCategoryNotFound(t *testing.T) {
	regDir := setupTestRegistry(t)
	reg := New(regDir)

	err := reg.RemoveSkill("nonexistent", "cloudflare", "")
	if err == nil {
		t.Error("Expected error for nonexistent skill in category")
	}
}

func TestFindMCPDefinitionMcpJSON(t *testing.T) {
	dir := t.TempDir()
	validDef := `{"mcpServers": {"test": {"type": "stdio", "command": "test"}}}`
	os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(validDef), 0644)

	path, err := findMCPDefinition(dir)
	if err != nil {
		t.Fatalf("findMCPDefinition failed: %v", err)
	}
	if filepath.Base(path) != "mcp.json" {
		t.Errorf("Expected mcp.json, got %s", filepath.Base(path))
	}
}

func TestFindMCPDefinitionDotMcpJSON(t *testing.T) {
	dir := t.TempDir()
	validDef := `{"mcpServers": {"test": {"type": "stdio", "command": "test"}}}`
	os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(validDef), 0644)

	path, err := findMCPDefinition(dir)
	if err != nil {
		t.Fatalf("findMCPDefinition failed: %v", err)
	}
	if filepath.Base(path) != ".mcp.json" {
		t.Errorf("Expected .mcp.json, got %s", filepath.Base(path))
	}
}

func TestFindMCPDefinitionNestedJSON(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "subdir")
	os.MkdirAll(nested, 0755)
	validDef := `{"mcpServers": {"test": {"type": "stdio", "command": "test"}}}`
	os.WriteFile(filepath.Join(nested, "custom.json"), []byte(validDef), 0644)

	path, err := findMCPDefinition(dir)
	if err != nil {
		t.Fatalf("findMCPDefinition failed: %v", err)
	}
	if filepath.Base(path) != "custom.json" {
		t.Errorf("Expected custom.json, got %s", filepath.Base(path))
	}
}

func TestFindMCPDefinitionNotFound(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not json"), 0644)

	_, err := findMCPDefinition(dir)
	if err == nil {
		t.Error("Expected error when no MCP definition found")
	}
}

func TestFindMCPDefinitionInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{not valid json`), 0644)

	_, err := findMCPDefinition(dir)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestFindMCPDefinitionMissingMcpServers(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{"other": "data"}`), 0644)

	_, err := findMCPDefinition(dir)
	if err == nil {
		t.Error("Expected error for JSON without mcpServers")
	}
}

func TestAddMCPFromDirectory(t *testing.T) {
	regDir := setupTestRegistry(t)
	sourceDir := t.TempDir()
	validDef := `{"mcpServers": {"dir-mcp": {"type": "stdio", "command": "test"}}}`
	os.WriteFile(filepath.Join(sourceDir, ".mcp.json"), []byte(validDef), 0644)

	reg := New(regDir)
	err := reg.AddMCP(sourceDir)
	if err != nil {
		t.Fatalf("AddMCP from directory failed: %v", err)
	}

	// Verify MCP was added
	mcps, _ := reg.ListMCP()
	found := false
	for _, m := range mcps {
		if m == filepath.Base(sourceDir) {
			found = true
		}
	}
	if !found {
		t.Errorf("MCP not found in list: %v", mcps)
	}
}

func TestAddMCPFromJSONFile(t *testing.T) {
	regDir := setupTestRegistry(t)
	sourceFile := filepath.Join(t.TempDir(), "custom-mcp.json")
	validDef := `{"mcpServers": {"custom": {"type": "stdio", "command": "test"}}}`
	os.WriteFile(sourceFile, []byte(validDef), 0644)

	reg := New(regDir)
	err := reg.AddMCP(sourceFile)
	if err != nil {
		t.Fatalf("AddMCP from file failed: %v", err)
	}
}

func TestAddMCPInvalidJSON(t *testing.T) {
	regDir := setupTestRegistry(t)
	sourceFile := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(sourceFile, []byte(`{not json`), 0644)

	reg := New(regDir)
	err := reg.AddMCP(sourceFile)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestAddMCPDirectoryNoDefinition(t *testing.T) {
	regDir := setupTestRegistry(t)
	sourceDir := t.TempDir()
	os.WriteFile(filepath.Join(sourceDir, "readme.txt"), []byte("no mcp here"), 0644)

	reg := New(regDir)
	err := reg.AddMCP(sourceDir)
	if err == nil {
		t.Error("Expected error for directory without MCP definition")
	}
}

func TestListMCPDetailsWithDirectory(t *testing.T) {
	regDir := setupTestRegistry(t)
	mcpDir := filepath.Join(regDir, "mcp", "my-mcp")
	os.MkdirAll(mcpDir, 0755)
	validDef := `{"mcpServers": {"my-mcp": {"type": "stdio", "command": "test"}}}`
	os.WriteFile(filepath.Join(mcpDir, ".mcp.json"), []byte(validDef), 0644)

	reg := New(regDir)
	details, err := reg.ListMCPDetails()
	if err != nil {
		t.Fatalf("ListMCPDetails failed: %v", err)
	}
	found := false
	for _, d := range details {
		if d.Name == "my-mcp" {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected my-mcp in details, got %v", details)
	}
}

func TestNormalizeGitURL(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"github shorthand", "github.com/user/repo", "https://github.com/user/repo"},
		{"already https", "https://github.com/user/repo", "https://github.com/user/repo"},
		{"git protocol", "git@github.com:user/repo.git", "git@github.com:user/repo.git"},
		{"other url", "https://example.com/repo", "https://example.com/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeGitURL(tt.input)
			if result != tt.expect {
				t.Errorf("NormalizeGitURL(%q) = %q, want %q", tt.input, result, tt.expect)
			}
		})
	}
}

// ── Plugin manifest discovery tests ──

func TestDiscoverSkillsPluginManifest(t *testing.T) {
	dir := t.TempDir()

	// Create a plugin with skills referenced in plugin.json
	pluginDir := filepath.Join(dir, ".claude-plugin")
	os.MkdirAll(pluginDir, 0755)

	// Create actual skill directories
	skillDir := filepath.Join(dir, "skills", "review")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: Code review skill\n---\n# Review"), 0644)

	skillDir2 := filepath.Join(dir, "skills", "test")
	os.MkdirAll(skillDir2, 0755)
	os.WriteFile(filepath.Join(skillDir2, "SKILL.md"), []byte("---\ndescription: Testing skill\n---\n# Test"), 0644)

	// Create plugin.json
	pluginJSON := `{
		"name": "my-plugin",
		"source": "my-plugin",
		"skills": ["./skills/review", "./skills/test"]
	}`
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(pluginJSON), 0644)

	skills, err := DiscoverSkills(dir)
	if err != nil {
		t.Fatalf("DiscoverSkills failed: %v", err)
	}

	foundNames := make(map[string]bool)
	for _, s := range skills {
		foundNames[s.Name] = true
	}

	if !foundNames["review"] {
		t.Error("expected to find 'review' skill from plugin.json")
	}
	if !foundNames["test"] {
		t.Error("expected to find 'test' skill from plugin.json")
	}
}

func TestDiscoverSkillsMarketplaceJSON(t *testing.T) {
	dir := t.TempDir()

	// Create marketplace.json with pluginRoot
	pluginDir := filepath.Join(dir, ".claude-plugin")
	os.MkdirAll(pluginDir, 0755)

	// Create skills under plugins/my-plugin/skills/
	skillDir := filepath.Join(dir, "plugins", "my-plugin", "skills", "code-review")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: Code review\n---\n# Code Review"), 0644)

	marketplaceJSON := `{
		"metadata": {
			"pluginRoot": "./plugins"
		},
		"plugins": [
			{
				"name": "my-plugin",
				"source": "my-plugin",
				"skills": ["./my-plugin/skills/code-review"]
			}
		]
	}`
	os.WriteFile(filepath.Join(pluginDir, "marketplace.json"), []byte(marketplaceJSON), 0644)

	skills, err := DiscoverSkills(dir)
	if err != nil {
		t.Fatalf("DiscoverSkills failed: %v", err)
	}

	found := false
	for _, s := range skills {
		if s.Name == "code-review" {
			found = true
			if s.Description != "Code review" {
				t.Errorf("expected description 'Code review', got %q", s.Description)
			}
			break
		}
	}
	if !found {
		t.Error("expected to find 'code-review' skill from marketplace.json")
	}
}

func TestDiscoverSkillsCodexPlugin(t *testing.T) {
	dir := t.TempDir()

	// Create .codex-plugin/plugin.json
	pluginDir := filepath.Join(dir, ".codex-plugin")
	os.MkdirAll(pluginDir, 0755)

	skillDir := filepath.Join(dir, "my-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: A codex skill\n---\n# Skill"), 0644)

	pluginJSON := `{
		"name": "codex-plugin",
		"skills": ["./my-skill"]
	}`
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(pluginJSON), 0644)

	skills, err := DiscoverSkills(dir)
	if err != nil {
		t.Fatalf("DiscoverSkills failed: %v", err)
	}

	found := false
	for _, s := range skills {
		if s.Name == "my-skill" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'my-skill' from .codex-plugin/plugin.json")
	}
}

func TestDiscoverSkillsPluginManifestDeduplication(t *testing.T) {
	dir := t.TempDir()

	// Create a skill in the standard skills/ directory AND referenced in plugin.json
	skillDir := filepath.Join(dir, "skills", "shared-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: Shared skill\n---\n# Shared"), 0644)

	// Also reference it in plugin.json
	pluginDir := filepath.Join(dir, ".claude-plugin")
	os.MkdirAll(pluginDir, 0755)
	pluginJSON := `{
		"name": "my-plugin",
		"skills": ["./skills/shared-skill"]
	}`
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(pluginJSON), 0644)

	skills, err := DiscoverSkills(dir)
	if err != nil {
		t.Fatalf("DiscoverSkills failed: %v", err)
	}

	count := 0
	for _, s := range skills {
		if s.Name == "shared-skill" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 'shared-skill' to appear exactly once, found %d times", count)
	}
}

func TestDiscoverSkillsInvalidPluginJSON(t *testing.T) {
	dir := t.TempDir()

	// Create an invalid plugin.json (should not crash, just skip)
	pluginDir := filepath.Join(dir, ".claude-plugin")
	os.MkdirAll(pluginDir, 0755)
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte("not valid json"), 0644)

	// Should not error
	skills, err := DiscoverSkills(dir)
	if err != nil {
		t.Fatalf("DiscoverSkills should not fail on invalid plugin.json: %v", err)
	}

	// May find the root SKILL.md or nothing
	_ = skills
}

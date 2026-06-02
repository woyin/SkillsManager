// internal/registry/registry_test.go
package registry

import (
	"os"
	"path/filepath"
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

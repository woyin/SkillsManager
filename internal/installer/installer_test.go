// internal/installer/installer_test.go
package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupTestEnv(t *testing.T) (registryDir, profilesDir, codexDir, claudeDir, projectDir string) {
	base := t.TempDir()
	registryDir = filepath.Join(base, "registry")
	profilesDir = filepath.Join(base, "profiles")
	codexDir = filepath.Join(base, ".codex", "skills")
	claudeDir = filepath.Join(base, ".claude", "skills")
	projectDir = filepath.Join(base, "project")

	// Create registry with test skills
	os.MkdirAll(filepath.Join(registryDir, "skills", "global", "global-skill"), 0755)
	os.WriteFile(filepath.Join(registryDir, "skills", "global", "global-skill", "SKILL.md"), []byte("# global"), 0644)
	os.MkdirAll(filepath.Join(registryDir, "skills", "cloudflare", "cf-skill"), 0755)
	os.WriteFile(filepath.Join(registryDir, "skills", "cloudflare", "cf-skill", "SKILL.md"), []byte("# cf"), 0644)
	os.MkdirAll(filepath.Join(registryDir, "skills", "codex-only", "codex-skill"), 0755)
	os.WriteFile(filepath.Join(registryDir, "skills", "codex-only", "codex-skill", "SKILL.md"), []byte("# codex"), 0644)
	os.MkdirAll(filepath.Join(registryDir, "skills", "claude-only", "claude-skill"), 0755)
	os.WriteFile(filepath.Join(registryDir, "skills", "claude-only", "claude-skill", "SKILL.md"), []byte("# claude"), 0644)

	// Create MCP
	mcpJSON := `{"mcpServers":{"test":{"type":"http","url":"https://example.com/mcp"}}}`
	os.MkdirAll(filepath.Join(registryDir, "mcp"), 0755)
	os.WriteFile(filepath.Join(registryDir, "mcp", "test.json"), []byte(mcpJSON), 0644)

	// Create profile
	profileData := map[string]interface{}{
		"skills": []string{"global", "cloudflare"},
		"mcp":    []string{"test"},
	}
	pData, _ := json.Marshal(profileData)
	os.MkdirAll(profilesDir, 0755)
	os.WriteFile(filepath.Join(profilesDir, "cloudflare.json"), pData, 0644)

	// Create target dirs
	os.MkdirAll(codexDir, 0755)
	os.MkdirAll(claudeDir, 0755)
	os.MkdirAll(projectDir, 0755)

	return
}

func TestInstallWithProfile(t *testing.T) {
	registryDir, profilesDir, codexDir, claudeDir, projectDir := setupTestEnv(t)

	inst, err := New(registryDir, profilesDir, codexDir, claudeDir)
	if err != nil {
		t.Fatalf("New installer failed: %v", err)
	}

	result, err := inst.Install(projectDir, "cloudflare", nil, nil)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Check symlinks created
	cfSkillCodex := filepath.Join(codexDir, "cf-skill")
	cfSkillClaude := filepath.Join(claudeDir, "cf-skill")
	globalSkillCodex := filepath.Join(codexDir, "global-skill")
	globalSkillClaude := filepath.Join(claudeDir, "global-skill")

	for _, link := range []string{cfSkillCodex, cfSkillClaude, globalSkillCodex, globalSkillClaude} {
		fi, err := os.Lstat(link)
		if err != nil {
			t.Errorf("Symlink not created: %s (%v)", link, err)
			continue
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("Not a symlink: %s", link)
		}
	}

	// Check .sm.json written
	smPath := filepath.Join(projectDir, ".sm.json")
	data, err := os.ReadFile(smPath)
	if err != nil {
		t.Fatalf(".sm.json not created: %v", err)
	}
	var config map[string]interface{}
	json.Unmarshal(data, &config)
	if config["profile"] != "cloudflare" {
		t.Errorf("Expected profile 'cloudflare' in .sm.json")
	}

	// Check MCP merged into .mcp.json
	mcpPath := filepath.Join(projectDir, ".mcp.json")
	mcpData, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf(".mcp.json not created: %v", err)
	}
	if len(mcpData) == 0 {
		t.Error(".mcp.json is empty")
	}

	// Check result
	if len(result.Skills) != 4 { // global-skill (2 links) + cf-skill (2 links)
		t.Errorf("Expected 4 skill links, got %d", len(result.Skills))
	}
}

func TestInstallWithAdHoc(t *testing.T) {
	registryDir, profilesDir, codexDir, claudeDir, projectDir := setupTestEnv(t)

	inst, err := New(registryDir, profilesDir, codexDir, claudeDir)
	if err != nil {
		t.Fatalf("New installer failed: %v", err)
	}

	result, err := inst.Install(projectDir, "", []string{"cf-skill"}, []string{"test"})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// cf-skill should go to both codex and claude
	if _, err := os.Lstat(filepath.Join(codexDir, "cf-skill")); err != nil {
		t.Error("cf-skill not in codex")
	}
	if _, err := os.Lstat(filepath.Join(claudeDir, "cf-skill")); err != nil {
		t.Error("cf-skill not in claude")
	}

	// MCP should be in .mcp.json
	mcpPath := filepath.Join(projectDir, ".mcp.json")
	if _, err := os.Stat(mcpPath); err != nil {
		t.Error(".mcp.json not created")
	}

	if len(result.MCP) != 1 {
		t.Errorf("Expected 1 MCP, got %d", len(result.MCP))
	}
}

func TestInstallCodexOnly(t *testing.T) {
	registryDir, profilesDir, codexDir, claudeDir, projectDir := setupTestEnv(t)

	inst, err := New(registryDir, profilesDir, codexDir, claudeDir)
	if err != nil {
		t.Fatalf("New installer failed: %v", err)
	}

	// codex-only skill should only appear in codex
	result, err := inst.Install(projectDir, "", []string{"codex-skill"}, nil)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(codexDir, "codex-skill")); err != nil {
		t.Error("codex-skill not in codex")
	}
	if _, err := os.Lstat(filepath.Join(claudeDir, "codex-skill")); err == nil {
		t.Error("codex-skill should NOT be in claude")
	}

	if len(result.Skills) != 1 {
		t.Errorf("Expected 1 skill link, got %d", len(result.Skills))
	}
}

func TestInstallNoConfig(t *testing.T) {
	registryDir, profilesDir, codexDir, claudeDir, projectDir := setupTestEnv(t)

	inst, err := New(registryDir, profilesDir, codexDir, claudeDir)
	if err != nil {
		t.Fatalf("New installer failed: %v", err)
	}

	_, err = inst.Install(projectDir, "", nil, nil)
	if err == nil {
		t.Error("Expected error when no profile and no ad-hoc items")
	}
}

// cmd/init_test.go
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCmdCreatesProjectConfig(t *testing.T) {
	// Test the initProject function in a temp directory
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	initProfile = ""
	err := initProject()
	if err != nil {
		t.Fatalf("initProject failed: %v", err)
	}

	// Check .sm.json was created
	if _, err := os.Stat(filepath.Join(tmpDir, ".sm.json")); err != nil {
		t.Error(".sm.json not created")
	}
}

func TestInitCmdCreatesSkillTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	err := initSkillTemplate("my-test-skill")
	if err != nil {
		t.Fatalf("initSkillTemplate failed: %v", err)
	}

	// Check SKILL.md was created
	skillMD := filepath.Join(tmpDir, "my-test-skill", "SKILL.md")
	data, err := os.ReadFile(skillMD)
	if err != nil {
		t.Fatalf("SKILL.md not created: %v", err)
	}

	content := string(data)
	// Check frontmatter
	if !strings.Contains(content, "name: my-test-skill") {
		t.Error("SKILL.md missing name in frontmatter")
	}
	if !strings.Contains(content, "description:") {
		t.Error("SKILL.md missing description in frontmatter")
	}
	if !strings.HasPrefix(content, "---") {
		t.Error("SKILL.md should start with YAML frontmatter")
	}
}

func TestInitCmdSkillTemplateAlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Create existing SKILL.md
	skillDir := filepath.Join(tmpDir, "existing-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("existing"), 0644)

	err := initSkillTemplate("existing-skill")
	if err == nil {
		t.Error("expected error for existing skill, got nil")
	}
}

// cmd/export_test.go
package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/prompt"
)

func TestExportCmdRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "export" {
			found = true
			break
		}
	}
	if !found {
		t.Error("export command not registered on root command")
	}
}

func TestBuildExportDataIncludesPromptSetsByDefault(t *testing.T) {
	dir := t.TempDir()

	oldReg, oldProf, oldData := RegistryDir, ProfilesDir, DataDir
	RegistryDir = filepath.Join(dir, "registry")
	ProfilesDir = filepath.Join(dir, "profiles")
	DataDir = filepath.Join(dir, "data")
	defer func() {
		RegistryDir, ProfilesDir, DataDir = oldReg, oldProf, oldData
	}()
	for _, path := range []string{
		filepath.Join(RegistryDir, "skills"),
		filepath.Join(RegistryDir, "mcp"),
		ProfilesDir,
	} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatalf("creating %s: %v", path, err)
		}
	}

	manager := prompt.NewManager(filepath.Join(RegistryDir, "prompts"))
	if err := manager.Save(&prompt.PromptSet{
		Name: "default",
		Prompts: map[string]string{
			"AGENTS.md": "# Agents",
		},
	}); err != nil {
		t.Fatalf("saving prompt set: %v", err)
	}

	data, err := buildExportData(parseIncludeFlags(""))
	if err != nil {
		t.Fatalf("buildExportData failed: %v", err)
	}

	if data.Prompts["default"].Prompts["AGENTS.md"] != "# Agents" {
		t.Fatalf("exported prompt content = %q, want %q", data.Prompts["default"].Prompts["AGENTS.md"], "# Agents")
	}
}

func TestImportCmdRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "import" {
			found = true
			break
		}
	}
	if !found {
		t.Error("import command not registered on root command")
	}
}

func TestParseIncludeFlags(t *testing.T) {
	tests := []struct {
		input    string
		expected map[string]bool
	}{
		{"", map[string]bool{"registry": true, "profiles": true, "projects": true, "prompts": true}},
		{"registry", map[string]bool{"registry": true, "profiles": false, "projects": false, "prompts": false}},
		{"registry,profiles", map[string]bool{"registry": true, "profiles": true, "projects": false, "prompts": false}},
		{"projects", map[string]bool{"registry": false, "profiles": false, "projects": true, "prompts": false}},
		{"prompts", map[string]bool{"registry": false, "profiles": false, "projects": false, "prompts": true}},
	}

	for _, tt := range tests {
		result := parseIncludeFlags(tt.input)
		for k, v := range tt.expected {
			if result[k] != v {
				t.Errorf("parseIncludeFlags(%q)[%q] = %v, want %v", tt.input, k, result[k], v)
			}
		}
	}
}

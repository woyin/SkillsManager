// internal/tool/tool_test.go
package tool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAllTools(t *testing.T) {
	tools := AllTools()
	if len(tools) < 6 {
		t.Errorf("expected at least 6 tools, got %d", len(tools))
	}

	// Verify all expected tools are present
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}

	expected := []string{"claude", "codex", "gemini", "opencode", "hermes", "openclaw"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected tool %q not found", name)
		}
	}
}

func TestDefaultTools(t *testing.T) {
	tools := DefaultTools()
	if len(tools) != 2 {
		t.Errorf("expected 2 default tools, got %d", len(tools))
	}

	// Should be Claude and Codex
	if tools[0].Name != "claude" {
		t.Errorf("expected first tool to be 'claude', got %q", tools[0].Name)
	}
	if tools[1].Name != "codex" {
		t.Errorf("expected second tool to be 'codex', got %q", tools[1].Name)
	}
}

func TestToolByName(t *testing.T) {
	tests := []struct {
		name    string
		wantNil bool
	}{
		{"claude", false},
		{"codex", false},
		{"gemini", false},
		{"nonexistent", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := ToolByName(tt.name)
			if tt.wantNil && tool != nil {
				t.Error("expected nil, got tool")
			}
			if !tt.wantNil && tool == nil {
				t.Error("expected tool, got nil")
			}
		})
	}
}

func TestToolsByNames(t *testing.T) {
	names := []string{"claude", "codex", "nonexistent"}
	tools := ToolsByNames(names)

	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
}

func TestGetSkillDir(t *testing.T) {
	tool := Claude
	dir := GetSkillDir(tool)

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".claude", "skills")

	if dir != expected {
		t.Errorf("expected %q, got %q", expected, dir)
	}
}

func TestGetConfigPath(t *testing.T) {
	tool := Claude
	path := GetConfigPath(tool, "/project")

	expected := filepath.Join("/project", "CLAUDE.md")

	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestHasSkillDir(t *testing.T) {
	// This test depends on the actual system state
	// We just verify it doesn't panic
	tool := Claude
	_ = HasSkillDir(tool)
}

func TestIsInstalled(t *testing.T) {
	// This test depends on the actual system state
	// We just verify it doesn't panic
	tool := Claude
	_ = IsInstalled(tool)
}

func TestDetectInstalled(t *testing.T) {
	// Use all tools — at least some should be detected on any dev machine
	all := AllTools()
	installed := DetectInstalled(all)

	// installed should be a subset of all
	if len(installed) > len(all) {
		t.Errorf("Detected more tools than available: %d > %d", len(installed), len(all))
	}

	// Each detected tool should actually be installed
	for _, tool := range installed {
		if !IsInstalled(tool) {
			t.Errorf("Tool %q reported as detected but IsInstalled returns false", tool.Name)
		}
	}
}

func TestDetectInstalledEmpty(t *testing.T) {
	installed := DetectInstalled([]Tool{})
	if len(installed) != 0 {
		t.Errorf("Expected 0 from empty input, got %d", len(installed))
	}
}

// internal/tool/tool_test.go
package tool

import (
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/home"
)

func TestAllTools(t *testing.T) {
	tools := AllTools()
	if len(tools) < 6 {
		t.Errorf("expected at least 6 tools, got %d", len(tools))
	}

	// Verify all expected original tools are present
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

func TestAllToolsHasExtendedAgents(t *testing.T) {
	tools := AllTools()
	if len(tools) < 60 {
		t.Errorf("expected at least 60 tools (extended agent support), got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}

	// Spot-check some extended agents
	extended := []string{"cursor", "cline", "windsurf", "kiro-cli", "github-copilot", "roo", "kilo"}
	for _, name := range extended {
		if !names[name] {
			t.Errorf("expected extended agent %q not found", name)
		}
	}
}

func TestAllToolsUniqueNames(t *testing.T) {
	tools := AllTools()
	names := make(map[string]bool)
	for _, tool := range tools {
		if names[tool.Name] {
			t.Errorf("duplicate tool name: %q", tool.Name)
		}
		names[tool.Name] = true
	}
}

func TestAllToolsUniqueAgentNames(t *testing.T) {
	tools := AllTools()
	agentNames := make(map[string]bool)
	for _, tool := range tools {
		if tool.AgentName == "" {
			t.Errorf("tool %q has empty AgentName", tool.Name)
			continue
		}
		if agentNames[tool.AgentName] {
			t.Errorf("duplicate agent name: %q (tool: %s)", tool.AgentName, tool.Name)
		}
		agentNames[tool.AgentName] = true
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
		{"cursor", false},
		{"cline", false},
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

func TestToolByAgentName(t *testing.T) {
	tests := []struct {
		agent   string
		wantNil bool
	}{
		{"claude-code", false},
		{"codex", false},
		{"cursor", false},
		{"kiro-cli", false},
		{"nonexistent", true},
	}

	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			tool := ToolByAgentName(tt.agent)
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

func TestToolsByNamesWildcard(t *testing.T) {
	tools := ToolsByNames([]string{"*"})
	if len(tools) != len(allTools) {
		t.Errorf("wildcard should return all tools: expected %d, got %d", len(allTools), len(tools))
	}
}

func TestToolsByAgentName(t *testing.T) {
	// ToolsByNames should also match agent names
	tools := ToolsByNames([]string{"claude-code"})
	if len(tools) != 1 {
		t.Errorf("expected 1 tool for agent name 'claude-code', got %d", len(tools))
	}
	if len(tools) > 0 && tools[0].Name != "claude" {
		t.Errorf("expected tool name 'claude', got %q", tools[0].Name)
	}
}

func TestGetSkillDir(t *testing.T) {
	tool := Claude
	dir := GetSkillDir(tool)

	if err := home.Init(); err != nil {
		t.Skip("home directory not available")
	}
	expected := filepath.Join(home.Dir(), ".claude", "skills")

	if dir != expected {
		t.Errorf("expected %q, got %q", expected, dir)
	}
}

func TestGetProjectSkillDir(t *testing.T) {
	dir := GetProjectSkillDir(Codex, "/project")
	expected := filepath.Join("/project", ".agents", "skills")
	if dir != expected {
		t.Errorf("expected %q, got %q", expected, dir)
	}

	// PromptScript has no global dir
	dir = GetProjectSkillDir(PromptScript, "/project")
	expected = filepath.Join("/project", ".agents", "skills")
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

func TestIsInstalledEmptyBinary(t *testing.T) {
	tool := Tool{Name: "test", Binary: ""}
	if IsInstalled(tool) {
		t.Error("tool with empty binary should not be installed")
	}
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

func TestAliasCompleteness(t *testing.T) {
	for _, tt := range allTools {
		t.Run(tt.Name, func(t *testing.T) {
			found := ToolByName(tt.Name)
			if found == nil {
				t.Errorf("ToolByName(%q) returned nil", tt.Name)
			}
		})
	}
}

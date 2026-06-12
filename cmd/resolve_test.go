package cmd

import (
	"testing"

	"github.com/woyin/skills-manager/internal/registry"
)

func TestSpecialFlagsResolve(t *testing.T) {
	tests := []struct {
		name   string
		flags  specialFlags
		expect string
	}{
		{"global", specialFlags{Global: true}, registry.Global},
		{"codex", specialFlags{Codex: true}, registry.CodexOnly},
		{"claude", specialFlags{Claude: true}, registry.ClaudeOnly},
		{"gemini", specialFlags{Gemini: true}, registry.GeminiOnly},
		{"opencode", specialFlags{OpenCode: true}, registry.OpenCodeOnly},
		{"hermes", specialFlags{Hermes: true}, registry.HermesOnly},
		{"openclaw", specialFlags{OpenClaw: true}, registry.OpenClawOnly},
		{"none", specialFlags{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.flags.Resolve()
			if result != tt.expect {
				t.Errorf("Resolve() = %q, want %q", result, tt.expect)
			}
		})
	}
}

func TestSpecialFlagsResolvePriority(t *testing.T) {
	// Global takes precedence over others
	flags := specialFlags{Global: true, Codex: true, Claude: true}
	result := flags.Resolve()
	if result != registry.Global {
		t.Errorf("Expected Global to take priority, got %q", result)
	}
}

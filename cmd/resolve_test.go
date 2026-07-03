package cmd

import (
	"testing"

	"github.com/woyin/skills-manager/internal/registry"
)

// setFlag 返回一个仅置位指定标志的 specialFlags，用于隔离测试单个标志的解析。
func setFlag(flag string) *specialFlags {
	f := newSpecialFlags()
	if flag == "global" {
		f.global = true
		return f
	}
	if b, ok := f.vals[flag]; ok {
		*b = true
	}
	return f
}

func TestSpecialFlagsResolve(t *testing.T) {
	tests := []struct {
		name   string
		flag   string
		expect string
	}{
		{"global", "global", registry.Global},
		{"codex", "codex", registry.CodexOnly},
		{"claude", "claude", registry.ClaudeOnly},
		{"gemini", "gemini", registry.GeminiOnly},
		{"opencode", "opencode", registry.OpenCodeOnly},
		{"hermes", "hermes", registry.HermesOnly},
		{"openclaw", "openclaw", registry.OpenClawOnly},
		{"none", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := setFlag(tt.flag).Resolve()
			if result != tt.expect {
				t.Errorf("Resolve() = %q, want %q", result, tt.expect)
			}
		})
	}
}

func TestSpecialFlagsResolvePriority(t *testing.T) {
	// Global takes precedence over single-tool flags.
	f := newSpecialFlags()
	f.global = true
	*f.vals["codex"] = true
	*f.vals["claude"] = true
	result := f.Resolve()
	if result != registry.Global {
		t.Errorf("Expected Global to take priority, got %q", result)
	}
}

// cmd/resolve.go
package cmd

import "github.com/woyin/skills-manager/internal/registry"

// resolveSpecial returns the special directory name based on the boolean flags.
// Used by add and rm commands.
type specialFlags struct {
	Global    bool
	Codex     bool
	Claude    bool
	Gemini    bool
	OpenCode  bool
	Hermes    bool
	OpenClaw  bool
}

func (f *specialFlags) Resolve() string {
	switch {
	case f.Global:
		return registry.Global
	case f.Codex:
		return registry.CodexOnly
	case f.Claude:
		return registry.ClaudeOnly
	case f.Gemini:
		return registry.GeminiOnly
	case f.OpenCode:
		return registry.OpenCodeOnly
	case f.Hermes:
		return registry.HermesOnly
	case f.OpenClaw:
		return registry.OpenClawOnly
	}
	return ""
}

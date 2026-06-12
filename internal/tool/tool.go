// internal/tool/tool.go
package tool

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Tool represents an AI coding assistant tool
type Tool struct {
	Name       string // Tool name (e.g., "claude", "codex")
	SkillDir   string // Skills directory path
	ConfigFile string // Main config file name (e.g., "CLAUDE.md")
	Binary     string // CLI binary name
}

// Common tool definitions
var (
	Claude = Tool{
		Name:       "claude",
		SkillDir:   filepath.Join(".claude", "skills"),
		ConfigFile: "CLAUDE.md",
		Binary:     "claude",
	}

	Codex = Tool{
		Name:       "codex",
		SkillDir:   filepath.Join(".codex", "skills"),
		ConfigFile: "AGENTS.md",
		Binary:     "codex",
	}

	Gemini = Tool{
		Name:       "gemini",
		SkillDir:   filepath.Join(".gemini", "skills"),
		ConfigFile: "GEMINI.md",
		Binary:     "gemini",
	}

	OpenCode = Tool{
		Name:       "opencode",
		SkillDir:   filepath.Join(".opencode", "skills"),
		ConfigFile: "OPENCODE.md",
		Binary:     "opencode",
	}

	Hermes = Tool{
		Name:       "hermes",
		SkillDir:   filepath.Join(".hermes", "skills"),
		ConfigFile: "HERMES.md",
		Binary:     "hermes",
	}

	OpenClaw = Tool{
		Name:       "openclaw",
		SkillDir:   filepath.Join(".openclaw", "skills"),
		ConfigFile: "OPENCLAW.md",
		Binary:     "openclaw",
	}
)

// AllTools returns all supported tools
func AllTools() []Tool {
	return []Tool{Claude, Codex, Gemini, OpenCode, Hermes, OpenClaw}
}

// DefaultTools returns the default tools (Claude and Codex)
func DefaultTools() []Tool {
	return []Tool{Claude, Codex}
}

// DetectInstalled returns only the tools that are installed on the system
func DetectInstalled(tools []Tool) []Tool {
	var installed []Tool
	for _, tool := range tools {
		if IsInstalled(tool) {
			installed = append(installed, tool)
		}
	}
	return installed
}

// IsInstalled checks if a tool's CLI binary is available in PATH
func IsInstalled(t Tool) bool {
	_, err := exec.LookPath(t.Binary)
	return err == nil
}

// HasSkillDir checks if the tool's skills directory exists
func HasSkillDir(t Tool) bool {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, t.SkillDir)
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// GetSkillDir returns the absolute path to the tool's skills directory
func GetSkillDir(t Tool) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, t.SkillDir)
}

// GetConfigPath returns the path to the tool's config file in a project directory
func GetConfigPath(t Tool, projectDir string) string {
	return filepath.Join(projectDir, t.ConfigFile)
}

// ToolByName returns a tool by its name, or nil if not found
func ToolByName(name string) *Tool {
	tools := AllTools()
	for _, t := range tools {
		if t.Name == name {
			return &t
		}
	}
	return nil
}

// ToolsByNames returns tools matching the given names
func ToolsByNames(names []string) []Tool {
	var tools []Tool
	for _, name := range names {
		if t := ToolByName(name); t != nil {
			tools = append(tools, *t)
		}
	}
	return tools
}

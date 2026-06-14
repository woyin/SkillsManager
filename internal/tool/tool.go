// Package tool describes the catalog of AI coding assistants sm can install
// skills for. The catalog itself lives in data.go; this file holds the Tool
// type, the derived exported tool variables, and the lookup helpers.
package tool

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Tool represents an AI coding assistant tool.
type Tool struct {
	Name            string // Tool name (e.g., "claude", "codex")
	AgentName       string // --agent flag name (e.g., "claude-code", "codex")
	SkillDir        string // Global skills directory path (relative to home)
	ProjectSkillDir string // Project-level skills directory path (relative to project root)
	ConfigFile      string // Main config file name (e.g., "CLAUDE.md")
	Binary          string // CLI binary name
}

// allTools is the materialized catalog; it is the single slice every lookup
// helper iterates. Built once at init time from the catalog in data.go.
var allTools = makeTools()

// toolByName indexes allTools by Tool.Name for O(1) lookups.
var toolByName = func() map[string]*Tool {
	m := make(map[string]*Tool, len(allTools))
	for i := range allTools {
		m[allTools[i].Name] = &allTools[i]
	}
	return m
}()

// toolByAgentName indexes allTools by Tool.AgentName for O(1) lookups.
var toolByAgentName = func() map[string]*Tool {
	m := make(map[string]*Tool, len(allTools))
	for i := range allTools {
		m[allTools[i].AgentName] = &allTools[i]
	}
	return m
}()

// ── Exported tool variables (backward-compatible aliases into allTools) ──
//
// These keep existing callers (cmd, installer, tests) working unchanged.
// They are assigned in init so they share storage with allTools.

var (
	Claude, Codex, Gemini, OpenCode, Hermes, OpenClaw           Tool
	AiderDesk, Amp, Replit, Universal                           Tool
	Antigravity, AntigravityCLI, AstrBot, AutohandCode          Tool
	Augment, Bob, Cline, CodeArtsAgent, CodeBuddy, Codemaker    Tool
	CodeStudio, CommandCode, Continue, Cortex, Crush, Cursor    Tool
	DeepAgents, Devin, Dexto, Droid, Firebender, ForgeCode      Tool
	GitHubCopilot, Goose, IFlowCLI, InferenceSH, Jazz, Junie    Tool
	KiloCode, KimiCodeCLI, KiroCLI, Kode, Lingma, Loaf          Tool
	MCPJam, MistralVibe, Moxby, Mux, Neovate, Ona, OpenHands    Tool
	Pi, Pochi, PromptScript, Qoder, QoderCN, QwenCode, Reasonix Tool
	RovoDev, RooCode, TabnineCLI, Terramind, Tinycloud          Tool
	Trae, TraeCN, Warp, Windsurf, Zed, Zencoder, Zenflow, AdaL  Tool
)

func init() {
	// Point each exported alias at the corresponding catalog row so they
	// stay in sync with allTools and the catalog in data.go. A panic here
	// means the alias list drifted from the catalog — add the missing name.
	aliases := map[string]*Tool{
		"claude": &Claude, "codex": &Codex, "gemini": &Gemini, "opencode": &OpenCode, "hermes": &Hermes, "openclaw": &OpenClaw,
		"aider-desk": &AiderDesk, "amp": &Amp, "replit": &Replit, "universal": &Universal,
		"antigravity": &Antigravity, "antigravity-cli": &AntigravityCLI, "astrbot": &AstrBot, "autohand-code": &AutohandCode,
		"augment": &Augment, "bob": &Bob, "cline": &Cline, "codearts-agent": &CodeArtsAgent, "codebuddy": &CodeBuddy, "codemaker": &Codemaker,
		"codestudio": &CodeStudio, "command-code": &CommandCode, "continue": &Continue, "cortex": &Cortex, "crush": &Crush, "cursor": &Cursor,
		"deepagents": &DeepAgents, "devin": &Devin, "dexto": &Dexto, "droid": &Droid, "firebender": &Firebender, "forgecode": &ForgeCode,
		"github-copilot": &GitHubCopilot, "goose": &Goose, "iflow-cli": &IFlowCLI, "inference-sh": &InferenceSH, "jazz": &Jazz, "junie": &Junie,
		"kilo": &KiloCode, "kimi-code-cli": &KimiCodeCLI, "kiro-cli": &KiroCLI, "kode": &Kode, "lingma": &Lingma, "loaf": &Loaf,
		"mcpjam": &MCPJam, "mistral-vibe": &MistralVibe, "moxby": &Moxby, "mux": &Mux, "neovate": &Neovate, "ona": &Ona, "openhands": &OpenHands,
		"pi": &Pi, "pochi": &Pochi, "promptscript": &PromptScript, "qoder": &Qoder, "qoder-cn": &QoderCN, "qwen-code": &QwenCode, "reasonix": &Reasonix,
		"rovodev": &RovoDev, "roo": &RooCode, "tabnine-cli": &TabnineCLI, "terramind": &Terramind, "tinycloud": &Tinycloud,
		"trae": &Trae, "trae-cn": &TraeCN, "warp": &Warp, "windsurf": &Windsurf, "zed": &Zed, "zencoder": &Zencoder, "zenflow": &Zenflow, "adal": &AdaL,
	}
	for i := range allTools {
		t := &allTools[i]
		alias, ok := aliases[t.Name]
		if !ok {
			panic("tool: exported alias missing for " + t.Name)
		}
		*alias = *t
	}
}

// AllTools returns all supported tools.
func AllTools() []Tool {
	return allTools
}

// DefaultTools returns the default tools (Claude and Codex).
func DefaultTools() []Tool {
	return []Tool{Claude, Codex}
}

// DetectInstalled returns only the tools that are installed on the system.
func DetectInstalled(tools []Tool) []Tool {
	var installed []Tool
	for _, t := range tools {
		if IsInstalled(t) {
			installed = append(installed, t)
		}
	}
	return installed
}

// IsInstalled checks if a tool's CLI binary is available in PATH.
func IsInstalled(t Tool) bool {
	if t.Binary == "" {
		return false
	}
	_, err := exec.LookPath(t.Binary)
	return err == nil
}

// HasSkillDir checks if the tool's global skills directory exists.
func HasSkillDir(t Tool) bool {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, t.SkillDir)
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// GetSkillDir returns the absolute path to the tool's global skills directory.
func GetSkillDir(t Tool) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, t.SkillDir)
}

// GetProjectSkillDir returns the absolute path to the tool's project-level
// skills directory under projectDir.
func GetProjectSkillDir(t Tool, projectDir string) string {
	if t.ProjectSkillDir == "" {
		return ""
	}
	return filepath.Join(projectDir, t.ProjectSkillDir)
}

// GetConfigPath returns the path to the tool's config file in a project
// directory, or "" if the tool has no config file.
func GetConfigPath(t Tool, projectDir string) string {
	if t.ConfigFile == "" {
		return ""
	}
	return filepath.Join(projectDir, t.ConfigFile)
}

// ToolByName returns a pointer to the tool with the given name, or nil.
func ToolByName(name string) *Tool {
	return toolByName[name]
}

// ToolByAgentName returns a pointer to the tool with the given --agent flag
// name, or nil.
func ToolByAgentName(agentName string) *Tool {
	return toolByAgentName[agentName]
}

// ToolsByNames returns tools matching the given names (by Tool.Name or
// Tool.AgentName). A single "*" returns every tool.
func ToolsByNames(names []string) []Tool {
	var tools []Tool
	for _, name := range names {
		if name == "*" {
			return allTools
		}
		if t := ToolByName(name); t != nil {
			tools = append(tools, *t)
		} else if t := ToolByAgentName(name); t != nil {
			tools = append(tools, *t)
		}
	}
	return tools
}

// specialToolByDir maps a registry special-directory name (e.g. "codex-only")
// to the single tool that directory targets. This is the single source of
// truth shared with the installer and the cmd/resolve flag table; "global"
// (and any non-special category) is intentionally absent because it targets
// all tools rather than one.
var specialToolByDir = map[string]string{
	"codex-only":    "codex",
	"claude-only":   "claude",
	"gemini-only":   "gemini",
	"opencode-only": "opencode",
	"hermes-only":   "hermes",
	"openclaw-only": "openclaw",
}

// NameForSpecialDir returns the tool name targeted by a registry special
// directory (e.g. "codex-only" → "codex"), or "" if category is not a
// single-tool special directory (e.g. "global" or a custom category).
func NameForSpecialDir(specialDir string) string {
	return specialToolByDir[specialDir]
}

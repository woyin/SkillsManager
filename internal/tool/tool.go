// internal/tool/tool.go
package tool

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Tool represents an AI coding assistant tool
type Tool struct {
	Name            string // Tool name (e.g., "claude", "codex")
	AgentName       string // --agent flag name (e.g., "claude-code", "codex")
	SkillDir        string // Global skills directory path (relative to home)
	ProjectSkillDir string // Project-level skills directory path (relative to project root)
	ConfigFile      string // Main config file name (e.g., "CLAUDE.md")
	Binary          string // CLI binary name
}

// ── Original tool definitions (backward compatible) ──

var (
	Claude = Tool{
		Name: "claude", AgentName: "claude-code",
		SkillDir: filepath.Join(".claude", "skills"), ProjectSkillDir: filepath.Join(".claude", "skills"),
		ConfigFile: "CLAUDE.md", Binary: "claude",
	}

	Codex = Tool{
		Name: "codex", AgentName: "codex",
		SkillDir: filepath.Join(".codex", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "AGENTS.md", Binary: "codex",
	}

	Gemini = Tool{
		Name: "gemini", AgentName: "gemini-cli",
		SkillDir: filepath.Join(".gemini", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "GEMINI.md", Binary: "gemini",
	}

	OpenCode = Tool{
		Name: "opencode", AgentName: "opencode",
		SkillDir: filepath.Join(".config", "opencode", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "OPENCODE.md", Binary: "opencode",
	}

	Hermes = Tool{
		Name: "hermes", AgentName: "hermes-agent",
		SkillDir: filepath.Join(".hermes", "skills"), ProjectSkillDir: filepath.Join(".hermes", "skills"),
		ConfigFile: "HERMES.md", Binary: "hermes",
	}

	OpenClaw = Tool{
		Name: "openclaw", AgentName: "openclaw",
		SkillDir: filepath.Join(".openclaw", "skills"), ProjectSkillDir: "skills",
		ConfigFile: "OPENCLAW.md", Binary: "openclaw",
	}
)

// ── Additional tool definitions from vercel-labs/skills ──

var (
	AiderDesk = Tool{
		Name: "aider-desk", AgentName: "aider-desk",
		SkillDir: filepath.Join(".aider-desk", "skills"), ProjectSkillDir: filepath.Join(".aider-desk", "skills"),
		ConfigFile: "", Binary: "aider-desk",
	}
	Amp = Tool{
		Name: "amp", AgentName: "amp",
		SkillDir: filepath.Join(".config", "agents", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "", Binary: "amp",
	}
	Replit = Tool{
		Name: "replit", AgentName: "replit",
		SkillDir: filepath.Join(".config", "agents", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "", Binary: "",
	}
	Universal = Tool{
		Name: "universal", AgentName: "universal",
		SkillDir: filepath.Join(".config", "agents", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "", Binary: "",
	}
	Antigravity = Tool{
		Name: "antigravity", AgentName: "antigravity",
		SkillDir: filepath.Join(".gemini", "antigravity", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "", Binary: "",
	}
	AntigravityCLI = Tool{
		Name: "antigravity-cli", AgentName: "antigravity-cli",
		SkillDir: filepath.Join(".gemini", "antigravity-cli", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "", Binary: "",
	}
	AstrBot = Tool{
		Name: "astrbot", AgentName: "astrbot",
		SkillDir: filepath.Join(".astrbot", "data", "skills"), ProjectSkillDir: filepath.Join("data", "skills"),
		ConfigFile: "", Binary: "",
	}
	AutohandCode = Tool{
		Name: "autohand-code", AgentName: "autohand-code",
		SkillDir: filepath.Join(".autohand", "skills"), ProjectSkillDir: filepath.Join(".autohand", "skills"),
		ConfigFile: "", Binary: "",
	}
	Augment = Tool{
		Name: "augment", AgentName: "augment",
		SkillDir: filepath.Join(".augment", "skills"), ProjectSkillDir: filepath.Join(".augment", "skills"),
		ConfigFile: "", Binary: "",
	}
	Bob = Tool{
		Name: "bob", AgentName: "bob",
		SkillDir: filepath.Join(".bob", "skills"), ProjectSkillDir: filepath.Join(".bob", "skills"),
		ConfigFile: "", Binary: "",
	}
	Cline = Tool{
		Name: "cline", AgentName: "cline",
		SkillDir: filepath.Join(".agents", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "", Binary: "",
	}
	CodeArtsAgent = Tool{
		Name: "codearts-agent", AgentName: "codearts-agent",
		SkillDir: filepath.Join(".codeartsdoer", "skills"), ProjectSkillDir: filepath.Join(".codeartsdoer", "skills"),
		ConfigFile: "", Binary: "",
	}
	CodeBuddy = Tool{
		Name: "codebuddy", AgentName: "codebuddy",
		SkillDir: filepath.Join(".codebuddy", "skills"), ProjectSkillDir: filepath.Join(".codebuddy", "skills"),
		ConfigFile: "", Binary: "",
	}
	Codemaker = Tool{
		Name: "codemaker", AgentName: "codemaker",
		SkillDir: filepath.Join(".codemaker", "skills"), ProjectSkillDir: filepath.Join(".codemaker", "skills"),
		ConfigFile: "", Binary: "",
	}
	CodeStudio = Tool{
		Name: "codestudio", AgentName: "codestudio",
		SkillDir: filepath.Join(".codestudio", "skills"), ProjectSkillDir: filepath.Join(".codestudio", "skills"),
		ConfigFile: "", Binary: "",
	}
	CommandCode = Tool{
		Name: "command-code", AgentName: "command-code",
		SkillDir: filepath.Join(".commandcode", "skills"), ProjectSkillDir: filepath.Join(".commandcode", "skills"),
		ConfigFile: "", Binary: "",
	}
	Continue = Tool{
		Name: "continue", AgentName: "continue",
		SkillDir: filepath.Join(".continue", "skills"), ProjectSkillDir: filepath.Join(".continue", "skills"),
		ConfigFile: "", Binary: "",
	}
	Cortex = Tool{
		Name: "cortex", AgentName: "cortex",
		SkillDir: filepath.Join(".snowflake", "cortex", "skills"), ProjectSkillDir: filepath.Join(".cortex", "skills"),
		ConfigFile: "", Binary: "",
	}
	Crush = Tool{
		Name: "crush", AgentName: "crush",
		SkillDir: filepath.Join(".config", "crush", "skills"), ProjectSkillDir: filepath.Join(".crush", "skills"),
		ConfigFile: "", Binary: "",
	}
	Cursor = Tool{
		Name: "cursor", AgentName: "cursor",
		SkillDir: filepath.Join(".cursor", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "", Binary: "cursor",
	}
	DeepAgents = Tool{
		Name: "deepagents", AgentName: "deepagents",
		SkillDir: filepath.Join(".deepagents", "agent", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "", Binary: "",
	}
	Devin = Tool{
		Name: "devin", AgentName: "devin",
		SkillDir: filepath.Join(".config", "devin", "skills"), ProjectSkillDir: filepath.Join(".devin", "skills"),
		ConfigFile: "", Binary: "",
	}
	Dexto = Tool{
		Name: "dexto", AgentName: "dexto",
		SkillDir: filepath.Join(".agents", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "", Binary: "",
	}
	Droid = Tool{
		Name: "droid", AgentName: "droid",
		SkillDir: filepath.Join(".factory", "skills"), ProjectSkillDir: filepath.Join(".factory", "skills"),
		ConfigFile: "", Binary: "",
	}
	Firebender = Tool{
		Name: "firebender", AgentName: "firebender",
		SkillDir: filepath.Join(".firebender", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "", Binary: "",
	}
	ForgeCode = Tool{
		Name: "forgecode", AgentName: "forgecode",
		SkillDir: filepath.Join(".forge", "skills"), ProjectSkillDir: filepath.Join(".forge", "skills"),
		ConfigFile: "", Binary: "",
	}
	GitHubCopilot = Tool{
		Name: "github-copilot", AgentName: "github-copilot",
		SkillDir: filepath.Join(".copilot", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "", Binary: "",
	}
	Goose = Tool{
		Name: "goose", AgentName: "goose",
		SkillDir: filepath.Join(".config", "goose", "skills"), ProjectSkillDir: filepath.Join(".goose", "skills"),
		ConfigFile: "", Binary: "goose",
	}
	IFlowCLI = Tool{
		Name: "iflow-cli", AgentName: "iflow-cli",
		SkillDir: filepath.Join(".iflow", "skills"), ProjectSkillDir: filepath.Join(".iflow", "skills"),
		ConfigFile: "", Binary: "",
	}
	InferenceSH = Tool{
		Name: "inference-sh", AgentName: "inference-sh",
		SkillDir: filepath.Join(".inferencesh", "skills"), ProjectSkillDir: filepath.Join(".inferencesh", "skills"),
		ConfigFile: "", Binary: "",
	}
	Jazz = Tool{
		Name: "jazz", AgentName: "jazz",
		SkillDir: filepath.Join(".jazz", "skills"), ProjectSkillDir: filepath.Join(".jazz", "skills"),
		ConfigFile: "", Binary: "",
	}
	Junie = Tool{
		Name: "junie", AgentName: "junie",
		SkillDir: filepath.Join(".junie", "skills"), ProjectSkillDir: filepath.Join(".junie", "skills"),
		ConfigFile: "", Binary: "",
	}
	KiloCode = Tool{
		Name: "kilo", AgentName: "kilo",
		SkillDir: filepath.Join(".kilocode", "skills"), ProjectSkillDir: filepath.Join(".kilocode", "skills"),
		ConfigFile: "", Binary: "",
	}
	KimiCodeCLI = Tool{
		Name: "kimi-code-cli", AgentName: "kimi-code-cli",
		SkillDir: filepath.Join(".agents", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "", Binary: "",
	}
	KiroCLI = Tool{
		Name: "kiro-cli", AgentName: "kiro-cli",
		SkillDir: filepath.Join(".kiro", "skills"), ProjectSkillDir: filepath.Join(".kiro", "skills"),
		ConfigFile: "", Binary: "",
	}
	Kode = Tool{
		Name: "kode", AgentName: "kode",
		SkillDir: filepath.Join(".kode", "skills"), ProjectSkillDir: filepath.Join(".kode", "skills"),
		ConfigFile: "", Binary: "",
	}
	Lingma = Tool{
		Name: "lingma", AgentName: "lingma",
		SkillDir: filepath.Join(".lingma", "skills"), ProjectSkillDir: filepath.Join(".lingma", "skills"),
		ConfigFile: "", Binary: "",
	}
	Loaf = Tool{
		Name: "loaf", AgentName: "loaf",
		SkillDir: filepath.Join(".agents", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "", Binary: "",
	}
	MCPJam = Tool{
		Name: "mcpjam", AgentName: "mcpjam",
		SkillDir: filepath.Join(".mcpjam", "skills"), ProjectSkillDir: filepath.Join(".mcpjam", "skills"),
		ConfigFile: "", Binary: "",
	}
	MistralVibe = Tool{
		Name: "mistral-vibe", AgentName: "mistral-vibe",
		SkillDir: filepath.Join(".vibe", "skills"), ProjectSkillDir: filepath.Join(".vibe", "skills"),
		ConfigFile: "", Binary: "",
	}
	Moxby = Tool{
		Name: "moxby", AgentName: "moxby",
		SkillDir: filepath.Join(".moxby", "skills"), ProjectSkillDir: filepath.Join(".moxby", "skills"),
		ConfigFile: "", Binary: "",
	}
	Mux = Tool{
		Name: "mux", AgentName: "mux",
		SkillDir: filepath.Join(".mux", "skills"), ProjectSkillDir: filepath.Join(".mux", "skills"),
		ConfigFile: "", Binary: "",
	}
	Neovate = Tool{
		Name: "neovate", AgentName: "neovate",
		SkillDir: filepath.Join(".neovate", "skills"), ProjectSkillDir: filepath.Join(".neovate", "skills"),
		ConfigFile: "", Binary: "",
	}
	Ona = Tool{
		Name: "ona", AgentName: "ona",
		SkillDir: filepath.Join(".ona", "skills"), ProjectSkillDir: filepath.Join(".ona", "skills"),
		ConfigFile: "", Binary: "",
	}
	OpenHands = Tool{
		Name: "openhands", AgentName: "openhands",
		SkillDir: filepath.Join(".openhands", "skills"), ProjectSkillDir: filepath.Join(".openhands", "skills"),
		ConfigFile: "", Binary: "",
	}
	Pi = Tool{
		Name: "pi", AgentName: "pi",
		SkillDir: filepath.Join(".pi", "agent", "skills"), ProjectSkillDir: filepath.Join(".pi", "skills"),
		ConfigFile: "", Binary: "",
	}
	Pochi = Tool{
		Name: "pochi", AgentName: "pochi",
		SkillDir: filepath.Join(".pochi", "skills"), ProjectSkillDir: filepath.Join(".pochi", "skills"),
		ConfigFile: "", Binary: "",
	}
	PromptScript = Tool{
		Name: "promptscript", AgentName: "promptscript",
		SkillDir: "", ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "", Binary: "",
	}
	Qoder = Tool{
		Name: "qoder", AgentName: "qoder",
		SkillDir: filepath.Join(".qoder", "skills"), ProjectSkillDir: filepath.Join(".qoder", "skills"),
		ConfigFile: "", Binary: "",
	}
	QoderCN = Tool{
		Name: "qoder-cn", AgentName: "qoder-cn",
		SkillDir: filepath.Join(".qoder-cn", "skills"), ProjectSkillDir: filepath.Join(".qoder", "skills"),
		ConfigFile: "", Binary: "",
	}
	QwenCode = Tool{
		Name: "qwen-code", AgentName: "qwen-code",
		SkillDir: filepath.Join(".qwen", "skills"), ProjectSkillDir: filepath.Join(".qwen", "skills"),
		ConfigFile: "", Binary: "",
	}
	Reasonix = Tool{
		Name: "reasonix", AgentName: "reasonix",
		SkillDir: filepath.Join(".reasonix", "skills"), ProjectSkillDir: filepath.Join(".reasonix", "skills"),
		ConfigFile: "", Binary: "",
	}
	RovoDev = Tool{
		Name: "rovodev", AgentName: "rovodev",
		SkillDir: filepath.Join(".rovodev", "skills"), ProjectSkillDir: filepath.Join(".rovodev", "skills"),
		ConfigFile: "", Binary: "",
	}
	RooCode = Tool{
		Name: "roo", AgentName: "roo",
		SkillDir: filepath.Join(".roo", "skills"), ProjectSkillDir: filepath.Join(".roo", "skills"),
		ConfigFile: "", Binary: "",
	}
	TabnineCLI = Tool{
		Name: "tabnine-cli", AgentName: "tabnine-cli",
		SkillDir: filepath.Join(".tabnine", "agent", "skills"), ProjectSkillDir: filepath.Join(".tabnine", "agent", "skills"),
		ConfigFile: "", Binary: "",
	}
	Terramind = Tool{
		Name: "terramind", AgentName: "terramind",
		SkillDir: filepath.Join(".terramind", "skills"), ProjectSkillDir: filepath.Join(".terramind", "skills"),
		ConfigFile: "", Binary: "",
	}
	Tinycloud = Tool{
		Name: "tinycloud", AgentName: "tinycloud",
		SkillDir: filepath.Join(".tinycloud", "skills"), ProjectSkillDir: filepath.Join(".tinycloud", "skills"),
		ConfigFile: "", Binary: "",
	}
	Trae = Tool{
		Name: "trae", AgentName: "trae",
		SkillDir: filepath.Join(".trae", "skills"), ProjectSkillDir: filepath.Join(".trae", "skills"),
		ConfigFile: "", Binary: "",
	}
	TraeCN = Tool{
		Name: "trae-cn", AgentName: "trae-cn",
		SkillDir: filepath.Join(".trae-cn", "skills"), ProjectSkillDir: filepath.Join(".trae", "skills"),
		ConfigFile: "", Binary: "",
	}
	Warp = Tool{
		Name: "warp", AgentName: "warp",
		SkillDir: filepath.Join(".agents", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "", Binary: "",
	}
	Windsurf = Tool{
		Name: "windsurf", AgentName: "windsurf",
		SkillDir: filepath.Join(".codeium", "windsurf", "skills"), ProjectSkillDir: filepath.Join(".windsurf", "skills"),
		ConfigFile: "", Binary: "",
	}
	Zed = Tool{
		Name: "zed", AgentName: "zed",
		SkillDir: filepath.Join(".agents", "skills"), ProjectSkillDir: filepath.Join(".agents", "skills"),
		ConfigFile: "", Binary: "zed",
	}
	Zencoder = Tool{
		Name: "zencoder", AgentName: "zencoder",
		SkillDir: filepath.Join(".zencoder", "skills"), ProjectSkillDir: filepath.Join(".zencoder", "skills"),
		ConfigFile: "", Binary: "",
	}
	Zenflow = Tool{
		Name: "zenflow", AgentName: "zenflow",
		SkillDir: filepath.Join(".zencoder", "skills"), ProjectSkillDir: filepath.Join(".zencoder", "skills"),
		ConfigFile: "", Binary: "",
	}
	AdaL = Tool{
		Name: "adal", AgentName: "adal",
		SkillDir: filepath.Join(".adal", "skills"), ProjectSkillDir: filepath.Join(".adal", "skills"),
		ConfigFile: "", Binary: "",
	}
)

// allTools is the complete list of all supported tools
var allTools = []Tool{
	// Original 6
	Claude, Codex, Gemini, OpenCode, Hermes, OpenClaw,
	// Additional agents from vercel-labs/skills
	AiderDesk, Amp, Replit, Universal, Antigravity, AntigravityCLI,
	AstrBot, AutohandCode, Augment, Bob, Cline, CodeArtsAgent,
	CodeBuddy, Codemaker, CodeStudio, CommandCode, Continue, Cortex,
	Crush, Cursor, DeepAgents, Devin, Dexto, Droid, Firebender,
	ForgeCode, GitHubCopilot, Goose, IFlowCLI, InferenceSH, Jazz,
	Junie, KiloCode, KimiCodeCLI, KiroCLI, Kode, Lingma, Loaf,
	MCPJam, MistralVibe, Moxby, Mux, Neovate, Ona, OpenHands, Pi,
	Pochi, PromptScript, Qoder, QoderCN, QwenCode, Reasonix,
	RovoDev, RooCode, TabnineCLI, Terramind, Tinycloud, Trae, TraeCN,
	Warp, Windsurf, Zed, Zencoder, Zenflow, AdaL,
}

// AllTools returns all supported tools
func AllTools() []Tool {
	return allTools
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
	if t.Binary == "" {
		return false
	}
	_, err := exec.LookPath(t.Binary)
	return err == nil
}

// HasSkillDir checks if the tool's global skills directory exists
func HasSkillDir(t Tool) bool {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, t.SkillDir)
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// GetSkillDir returns the absolute path to the tool's global skills directory
func GetSkillDir(t Tool) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, t.SkillDir)
}

// GetProjectSkillDir returns the absolute path to the tool's project-level skills directory
func GetProjectSkillDir(t Tool, projectDir string) string {
	if t.ProjectSkillDir == "" {
		return ""
	}
	return filepath.Join(projectDir, t.ProjectSkillDir)
}

// GetConfigPath returns the path to the tool's config file in a project directory
func GetConfigPath(t Tool, projectDir string) string {
	if t.ConfigFile == "" {
		return ""
	}
	return filepath.Join(projectDir, t.ConfigFile)
}

// ToolByName returns a tool by its name, or nil if not found
func ToolByName(name string) *Tool {
	for _, t := range allTools {
		if t.Name == name {
			cp := t
			return &cp
		}
	}
	return nil
}

// ToolByAgentName returns a tool by its --agent flag name, or nil if not found
func ToolByAgentName(agentName string) *Tool {
	for _, t := range allTools {
		if t.AgentName == agentName {
			cp := t
			return &cp
		}
	}
	return nil
}

// ToolsByNames returns tools matching the given names
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

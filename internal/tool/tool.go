// Package tool 描述 sm 支持的 AI 编程助手目录。
//
// 目录本身定义在 data.go（单一来源），本文件包含 Tool 类型、由目录派生
// 的导出工具变量，以及各种查找辅助函数。
//
// 单一来源契约：添加新工具只需在 data.go 增加一行；导出变量（Claude、
// Codex 等）与 allTools 切片在 init 时由目录派生，避免目录与别名漂移。
//
// Input: os, os/exec, path/filepath, github.com/woyin/skills-manager/internal/home
// Output: type Tool, type SpecialFlagSpec, func AllTools, func DefaultTools, func DetectInstalled, func IsInstalled, func HasSkillDir, func GetSkillDir, func GetProjectSkillDir, func ToolByName, func ToolByAgentName, func ToolsByNames, func NameForSpecialDir, func SpecialFlagSpecs
// Pos: 工具层-agent目录配置
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package tool

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/woyin/skills-manager/internal/home"
)

// Tool 描述一个 AI 编程助手。
type Tool struct {
	Name            string // 工具名（如 "claude"、"codex"）
	AgentName       string // --agent 标志名（如 "claude-code"、"codex"）
	SkillDir        string // 全局技能目录（相对 home）
	ProjectSkillDir string // 项目级技能目录（相对项目根）
	ConfigFile      string // 主配置文件名（如 "CLAUDE.md"）
	Binary          string // CLI 二进制名
	SpecialDir      string // 非空 = 对应注册表特殊目录（如 "codex-only"）；空串表示无单工具特殊目录
}

// allTools 是物化的目录；所有查找函数都遍历它。
// 在 init 时由 data.go 的 catalog 一次性构建。
var allTools = makeTools()

// toolByName 按 Tool.Name 索引 allTools，实现 O(1) 查找。
var toolByName = func() map[string]*Tool {
	m := make(map[string]*Tool, len(allTools))
	for i := range allTools {
		m[allTools[i].Name] = &allTools[i]
	}
	return m
}()

// toolByAgentName 按 Tool.AgentName 索引 allTools，实现 O(1) 查找。
var toolByAgentName = func() map[string]*Tool {
	m := make(map[string]*Tool, len(allTools))
	for i := range allTools {
		m[allTools[i].AgentName] = &allTools[i]
	}
	return m
}()

// ── 导出工具变量（向后兼容的别名，指向 allTools 的对应行） ──
//
// 这些变量让既有调用方（cmd、installer、测试）无需改动。
// 它们在 init 中被赋值，与 allTools 共享存储。
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
	// 把每个导出别名指向目录中对应行，保持与 allTools 及 data.go 同步。
	// 若此处 panic，说明别名清单与目录漂移 —— 需补齐缺失的名称。
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

// AllTools 返回全部支持的工具。
func AllTools() []Tool {
	return allTools
}

// DefaultTools 返回默认工具集合（Claude、Codex、Pi）。
// 用作 DetectInstalled 一无所获时的回退目标；其项目级 skill 目录分别是
// .claude/skills、.agents/skills、.pi/skills。
// 注意：Pi 无 binary，DetectInstalled 永远检测不到它，只有走回退集
// 或用户显式 -a pi 时才会被纳入。
func DefaultTools() []Tool {
	return []Tool{Claude, Codex, Pi}
}

// DetectInstalled 从给定工具集合中筛出本机已安装的。
func DetectInstalled(tools []Tool) []Tool {
	var installed []Tool
	for _, t := range tools {
		if IsInstalled(t) {
			installed = append(installed, t)
		}
	}
	return installed
}

// IsInstalled 判断工具的 CLI 二进制是否在 PATH 中可被发现。
func IsInstalled(t Tool) bool {
	if t.Binary == "" {
		return false
	}
	_, err := exec.LookPath(t.Binary)
	return err == nil
}

// HasSkillDir 判断工具的全局技能目录是否存在。
func HasSkillDir(t Tool) bool {
	dir := filepath.Join(home.Dir(), t.SkillDir)
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// GetSkillDir 返回工具的全局技能目录绝对路径。
func GetSkillDir(t Tool) string {
	return filepath.Join(home.Dir(), t.SkillDir)
}

// GetProjectSkillDir 返回工具在 projectDir 下的项目级技能目录绝对路径。
// 工具未配置项目级目录时返回空串。
func GetProjectSkillDir(t Tool, projectDir string) string {
	if t.ProjectSkillDir == "" {
		return ""
	}
	return filepath.Join(projectDir, t.ProjectSkillDir)
}

// GetConfigPath 返回工具在 projectDir 下的配置文件路径；无配置文件则返回空串。
func GetConfigPath(t Tool, projectDir string) string {
	if t.ConfigFile == "" {
		return ""
	}
	return filepath.Join(projectDir, t.ConfigFile)
}

// ToolByName 按名称返回工具指针；未找到返回 nil。
func ToolByName(name string) *Tool {
	return toolByName[name]
}

// ToolByAgentName 按 --agent 标志名返回工具指针；未找到返回 nil。
func ToolByAgentName(agentName string) *Tool {
	return toolByAgentName[agentName]
}

// ToolsByNames 按名称列表（Tool.Name 或 Tool.AgentName）返回工具。
// 单个 "*" 表示返回全部工具。
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

// specialToolByDir 把注册表特殊目录名（如 "codex-only"）映射到它唯一
// 目标的工具名。从 catalog 的 SpecialDir 字段派生，是单一真相来源的下游视图；
// "global"（及其它非特殊分类）刻意不在表中，因为它们目标为全部工具。
var specialToolByDir = func() map[string]string {
	m := make(map[string]string, len(allTools))
	for _, t := range allTools {
		if t.SpecialDir != "" {
			m[t.SpecialDir] = t.Name
		}
	}
	return m
}()

// NameForSpecialDir 返回特殊目录所针对的工具名（如 "codex-only" → "codex"）。
// 非单工具特殊目录（global 或自定义分类）返回空串。
func NameForSpecialDir(specialDir string) string {
	return specialToolByDir[specialDir]
}

// SpecialFlagSpec 描述一个 --<agent> 特殊目录标志。
type SpecialFlagSpec struct {
	Flag       string // cobra 标志名（如 "codex"），取自工具 catalog
	SpecialDir string // 对应注册表特殊目录（如 "codex-only"）
}

// SpecialFlagSpecs 返回全部单工具特殊目录标志（不含 global）。
// 供 cmd/specialFlags 在运行时派生标志集合，避免重复维护一份硬编码清单。
// 顺序遵循 catalog，保证 Bind/Resolve 的行为稳定可预期。
func SpecialFlagSpecs() []SpecialFlagSpec {
	specs := make([]SpecialFlagSpec, 0, len(specialToolByDir))
	for _, t := range allTools {
		if t.SpecialDir != "" {
			specs = append(specs, SpecialFlagSpec{Flag: t.Name, SpecialDir: t.SpecialDir})
		}
	}
	return specs
}

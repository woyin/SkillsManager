// Package tool 的 data.go 是工具目录的唯一真相来源。
//
// 每新增一个工具只需在 catalog 中追加一行；导出变量（Claude、Codex 等）
// 与 allTools 切片在 init 时由 catalog 派生。
//
// 字段说明（与 Tool 结构一一对应）：
//   - name / agentName：标识与 --agent 标志名；
//   - skillDir / projectSkillDir：相对 home / 项目根的技能目录；
//   - configFile：主配置文件名（可空）；
//   - binary：CLI 二进制名（用于 IsInstalled 检测，可空）。
//
// Input: path/filepath
// Output: 无导出（内部 catalog 数据源）
// Pos: 工具层-agent目录数据
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package tool

import "path/filepath"

// toolDef is one row of the tool catalog. The fields mirror Tool, with
// SkillDir/ProjectSkillDir stored as already-joined relative paths so the
// catalog stays a compact, single-source-of-truth data table.
type toolDef struct {
	name            string
	agentName       string
	skillDir        string
	projectSkillDir string
	configFile      string
	binary          string
	specialDir      string // 非空 = 该工具对应注册表特殊目录（如 "codex-only"）；是 specialDir 映射的唯一来源
}

// join 用 filepath.Join 拼接目录字段，让目录行更易读。
func join(parts ...string) string { return filepath.Join(parts...) }

// catalog is the single source of truth for every supported tool. Adding a
// new tool is a one-line change here; the exported vars (Claude, Codex, …)
// and the allTools slice are derived from it below.
var catalog = []toolDef{
	// ── Original first-class tools ──
	// specialDir 值与 registry 包的特殊目录常量字面一致（"codex-only" 等），
	// 是 cmd/specialFlags 与 tool.specialToolByDir 的唯一派生源。
	{name: "claude", agentName: "claude-code", skillDir: join(".claude", "skills"), projectSkillDir: join(".claude", "skills"), configFile: "CLAUDE.md", binary: "claude", specialDir: "claude-only"},
	{name: "codex", agentName: "codex", skillDir: join(".codex", "skills"), projectSkillDir: join(".agents", "skills"), configFile: "AGENTS.md", binary: "codex", specialDir: "codex-only"},
	{name: "gemini", agentName: "gemini-cli", skillDir: join(".gemini", "skills"), projectSkillDir: join(".agents", "skills"), configFile: "GEMINI.md", binary: "gemini", specialDir: "gemini-only"},
	{name: "opencode", agentName: "opencode", skillDir: join(".config", "opencode", "skills"), projectSkillDir: join(".agents", "skills"), configFile: "OPENCODE.md", binary: "opencode", specialDir: "opencode-only"},
	{name: "hermes", agentName: "hermes-agent", skillDir: join(".hermes", "skills"), projectSkillDir: join(".hermes", "skills"), configFile: "HERMES.md", binary: "hermes", specialDir: "hermes-only"},
	{name: "openclaw", agentName: "openclaw", skillDir: join(".openclaw", "skills"), projectSkillDir: "skills", configFile: "OPENCLAW.md", binary: "openclaw", specialDir: "openclaw-only"},

	// ── Additional agents (from vercel-labs/skills) ──
	{name: "aider-desk", agentName: "aider-desk", skillDir: join(".aider-desk", "skills"), projectSkillDir: join(".aider-desk", "skills"), binary: "aider-desk"},
	{name: "amp", agentName: "amp", skillDir: join(".config", "agents", "skills"), projectSkillDir: join(".agents", "skills"), binary: "amp"},
	{name: "replit", agentName: "replit", skillDir: join(".config", "agents", "skills"), projectSkillDir: join(".agents", "skills")},
	{name: "universal", agentName: "universal", skillDir: join(".config", "agents", "skills"), projectSkillDir: join(".agents", "skills")},
	{name: "antigravity", agentName: "antigravity", skillDir: join(".gemini", "antigravity", "skills"), projectSkillDir: join(".agents", "skills")},
	{name: "antigravity-cli", agentName: "antigravity-cli", skillDir: join(".gemini", "antigravity-cli", "skills"), projectSkillDir: join(".agents", "skills")},
	{name: "astrbot", agentName: "astrbot", skillDir: join(".astrbot", "data", "skills"), projectSkillDir: join("data", "skills")},
	{name: "autohand-code", agentName: "autohand-code", skillDir: join(".autohand", "skills"), projectSkillDir: join(".autohand", "skills")},
	{name: "augment", agentName: "augment", skillDir: join(".augment", "skills"), projectSkillDir: join(".augment", "skills")},
	{name: "bob", agentName: "bob", skillDir: join(".bob", "skills"), projectSkillDir: join(".bob", "skills")},
	{name: "cline", agentName: "cline", skillDir: join(".agents", "skills"), projectSkillDir: join(".agents", "skills")},
	{name: "codearts-agent", agentName: "codearts-agent", skillDir: join(".codeartsdoer", "skills"), projectSkillDir: join(".codeartsdoer", "skills")},
	{name: "codebuddy", agentName: "codebuddy", skillDir: join(".codebuddy", "skills"), projectSkillDir: join(".codebuddy", "skills")},
	{name: "codemaker", agentName: "codemaker", skillDir: join(".codemaker", "skills"), projectSkillDir: join(".codemaker", "skills")},
	{name: "codestudio", agentName: "codestudio", skillDir: join(".codestudio", "skills"), projectSkillDir: join(".codestudio", "skills")},
	{name: "command-code", agentName: "command-code", skillDir: join(".commandcode", "skills"), projectSkillDir: join(".commandcode", "skills")},
	{name: "continue", agentName: "continue", skillDir: join(".continue", "skills"), projectSkillDir: join(".continue", "skills")},
	{name: "cortex", agentName: "cortex", skillDir: join(".snowflake", "cortex", "skills"), projectSkillDir: join(".cortex", "skills")},
	{name: "crush", agentName: "crush", skillDir: join(".config", "crush", "skills"), projectSkillDir: join(".crush", "skills")},
	{name: "cursor", agentName: "cursor", skillDir: join(".cursor", "skills"), projectSkillDir: join(".agents", "skills"), binary: "cursor"},
	{name: "deepagents", agentName: "deepagents", skillDir: join(".deepagents", "agent", "skills"), projectSkillDir: join(".agents", "skills")},
	{name: "devin", agentName: "devin", skillDir: join(".config", "devin", "skills"), projectSkillDir: join(".devin", "skills")},
	{name: "dexto", agentName: "dexto", skillDir: join(".agents", "skills"), projectSkillDir: join(".agents", "skills")},
	{name: "droid", agentName: "droid", skillDir: join(".factory", "skills"), projectSkillDir: join(".factory", "skills")},
	{name: "firebender", agentName: "firebender", skillDir: join(".firebender", "skills"), projectSkillDir: join(".agents", "skills")},
	{name: "forgecode", agentName: "forgecode", skillDir: join(".forge", "skills"), projectSkillDir: join(".forge", "skills")},
	{name: "github-copilot", agentName: "github-copilot", skillDir: join(".copilot", "skills"), projectSkillDir: join(".agents", "skills")},
	{name: "goose", agentName: "goose", skillDir: join(".config", "goose", "skills"), projectSkillDir: join(".goose", "skills"), binary: "goose"},
	{name: "iflow-cli", agentName: "iflow-cli", skillDir: join(".iflow", "skills"), projectSkillDir: join(".iflow", "skills")},
	{name: "inference-sh", agentName: "inference-sh", skillDir: join(".inferencesh", "skills"), projectSkillDir: join(".inferencesh", "skills")},
	{name: "jazz", agentName: "jazz", skillDir: join(".jazz", "skills"), projectSkillDir: join(".jazz", "skills")},
	{name: "junie", agentName: "junie", skillDir: join(".junie", "skills"), projectSkillDir: join(".junie", "skills")},
	{name: "kilo", agentName: "kilo", skillDir: join(".kilocode", "skills"), projectSkillDir: join(".kilocode", "skills")},
	{name: "kimi-code-cli", agentName: "kimi-code-cli", skillDir: join(".agents", "skills"), projectSkillDir: join(".agents", "skills")},
	{name: "kiro-cli", agentName: "kiro-cli", skillDir: join(".kiro", "skills"), projectSkillDir: join(".kiro", "skills")},
	{name: "kode", agentName: "kode", skillDir: join(".kode", "skills"), projectSkillDir: join(".kode", "skills")},
	{name: "lingma", agentName: "lingma", skillDir: join(".lingma", "skills"), projectSkillDir: join(".lingma", "skills")},
	{name: "loaf", agentName: "loaf", skillDir: join(".agents", "skills"), projectSkillDir: join(".agents", "skills")},
	{name: "mcpjam", agentName: "mcpjam", skillDir: join(".mcpjam", "skills"), projectSkillDir: join(".mcpjam", "skills")},
	{name: "mistral-vibe", agentName: "mistral-vibe", skillDir: join(".vibe", "skills"), projectSkillDir: join(".vibe", "skills")},
	{name: "moxby", agentName: "moxby", skillDir: join(".moxby", "skills"), projectSkillDir: join(".moxby", "skills")},
	{name: "mux", agentName: "mux", skillDir: join(".mux", "skills"), projectSkillDir: join(".mux", "skills")},
	{name: "neovate", agentName: "neovate", skillDir: join(".neovate", "skills"), projectSkillDir: join(".neovate", "skills")},
	{name: "ona", agentName: "ona", skillDir: join(".ona", "skills"), projectSkillDir: join(".ona", "skills")},
	{name: "openhands", agentName: "openhands", skillDir: join(".openhands", "skills"), projectSkillDir: join(".openhands", "skills")},
	{name: "pi", agentName: "pi", skillDir: join(".pi", "agent", "skills"), projectSkillDir: join(".pi", "skills")},
	{name: "pochi", agentName: "pochi", skillDir: join(".pochi", "skills"), projectSkillDir: join(".pochi", "skills")},
	{name: "promptscript", agentName: "promptscript", projectSkillDir: join(".agents", "skills")},
	{name: "qoder", agentName: "qoder", skillDir: join(".qoder", "skills"), projectSkillDir: join(".qoder", "skills")},
	{name: "qoder-cn", agentName: "qoder-cn", skillDir: join(".qoder-cn", "skills"), projectSkillDir: join(".qoder", "skills")},
	{name: "qwen-code", agentName: "qwen-code", skillDir: join(".qwen", "skills"), projectSkillDir: join(".qwen", "skills")},
	{name: "reasonix", agentName: "reasonix", skillDir: join(".reasonix", "skills"), projectSkillDir: join(".reasonix", "skills")},
	{name: "rovodev", agentName: "rovodev", skillDir: join(".rovodev", "skills"), projectSkillDir: join(".rovodev", "skills")},
	{name: "roo", agentName: "roo", skillDir: join(".roo", "skills"), projectSkillDir: join(".roo", "skills")},
	{name: "tabnine-cli", agentName: "tabnine-cli", skillDir: join(".tabnine", "agent", "skills"), projectSkillDir: join(".tabnine", "agent", "skills")},
	{name: "terramind", agentName: "terramind", skillDir: join(".terramind", "skills"), projectSkillDir: join(".terramind", "skills")},
	{name: "tinycloud", agentName: "tinycloud", skillDir: join(".tinycloud", "skills"), projectSkillDir: join(".tinycloud", "skills")},
	{name: "trae", agentName: "trae", skillDir: join(".trae", "skills"), projectSkillDir: join(".trae", "skills")},
	{name: "trae-cn", agentName: "trae-cn", skillDir: join(".trae-cn", "skills"), projectSkillDir: join(".trae", "skills")},
	{name: "warp", agentName: "warp", skillDir: join(".agents", "skills"), projectSkillDir: join(".agents", "skills")},
	{name: "windsurf", agentName: "windsurf", skillDir: join(".codeium", "windsurf", "skills"), projectSkillDir: join(".windsurf", "skills")},
	{name: "zed", agentName: "zed", skillDir: join(".agents", "skills"), projectSkillDir: join(".agents", "skills"), binary: "zed"},
	{name: "zencoder", agentName: "zencoder", skillDir: join(".zencoder", "skills"), projectSkillDir: join(".zencoder", "skills")},
	{name: "zenflow", agentName: "zenflow", skillDir: join(".zencoder", "skills"), projectSkillDir: join(".zencoder", "skills")},
	{name: "adal", agentName: "adal", skillDir: join(".adal", "skills"), projectSkillDir: join(".adal", "skills")},
	{name: "eve", agentName: "eve", skillDir: join("agent", "skills"), projectSkillDir: join("agent", "skills")}, // project-only (no global dir)
	{name: "grok", agentName: "grok", skillDir: join(".grok", "skills"), projectSkillDir: join(".grok", "skills")},
	{name: "kimchi", agentName: "kimchi", skillDir: join(".kimchi", "skills"), projectSkillDir: join(".kimchi", "skills")},
	{name: "zcode", agentName: "zcode", skillDir: join(".zcode", "skills"), projectSkillDir: join(".zcode", "skills")},
}

// makeTools 把 catalog 展开为具体的 Tool 值，保持顺序。
func makeTools() []Tool {
	tools := make([]Tool, len(catalog))
	for i, d := range catalog {
		tools[i] = Tool{
			Name:            d.name,
			AgentName:       d.agentName,
			SkillDir:        d.skillDir,
			ProjectSkillDir: d.projectSkillDir,
			ConfigFile:      d.configFile,
			Binary:          d.binary,
			SpecialDir:      d.specialDir,
		}
	}
	return tools
}

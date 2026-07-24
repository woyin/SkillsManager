// Package registry 管理 sm 的磁盘技能与 MCP 注册表。
//
// 注册表根目录结构：
//
//	<registry>/
//	  skills/
//	    global/         特殊目录：安装到全部工具
//	    codex-only/     特殊目录：仅安装到 Codex
//	    claude-only/    特殊目录：仅安装到 Claude
//	    ...             用户自定义分类目录
//	  mcp/               MCP 服务定义（.json 或 git 仓库）
//
// 特殊目录（specialDirs）拥有固定的安装目标，其它目录默认安装到全部工具。
//
// 分层约束：本包是底层，不得导入 internal/tool。特殊目录字符串字面量
// 与 tool catalog 的 specialDir 字段保持一致；运行时“工具→特殊目录”
// 映射以 tool catalog 为单一来源，此处只保留目录名常量与集合查询。
//
// Input: path/filepath
// Output: type Registry, type ItemDetail, type DiscoveredSkill, type SkillMatch, func New, func IsSpecialDir
// Pos: 数据层-注册表类型
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package registry

import "path/filepath"

// 特殊目录常量：具有固定安装目标的目录名。
// 这些常量同时是 specialDirs 集合和 tool catalog specialDir 字段的键，
// 修改时需同步 tool/data.go。
const (
	Global       = "global"        // 安装到全部工具
	CodexOnly    = "codex-only"    // 仅 Codex
	ClaudeOnly   = "claude-only"   // 仅 Claude
	GeminiOnly   = "gemini-only"   // 仅 Gemini
	OpenCodeOnly = "opencode-only" // 仅 OpenCode
	HermesOnly   = "hermes-only"   // 仅 Hermes
	OpenClawOnly = "openclaw-only" // 仅 OpenClaw
)

// specialDirs 记录哪些目录名属于"特殊目录"。
// 集合形式便于 IsSpecialDir 做 O(1) 查询。
var specialDirs = map[string]bool{
	Global:       true,
	CodexOnly:    true,
	ClaudeOnly:   true,
	GeminiOnly:   true,
	OpenCodeOnly: true,
	HermesOnly:   true,
	OpenClawOnly: true,
}

// Registry 是注册表的入口：封装根目录，提供增删查改方法。
// 该结构体无状态，可被多个 goroutine 共享。
type Registry struct {
	dir string
}

// ItemDetail 描述一条注册表条目，供 Web API 与 list 命令使用。
type ItemDetail struct {
	Name        string `json:"name"`
	Category    string `json:"category,omitempty"`   // 技能所属分类（MCP 为空）
	Path        string `json:"path"`                 // 磁盘绝对路径
	SourceURL   string `json:"source_url,omitempty"` // git 远端 URL（若为 git 管理）
	LastUpdated string `json:"last_updated"`         // ISO8601 修改时间
}

// DiscoveredSkill 表示在某个仓库/目录中发现的一个技能。
type DiscoveredSkill struct {
	Name        string
	Description string
	Path        string // 技能目录路径
	SkillMDPath string // SKILL.md 文件路径
	// Internal 为 true 时，该技能被标记为内部技能。除非环境变量
	// INSTALL_INTERNAL_SKILLS 为真值，内部技能在发现时被过滤。
	Internal bool
}

// SkillMatch 是按名检索 registry 时的一条命中：技能路径及其所在分类目录。
// 供 Registry Install 判断单匹配/多匹配歧义用（见 FindSkillCategories）。
type SkillMatch struct {
	Path     string
	Category string
}

// ── 插件清单相关类型 ──

// pluginMarketplace 对应 .{xxx}-plugin/marketplace.json，可声明多个插件。
type pluginMarketplace struct {
	Metadata pluginMetadata   `json:"metadata"`
	Plugins  []pluginManifest `json:"plugins"`
}

// pluginMetadata 是 marketplace.json 的顶层 metadata 对象。
type pluginMetadata struct {
	PluginRoot string `json:"pluginRoot"` // 技能相对根（默认为仓库根）
}

// pluginManifest 描述单个插件：名称、来源、所含技能相对路径列表。
type pluginManifest struct {
	Name       string   `json:"name"`
	Source     string   `json:"source"`
	Skills     []string `json:"skills"`
	PluginRoot string   `json:"pluginRoot,omitempty"`
}

// New 返回以 dir 为根的 Registry。
func New(dir string) *Registry {
	return &Registry{dir: dir}
}

// skillsDir 返回注册表中 skills 子目录的绝对路径。
func (r *Registry) skillsDir() string {
	return filepath.Join(r.dir, "skills")
}

// mcpDir 返回注册表中 mcp 子目录的绝对路径。
func (r *Registry) mcpDir() string {
	return filepath.Join(r.dir, "mcp")
}

// IsSpecialDir 判断给定分类目录是否为特殊目录。
func IsSpecialDir(category string) bool {
	return specialDirs[category]
}

// Dir 返回注册表根目录。
func (r *Registry) Dir() string {
	return r.dir
}

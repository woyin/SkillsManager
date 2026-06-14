// Package registry manages the on-disk skill and MCP registry.
package registry

import "path/filepath"

// Special directories that have fixed install targets.
const (
	Global       = "global"
	CodexOnly    = "codex-only"
	ClaudeOnly   = "claude-only"
	GeminiOnly   = "gemini-only"
	OpenCodeOnly = "opencode-only"
	HermesOnly   = "hermes-only"
	OpenClawOnly = "openclaw-only"
)

var specialDirs = map[string]bool{
	Global:       true,
	CodexOnly:    true,
	ClaudeOnly:   true,
	GeminiOnly:   true,
	OpenCodeOnly: true,
	HermesOnly:   true,
	OpenClawOnly: true,
}

// Registry manages the on-disk skill and MCP registry: adding, removing, and
// listing entries, and resolving them to filesystem paths.
type Registry struct {
	dir string
}

// ItemDetail describes one registry entry for the web API and list output.
type ItemDetail struct {
	Name        string `json:"name"`
	Category    string `json:"category,omitempty"`
	Path        string `json:"path"`
	SourceURL   string `json:"source_url,omitempty"`
	LastUpdated string `json:"last_updated"`
}

// DiscoveredSkill represents a skill found in a repository.
type DiscoveredSkill struct {
	Name        string
	Description string
	Path        string // Path to the skill directory
	SkillMDPath string // Path to the SKILL.md file
	// Internal is true when the SKILL.md frontmatter declares
	// `metadata.internal: true`. Internal skills are hidden from discovery
	// unless the INSTALL_INTERNAL_SKILLS environment variable is set to a
	// truthy value (mirrors npx skills behavior).
	Internal bool
}

// ── Plugin manifest types ──

// pluginMarketplace represents a .claude-plugin/marketplace.json file.
type pluginMarketplace struct {
	Metadata pluginMetadata   `json:"metadata"`
	Plugins  []pluginManifest `json:"plugins"`
}

// pluginMetadata holds marketplace-level metadata.
type pluginMetadata struct {
	PluginRoot string `json:"pluginRoot"`
}

// pluginManifest represents a single plugin definition.
type pluginManifest struct {
	Name       string   `json:"name"`
	Source     string   `json:"source"`
	Skills     []string `json:"skills"`
	PluginRoot string   `json:"pluginRoot,omitempty"`
}

// New returns a Registry rooted at dir.
func New(dir string) *Registry {
	return &Registry{dir: dir}
}

func (r *Registry) skillsDir() string {
	return filepath.Join(r.dir, "skills")
}

func (r *Registry) mcpDir() string {
	return filepath.Join(r.dir, "mcp")
}

// IsSpecialDir returns true if the category is a special directory.
func IsSpecialDir(category string) bool {
	return specialDirs[category]
}

// Dir returns the registry root directory.
func (r *Registry) Dir() string {
	return r.dir
}

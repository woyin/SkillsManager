// cmd/resolve.go 定义 specialFlags：把 --global/--codex/--claude 等
// 布尔标志解析为注册表特殊目录名。单一表格驱动绑定与解析。
// cmd/resolve.go
package cmd

import (
	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/registry"
)

// resolveSpecial returns the special directory name based on the boolean flags.
// Used by add and rm commands.
type specialFlags struct {
	Global   bool
	Codex    bool
	Claude   bool
	Gemini   bool
	OpenCode bool
	Hermes   bool
	OpenClaw bool
}

// specialFlagSpec describes one `--<agent>` flag: the long flag name, a pointer
// into specialFlags, and the registry special-directory it resolves to. This
// single table drives both flag registration (Bind) and resolution (Resolve),
// so add/rm never duplicate the list and the set of flags cannot drift from
// the registry's special directories.
type specialFlagSpec struct {
	flag    string
	field   *bool
	special string
}

// specs returns the flag specs bound to f. Built fresh on each call so it
// always reflects the current flag values.
func (f *specialFlags) specs() []specialFlagSpec {
	return []specialFlagSpec{
		{flag: "global", field: &f.Global, special: registry.Global},
		{flag: "codex", field: &f.Codex, special: registry.CodexOnly},
		{flag: "claude", field: &f.Claude, special: registry.ClaudeOnly},
		{flag: "gemini", field: &f.Gemini, special: registry.GeminiOnly},
		{flag: "opencode", field: &f.OpenCode, special: registry.OpenCodeOnly},
		{flag: "hermes", field: &f.Hermes, special: registry.HermesOnly},
		{flag: "openclaw", field: &f.OpenClaw, special: registry.OpenClawOnly},
	}
}

// Bind registers the seven `--<agent>` flags on c, using verb as the prefix
// (e.g. "Add to" → "Add to codex-only directory"). global is described
// separately since it targets all tools rather than a single agent.
func (f *specialFlags) Bind(c *cobra.Command, verb string) {
	for _, s := range f.specs() {
		desc := verb + " " + s.special + " directory"
		if s.flag == "global" {
			desc = verb + " global directory (all tools)"
		}
		c.Flags().BoolVar(s.field, s.flag, false, desc)
	}
}

// Resolve returns the special directory name for the first flag the user set,
// or "" if none. Mirrors the original first-match behavior of add/rm.
func (f *specialFlags) Resolve() string {
	for _, s := range f.specs() {
		if *s.field {
			return s.special
		}
	}
	return ""
}

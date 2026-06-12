// cmd/rm.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
)

var (
	rmFlags specialFlags
	rmIsMCP bool
)

var rmCmd = &cobra.Command{
	Use:   "rm <name> [category]",
	Short: "Remove a skill or MCP from the registry",
	Long: `Remove a skill or MCP server definition from the registry.
Also cleans up symlinks in installed locations.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		reg := registry.New(RegistryDir)

		if rmIsMCP {
			if err := reg.RemoveMCP(name); err != nil {
				return fmt.Errorf("removing MCP: %w", err)
			}
			fmt.Printf("✓ Removed MCP %q\n", name)
			return nil
		}

		special := rmFlags.Resolve()

		category := ""
		if len(args) > 1 {
			category = args[1]
		}

		// Find the skill path before removal (for symlink cleanup)
		skillPath, _ := reg.GetSkillPath(name, category, special)

		if err := reg.RemoveSkill(name, category, special); err != nil {
			return fmt.Errorf("removing skill: %w", err)
		}

		// Clean up symlinks in installed locations
		if skillPath != "" {
			home, _ := os.UserHomeDir()
			for _, t := range tool.AllTools() {
				dir := filepath.Join(home, t.SkillDir)
				links, _ := symlink.FindPointingTo(dir, skillPath)
				for _, link := range links {
					os.Remove(link)
					fmt.Printf("  Removed symlink: %s\n", link)
				}
			}
		}

		fmt.Printf("✓ Removed skill %q\n", name)
		return nil
	},
}

func init() {
	rmCmd.Flags().BoolVar(&rmFlags.Global, "global", false, "Remove from global directory")
	rmCmd.Flags().BoolVar(&rmFlags.Codex, "codex", false, "Remove from codex-only directory")
	rmCmd.Flags().BoolVar(&rmFlags.Claude, "claude", false, "Remove from claude-only directory")
	rmCmd.Flags().BoolVar(&rmFlags.Gemini, "gemini", false, "Remove from gemini-only directory")
	rmCmd.Flags().BoolVar(&rmFlags.OpenCode, "opencode", false, "Remove from opencode-only directory")
	rmCmd.Flags().BoolVar(&rmFlags.Hermes, "hermes", false, "Remove from hermes-only directory")
	rmCmd.Flags().BoolVar(&rmFlags.OpenClaw, "openclaw", false, "Remove from openclaw-only directory")
	rmCmd.Flags().BoolVar(&rmIsMCP, "mcp", false, "Remove MCP server definition")

	rootCmd.AddCommand(rmCmd)
}

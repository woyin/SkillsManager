// cmd/rm.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/symlink"
)

var (
	rmGlobal bool
	rmCodex  bool
	rmClaude bool
	rmIsMCP  bool
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

		var special string
		if rmGlobal {
			special = registry.Global
		} else if rmCodex {
			special = registry.CodexOnly
		} else if rmClaude {
			special = registry.ClaudeOnly
		}

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
			for _, dir := range []string{
				filepath.Join(home, ".codex", "skills"),
				filepath.Join(home, ".claude", "skills"),
			} {
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
	rmCmd.Flags().BoolVar(&rmGlobal, "global", false, "Remove from global directory")
	rmCmd.Flags().BoolVar(&rmCodex, "codex", false, "Remove from codex-only directory")
	rmCmd.Flags().BoolVar(&rmClaude, "claude", false, "Remove from claude-only directory")
	rmCmd.Flags().BoolVar(&rmIsMCP, "mcp", false, "Remove MCP server definition")

	rootCmd.AddCommand(rmCmd)
}

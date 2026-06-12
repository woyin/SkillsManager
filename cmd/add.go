// cmd/add.go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/registry"
)

var (
	addFlags specialFlags
	addIsMCP bool
)

var addCmd = &cobra.Command{
	Use:   "add <source> [category]",
	Short: "Add a skill or MCP to the registry",
	Long: `Add a skill or MCP server definition to the registry.
Source can be a GitHub URL or local path.
Category is the directory name under registry/skills/ or registry/mcp/.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		source := args[0]
		reg := registry.New(RegistryDir)

		if addIsMCP {
			if err := reg.AddMCP(source); err != nil {
				return fmt.Errorf("adding MCP: %w", err)
			}
			fmt.Printf("✓ Added MCP from %s\n", source)
			return nil
		}

		special := addFlags.Resolve()

		category := ""
		if len(args) > 1 {
			category = args[1]
		}

		if err := reg.AddSkill(source, category, special); err != nil {
			return fmt.Errorf("adding skill: %w", err)
		}

		dest := special
		if dest == "" {
			dest = category
		}
		name := registry.SkillNameFromPath(source)
		fmt.Printf("✓ Added skill %q to %s\n", name, dest)
		return nil
	},
}

func init() {
	addCmd.Flags().BoolVar(&addFlags.Global, "global", false, "Add to global directory (all tools)")
	addCmd.Flags().BoolVar(&addFlags.Codex, "codex", false, "Add to codex-only directory")
	addCmd.Flags().BoolVar(&addFlags.Claude, "claude", false, "Add to claude-only directory")
	addCmd.Flags().BoolVar(&addFlags.Gemini, "gemini", false, "Add to gemini-only directory")
	addCmd.Flags().BoolVar(&addFlags.OpenCode, "opencode", false, "Add to opencode-only directory")
	addCmd.Flags().BoolVar(&addFlags.Hermes, "hermes", false, "Add to hermes-only directory")
	addCmd.Flags().BoolVar(&addFlags.OpenClaw, "openclaw", false, "Add to openclaw-only directory")
	addCmd.Flags().BoolVar(&addIsMCP, "mcp", false, "Add as MCP server definition")

	rootCmd.AddCommand(addCmd)
}

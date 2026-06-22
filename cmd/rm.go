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
	rmFlags  specialFlags
	rmIsMCP  bool
	rmAll    bool
	rmAgents []string
	rmSkills []string
	rmYes    bool
)

var rmCmd = &cobra.Command{
	Use:     "rm <name> [category]",
	Aliases: []string{"remove"},
	Short:   "Remove a skill or MCP from the registry",
	Long: `Remove a skill or MCP server definition from the registry.
Also cleans up symlinks in installed locations.

Examples:
  # Remove a skill by name
  sm rm my-skill

  # Remove from specific category
  sm rm my-skill cloudflare

  # Remove from global scope
  sm rm --global my-skill

  # Remove from specific agents only
  sm rm --agent claude-code cursor my-skill

  # Remove a specific skill from all agents
  sm rm my-skill --agent '*'

  # Remove all installed skills without confirmation
  sm rm --all

  # Remove all skills from a specific agent
  sm rm --skill '*' -a cursor

  # Remove MCP
  sm rm cloudflare --mcp
`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if rmIsMCP {
			if len(args) == 0 {
				return fmt.Errorf("MCP name required")
			}
			return removeMCP(args[0])
		}

		// --all mode
		if rmAll {
			return removeAll()
		}

		// --agent mode with --skill
		if len(rmAgents) > 0 {
			return removeFromAgents(args)
		}

		// Standard remove
		if len(args) == 0 {
			return fmt.Errorf("skill name required")
		}

		name := args[0]
		return removeSkill(name, args)
	},
}

func removeMCP(name string) error {
	reg := registry.New(RegistryDir)
	if err := reg.RemoveMCP(name); err != nil {
		return fmt.Errorf("removing MCP: %w", err)
	}
	fmt.Printf("✓ Removed MCP %q\n", name)
	return nil
}

func removeAll() error {
	reg := registry.New(RegistryDir)
	skills, err := reg.ListSkills()
	if err != nil {
		return fmt.Errorf("listing skills: %w", err)
	}

	if !rmYes {
		count := 0
		for _, names := range skills {
			count += len(names)
		}
		fmt.Printf("This will remove %d skill(s) and all their symlinks. Continue? [y/N]: ", count)
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	home, _ := os.UserHomeDir()
	removed := 0
	for category, names := range skills {
		for _, name := range names {
			skillPath, _ := reg.GetSkillPath(name, category, "")
			// Clean up symlinks
			if skillPath != "" {
				for _, t := range tool.AllTools() {
					dir := filepath.Join(home, t.SkillDir)
					links, _ := symlink.FindPointingTo(dir, skillPath)
					for _, link := range links {
						os.Remove(link)
					}
				}
			}
			// Remove from registry
			reg.RemoveSkill(name, category, "")
			removed++
		}
	}

	fmt.Printf("✓ Removed %d skill(s)\n", removed)
	return nil
}

func removeFromAgents(args []string) error {
	targetTools := tool.ToolsByNames(rmAgents)
	if len(targetTools) == 0 {
		return fmt.Errorf("no matching agents found for: %v", rmAgents)
	}

	skillsToRemove := rmSkills
	if len(args) > 0 && len(skillsToRemove) == 0 {
		skillsToRemove = args
	}

	home, _ := os.UserHomeDir()
	removed := 0

	for _, t := range targetTools {
		agentDir := filepath.Join(home, t.SkillDir)
		entries, err := os.ReadDir(agentDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			name := entry.Name()

			// Filter by skill names
			if len(skillsToRemove) > 0 {
				match := false
				for _, s := range skillsToRemove {
					if s == "*" || s == name {
						match = true
						break
					}
				}
				if !match {
					continue
				}
			}

			linkPath := filepath.Join(agentDir, name)
			if err := os.Remove(linkPath); err == nil {
				fmt.Printf("  ✓ Removed %s from %s\n", name, t.Name)
				removed++
			}
		}
	}

	fmt.Printf("\n✓ Removed %d skill(s) from %d agent(s)\n", removed, len(targetTools))
	return nil
}

func removeSkill(name string, args []string) error {
	reg := registry.New(RegistryDir)
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
}

func init() {
	rmFlags.Bind(rmCmd, "Remove from")
	rmCmd.Flags().BoolVar(&rmIsMCP, "mcp", false, "Remove MCP server definition")

	// New flags from vercel-labs/skills
	rmCmd.Flags().BoolVar(&rmAll, "all", false, "Shorthand for --skill '*' --agent '*' -y")
	rmCmd.Flags().StringArrayVarP(&rmAgents, "agent", "a", nil, "Remove from specific agents (use '*' for all)")
	rmCmd.Flags().StringArrayVarP(&rmSkills, "skill", "s", nil, "Specify skills to remove (use '*' for all)")
	rmCmd.Flags().BoolVarP(&rmYes, "yes", "y", false, "Skip confirmation prompts")

	rootCmd.AddCommand(rmCmd)
}

// cmd/rm.go 实现 `sm rm`：从注册表移除技能/MCP，
// 并清理各代理目录中指向它的符号链接。
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
	rmFlags   = newSpecialFlags()
	rmIsMCP   bool
	rmAll     bool
	rmAgents  []string
	rmSkills  []string
	rmYes     bool
	rmProject bool   // --project: 只清理项目级目录 ./<agent>/skills 的符号链接
	rmDir     string // --dir: 项目根（配合 --project，默认当前目录）
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

		// --all 模式
		if rmAll {
			return removeAll()
		}

		// --agent + --skill 模式
		if len(rmAgents) > 0 {
			return removeFromAgents(args)
		}

		// 标准移除流程
		if len(args) == 0 {
			return fmt.Errorf("skill name required")
		}

		name := args[0]
		return removeSkill(name, args)
	},
}

// 从注册表移除一个 MCP。

func removeMCP(name string) error {
	reg := registry.New(RegistryDir)
	if err := reg.RemoveMCP(name); err != nil {
		return fmt.Errorf("removing MCP: %w", err)
	}
	fmt.Printf("✓ Removed MCP %q\n", name)
	return nil
}

// 移除注册表中的全部技能及其在各代理目录中的符号链接（需确认）。

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

	removed := 0
	for category, names := range skills {
		for _, name := range names {
			skillPath, _ := reg.GetSkillPath(name, category, "")
			// Clean up symlinks (global + project if --project)
			if skillPath != "" {
				for _, t := range tool.AllTools() {
					for _, dir := range rmScanDirs(t) {
						links, _ := symlink.FindPointingTo(dir, skillPath)
						for _, link := range links {
							os.Remove(link)
						}
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

// 从指定代理目录移除匹配的技能符号链接。

func removeFromAgents(args []string) error {
	targetTools := tool.ToolsByNames(rmAgents)
	if len(targetTools) == 0 {
		return fmt.Errorf("no matching agents found for: %v", rmAgents)
	}

	skillsToRemove := rmSkills
	if len(args) > 0 && len(skillsToRemove) == 0 {
		skillsToRemove = args
	}

	removed := 0

	for _, t := range targetTools {
		for _, agentDir := range rmScanDirs(t) {
			entries, err := os.ReadDir(agentDir)
			if err != nil {
				continue
			}

			for _, entry := range entries {
				name := entry.Name()

				// 按技能名过滤
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
	}

	fmt.Printf("\n✓ Removed %d skill(s) from %d agent(s)\n", removed, len(targetTools))
	return nil
}

// 从注册表移除单个技能，并清理指向它的符号链接。

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

	// Clean up symlinks in installed locations (global + project if --project)
	if skillPath != "" {
		for _, t := range tool.AllTools() {
			for _, dir := range rmScanDirs(t) {
				links, _ := symlink.FindPointingTo(dir, skillPath)
				for _, link := range links {
					os.Remove(link)
					fmt.Printf("  Removed symlink: %s\n", link)
				}
			}
		}
	}

	fmt.Printf("✓ Removed skill %q\n", name)
	return nil
}

// rmScanDirs 返回工具 t 下应扫描清理的技能目录列表：始终含全局目录，
// 当 --project 设置时追加项目级目录（跳过无 ProjectSkillDir 的工具）。
func rmScanDirs(t tool.Tool) []string {
	home, _ := os.UserHomeDir()
	dirs := []string{filepath.Join(home, t.SkillDir)}
	if rmProject {
		if pd := tool.GetProjectSkillDir(t, rmProjectDir()); pd != "" {
			dirs = append(dirs, pd)
		}
	}
	return dirs
}

// rmProjectDir 解析 --dir 指定的项目根，未指定则用当前目录。
func rmProjectDir() string {
	if rmDir != "" {
		return rmDir
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

func init() {
	rmFlags.Bind(rmCmd, "Remove from")
	rmCmd.Flags().BoolVar(&rmIsMCP, "mcp", false, "Remove MCP server definition")

	// New flags from vercel-labs/skills
	rmCmd.Flags().BoolVar(&rmAll, "all", false, "Shorthand for --skill '*' --agent '*' -y")
	rmCmd.Flags().StringArrayVarP(&rmAgents, "agent", "a", nil, "Remove from specific agents (use '*' for all)")
	rmCmd.Flags().StringArrayVarP(&rmSkills, "skill", "s", nil, "Specify skills to remove (use '*' for all)")
	rmCmd.Flags().BoolVarP(&rmYes, "yes", "y", false, "Skip confirmation prompts")
	rmCmd.Flags().BoolVar(&rmProject, "project", false,
		"Also clean project-level symlinks (./<agent>/skills) in addition to global")
	rmCmd.Flags().StringVar(&rmDir, "dir", "", "Project root for --project (default: current dir)")

	rootCmd.AddCommand(rmCmd)
}

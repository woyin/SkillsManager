// cmd/rm.go 实现 `sm rm`：默认卸装（清 agent 目录）并在无其它引用时删除 registry 原件。
//
// Input: fmt, os, path/filepath, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/home, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/internal/symlink, github.com/woyin/skills-manager/internal/tool
// Output: var rmCmd, func removeMCP, func removeAll, func removeFromAgents, func removeSkill, func countReferencesTo, func rmScanDirs
// Pos: 控制层-rm命令实现
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/home"
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
	rmProject bool   // --project: 仅项目级（默认项目+全局）
	rmDir     string // --dir: 项目根
)

var rmCmd = &cobra.Command{
	Use:     "rm <name> [category]",
	Aliases: []string{"remove"},
	Short:   "Uninstall a skill and remove registry original if unused",
	Long: `Uninstall a skill from agent skill dirs and remove the registry original
when nothing else references it.

Default scope: project (./<agent>/skills) plus global (~/<agent>/skills).
Use --project or --global to limit. Use --agent to limit agents.

Examples:
  sm rm my-skill
  sm rm my-skill --project
  sm rm --agent claude my-skill
  sm rm --all
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

		if rmAll {
			return removeAll()
		}

		// --agent 模式：只清 agent 目录链接（兼容旧行为）
		if len(rmAgents) > 0 {
			return removeFromAgents(args)
		}

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

	removed := 0
	for category, names := range skills {
		for _, name := range names {
			skillPath, _ := reg.GetSkillPath(name, category, "")
			if skillPath != "" {
				for _, t := range tool.AllTools() {
					for _, dir := range rmScanDirs(t) {
						links, _ := symlink.FindPointingTo(dir, skillPath)
						for _, link := range links {
							os.Remove(link)
						}
						// also remove same-name entries (copy installs)
						os.RemoveAll(filepath.Join(dir, name))
					}
				}
			}
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

	removed := 0

	for _, t := range targetTools {
		for _, agentDir := range rmScanDirs(t) {
			entries, err := os.ReadDir(agentDir)
			if err != nil {
				continue
			}

			for _, entry := range entries {
				name := entry.Name()

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
				if err := os.RemoveAll(linkPath); err == nil {
					fmt.Printf("  ✓ Removed %s from %s\n", name, t.Name)
					removed++
				}
			}
		}
	}

	fmt.Printf("\n✓ Removed %d skill(s) from %d agent(s)\n", removed, len(targetTools))
	return nil
}

// removeSkill 卸装并在无引用时删 registry 原件。
func removeSkill(name string, args []string) error {
	reg := registry.New(RegistryDir)
	special := rmFlags.Resolve()

	category := ""
	if len(args) > 1 {
		category = args[1]
	}

	skillPath, _ := reg.GetSkillPath(name, category, special)

	// 1) 卸装：清默认范围内 agent 目录中的同名条目
	uninstalled := 0
	for _, t := range tool.AllTools() {
		for _, dir := range rmScanDirs(t) {
			linkPath := filepath.Join(dir, name)
			if _, err := os.Lstat(linkPath); err != nil {
				continue
			}
			if err := os.RemoveAll(linkPath); err == nil {
				fmt.Printf("  Uninstalled: %s\n", linkPath)
				uninstalled++
			}
		}
		// 也清指向 registry 原件的其它名字链接（极少见）
		if skillPath != "" {
			for _, dir := range rmScanDirs(t) {
				links, _ := symlink.FindPointingTo(dir, skillPath)
				for _, link := range links {
					os.Remove(link)
					fmt.Printf("  Removed symlink: %s\n", link)
					uninstalled++
				}
			}
		}
	}

	// 2) 若 registry 中还有其它 agent 引用该原件，则保留 registry
	if skillPath != "" {
		if remaining := countReferencesTo(skillPath, name); remaining > 0 {
			fmt.Printf("✓ Uninstalled skill %q (%d agent link(s) remain elsewhere; registry kept)\n", name, remaining)
			return nil
		}
		if err := reg.RemoveSkill(name, category, special); err != nil {
			// 已卸装但 registry 删除失败：报告但不回滚卸装
			return fmt.Errorf("uninstalled from agents, but removing registry original failed: %w", err)
		}
		fmt.Printf("✓ Removed skill %q (uninstalled %d, registry original deleted)\n", name, uninstalled)
		return nil
	}

	// 无 registry 原件：仅卸装
	if uninstalled == 0 {
		return fmt.Errorf("skill %q not found in agent dirs or registry", name)
	}
	fmt.Printf("✓ Uninstalled skill %q (%d location(s); no registry original)\n", name, uninstalled)
	return nil
}

// countReferencesTo 统计仍指向 skillPath 或同名的 agent 安装条目数（全工具、全局+项目）。
func countReferencesTo(skillPath, name string) int {
	count := 0
	for _, t := range tool.AllTools() {
		dirs := []string{filepath.Join(home.Dir(), t.SkillDir)}
		if pd := tool.GetProjectSkillDir(t, rmProjectDir()); pd != "" {
			dirs = append(dirs, pd)
		}
		for _, dir := range dirs {
			linkPath := filepath.Join(dir, name)
			if _, err := os.Lstat(linkPath); err == nil {
				count++
				continue
			}
			if skillPath != "" {
				links, _ := symlink.FindPointingTo(dir, skillPath)
				count += len(links)
			}
		}
	}
	return count
}

// rmScanDirs 返回应清理的技能目录：默认项目+全局；--project 仅项目。
// specialFlags 的 --global 表示 registry 分类，不收窄扫目录。
func rmScanDirs(t tool.Tool) []string {
	var dirs []string
	if !rmProject {
		dirs = append(dirs, filepath.Join(home.Dir(), t.SkillDir))
	}
	if pd := tool.GetProjectSkillDir(t, rmProjectDir()); pd != "" {
		dirs = append(dirs, pd)
	}
	return dirs
}

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
	rmCmd.Flags().BoolVar(&rmAll, "all", false, "Shorthand for --skill '*' --agent '*' -y")
	rmCmd.Flags().StringArrayVarP(&rmAgents, "agent", "a", nil, "Remove from specific agents (use '*' for all)")
	rmCmd.Flags().StringArrayVarP(&rmSkills, "skill", "s", nil, "Specify skills to remove (use '*' for all)")
	rmCmd.Flags().BoolVarP(&rmYes, "yes", "y", false, "Skip confirmation prompts")
	rmCmd.Flags().BoolVar(&rmProject, "project", false,
		"Only clean project-level installs (./<agent>/skills)")
	rmCmd.Flags().StringVar(&rmDir, "dir", "", "Project root (default: current dir)")

	rootCmd.AddCommand(rmCmd)
}

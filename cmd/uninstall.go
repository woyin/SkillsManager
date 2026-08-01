// cmd/uninstall.go 实现 `sm uninstall`：按 scope/agent/skill 从代理目录移除
// 由 sm 安装的符号链接（不影响注册表与 profiles）。
//
// Input: fmt, os, path/filepath, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/home, github.com/woyin/skills-manager/internal/symlink, github.com/woyin/skills-manager/internal/tool
// Output: var uninstallCmd, type uninstallOptions, func removeInstalledSymlinks, func matchesAny
// Pos: 控制层-uninstall命令实现
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/lockfile"
	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
)

var (
	uninstallAgents  []string
	uninstallSkills  []string
	uninstallProject bool // 仅项目
	uninstallGlobal  bool // 仅全局
	uninstallDir     string
	uninstallAll     bool
	uninstallYes     bool
)

type uninstallOptions struct {
	homeDir     string
	agents      []string
	skills      []string
	projectOnly bool
	globalOnly  bool
	projectDir  string
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove installed SkillsManager symlinks from agent dirs",
	Long: `Remove symlinks installed by SkillsManager from agent skill dirs.
Does not remove Registry originals or profiles; to delete a Registry original
(and its known installs) use sm rm <name> instead.

Default scope: project (./<agent>/skills) and global (~/<agent>/skills).
Use --project or --global to narrow. Filter with --agent and --skill.
Use --all -y to remove every SkillsManager symlink in the selected scope.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir, err := project.ResolveProjectDir(uninstallDir)
		if err != nil {
			return err
		}
		if uninstallAll {
			uninstallAgents = []string{"*"}
			uninstallSkills = []string{"*"}
		}
		if uninstallAll && !uninstallYes {
			return fmt.Errorf("--all requires --yes")
		}
		if uninstallProject && uninstallGlobal {
			return fmt.Errorf("use only one of --project or --global")
		}

		removed, err := removeInstalledSymlinks(uninstallOptions{
			homeDir:     home.Dir(),
			agents:      uninstallAgents,
			skills:      uninstallSkills,
			projectOnly: uninstallProject,
			globalOnly:  uninstallGlobal,
			projectDir:  projectDir,
		})
		if err != nil {
			return err
		}
		if removed == 0 {
			fmt.Println("No installed skills found to remove")
		} else {
			fmt.Printf("✓ Removed %d symlink(s)\n", removed)
		}
		return nil
	},
}

func removeInstalledSymlinks(opts uninstallOptions) (int, error) {
	targetTools := tool.ToolsByNames(opts.agents)
	if len(opts.agents) == 0 {
		targetTools = tool.AllTools()
	}
	if len(targetTools) == 0 {
		return 0, fmt.Errorf("no matching agents found for: %v", opts.agents)
	}

	// 默认项目+全局；--project / --global 收窄
	scanProject := !opts.globalOnly
	scanGlobal := !opts.projectOnly

	removed := 0
	for _, t := range targetTools {
		var dirs []string
		if scanGlobal && t.SkillDir != "" {
			dirs = append(dirs, filepath.Join(opts.homeDir, t.SkillDir))
		}
		if scanProject {
			if d := tool.GetProjectSkillDir(t, opts.projectDir); d != "" {
				dirs = append(dirs, d)
			}
		}
		for _, dir := range dirs {
			entries, err := os.ReadDir(dir)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return removed, err
			}
			for _, entry := range entries {
				if !matchesAny(entry.Name(), opts.skills) {
					continue
				}
				linkPath := filepath.Join(dir, entry.Name())
				// 仅移除指向 registry 的 symlink（Link Install 的标准形态）
				if !symlink.IsSymlink(linkPath) || !symlink.PointInside(linkPath, RegistryDir) {
					// 也接受指向 source cache 的链接（历史/边缘）
					if !symlink.IsSymlink(linkPath) || !symlink.PointInside(linkPath, filepath.Join(DataDir, "sources")) {
						continue
					}
				}
				if err := os.Remove(linkPath); err != nil {
					return removed, err
				}
				fmt.Printf("  Removed: %s\n", linkPath)
				// Clean lockfile entry for project-scope removals.
				if scanProject && !opts.globalOnly {
					lm := lockfile.NewManager(opts.projectDir)
					if lm.Exists() {
						if err := lm.Remove(entry.Name()); err != nil {
							fmt.Fprintf(os.Stderr, "warning: removing %s from lockfile: %v\n", entry.Name(), err)
						}
					}
				}
				removed++
			}
		}
	}
	return removed, nil
}

func matchesAny(name string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, filter := range filters {
		if filter == "*" || filter == name {
			return true
		}
	}
	return false
}

func init() {
	uninstallCmd.Flags().StringArrayVarP(&uninstallAgents, "agent", "a", nil, "Target specific agents (use '*' for all)")
	uninstallCmd.Flags().StringArrayVarP(&uninstallSkills, "skill", "s", nil, "Target specific skills (use '*' for all)")
	uninstallCmd.Flags().BoolVar(&uninstallProject, "project", false, "Only project skill dirs (./<agent>/skills)")
	uninstallCmd.Flags().BoolVarP(&uninstallGlobal, "global", "g", false, "Only global skill dirs (~/<agent>/skills)")
	uninstallCmd.Flags().StringVar(&uninstallDir, "dir", "", "Project directory (default: current dir)")
	uninstallCmd.Flags().BoolVar(&uninstallAll, "all", false, "Remove all SkillsManager symlinks from selected scope")
	uninstallCmd.Flags().BoolVarP(&uninstallYes, "yes", "y", false, "Confirm destructive --all uninstall")
	rootCmd.AddCommand(uninstallCmd)
}

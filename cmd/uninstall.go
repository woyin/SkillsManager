// cmd/uninstall.go 实现 `sm uninstall`：按 scope/agent/skill 从代理目录移除
// 由 sm 安装的符号链接（不影响注册表与 profiles）。
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
)

var (
	uninstallAgents  []string
	uninstallSkills  []string
	uninstallProject bool
	uninstallDir     string
	uninstallAll     bool
	uninstallYes     bool
)

type uninstallOptions struct {
	homeDir    string
	agents     []string
	skills     []string
	project    bool
	projectDir string
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove installed SkillsManager symlinks from tool directories",
	Long: `Remove symlinks installed by SkillsManager from tool skill directories.
Does not remove registry entries or profiles.

Default scope is global agent skill directories. Use --project to target current project directories.
Filter with --agent and --skill. Use --all -y to explicitly remove every registry symlink.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, _ := os.UserHomeDir()
		projectDir := uninstallDir
		if projectDir == "" {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			projectDir = wd
		}
		if uninstallAll {
			uninstallAgents = []string{"*"}
			uninstallSkills = []string{"*"}
		}
		if uninstallAll && !uninstallYes {
			return fmt.Errorf("--all requires --yes")
		}

		removed, err := removeInstalledSymlinks(uninstallOptions{
			homeDir:    home,
			agents:     uninstallAgents,
			skills:     uninstallSkills,
			project:    uninstallProject,
			projectDir: projectDir,
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

	removed := 0
	for _, t := range targetTools {
		skillDir := t.SkillDir
		baseDir := opts.homeDir
		if opts.project {
			skillDir = t.ProjectSkillDir
			baseDir = opts.projectDir
		}
		if skillDir == "" {
			continue
		}
		dir := filepath.Join(baseDir, skillDir)
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
			if !symlink.IsSymlink(linkPath) || !symlink.PointInside(linkPath, RegistryDir) {
				continue
			}
			if err := os.Remove(linkPath); err != nil {
				return removed, err
			}
			fmt.Printf("  Removed: %s\n", linkPath)
			removed++
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
	uninstallCmd.Flags().BoolVar(&uninstallProject, "project", false, "Target project skill directories instead of global agent directories")
	uninstallCmd.Flags().StringVar(&uninstallDir, "dir", "", "Project directory for --project (default: current dir)")
	uninstallCmd.Flags().BoolVar(&uninstallAll, "all", false, "Remove all SkillsManager symlinks from selected scope")
	uninstallCmd.Flags().BoolVarP(&uninstallYes, "yes", "y", false, "Confirm destructive --all uninstall")
	rootCmd.AddCommand(uninstallCmd)
}

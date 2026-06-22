// cmd/uninstall.go 实现 `sm uninstall`：从全部代理目录移除
// 由 sm 安装的符号链接（不影响注册表与 profiles）。
// cmd/uninstall.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove all installed skills from tool directories",
	Long: `Remove all symlinks installed by SkillsManager from all tool skill directories.
Does not remove the registry entries or profiles.
Symlinks are global (shared across projects), so this removes every registry symlink.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, _ := os.UserHomeDir()
		removed := 0

		for _, t := range tool.AllTools() {
			dir := filepath.Join(home, t.SkillDir)
			entries, err := os.ReadDir(dir)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return err
			}

			for _, entry := range entries {
				linkPath := filepath.Join(dir, entry.Name())
				if !symlink.IsSymlink(linkPath) {
					continue
				}
				if symlink.PointInside(linkPath, RegistryDir) {
					os.Remove(linkPath)
					fmt.Printf("  Removed: %s\n", linkPath)
					removed++
				}
			}
		}

		if removed == 0 {
			fmt.Println("No installed skills found to remove")
		} else {
			fmt.Printf("✓ Removed %d symlink(s)\n", removed)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}

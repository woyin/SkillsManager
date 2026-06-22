// cmd/check.go 实现 `sm check`：扫描已安装符号链接与项目记录，
// 报告失效链接、孤立链接、丢失项目；--fix 可自动修复。
// cmd/check.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/db"
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
)

var checkFix bool

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check installation integrity",
	Long: `Scan installed symlinks and project records.
Report broken symlinks, missing projects, and orphaned entries.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, _ := os.UserHomeDir()
		issues := 0

		// 检查所有工具技能目录中的符号链接
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
				if !symlink.Verify(linkPath) {
					issues++
					fmt.Printf("⚠ Broken symlink: %s\n", linkPath)
					if checkFix {
						os.Remove(linkPath)
						fmt.Printf("  → Removed\n")
					}
					continue
				}
				if !symlink.PointInside(linkPath, RegistryDir) {
					issues++
					fmt.Printf("⚠ Orphaned symlink outside registry: %s\n", linkPath)
					if checkFix {
						os.Remove(linkPath)
						fmt.Printf("  → Removed\n")
					}
				}
			}
		}

		// 检查数据库中的项目记录
		dbPath := filepath.Join(DataDir, "sm.db")
		database, err := db.Open(dbPath)
		if err != nil {
			fmt.Printf("Note: No database found at %s\n", dbPath)
		} else {
			defer database.Close()

			projects, err := database.GetAllProjects()
			if err != nil {
				return err
			}

			for _, p := range projects {
				if _, err := os.Stat(p.Path); os.IsNotExist(err) {
					issues++
					fmt.Printf("⚠ Project directory missing: %s\n", p.Path)
					if checkFix {
						database.RemoveProject(p.Path)
						fmt.Printf("  → Removed from database\n")
					}
				}
			}
		}

		if issues == 0 {
			fmt.Println("✓ All installations healthy")
		} else {
			fmt.Printf("\nFound %d issue(s)\n", issues)
			if !checkFix {
				fmt.Println("Run with --fix to auto-repair")
			}
		}

		return nil
	},
}

func init() {
	checkCmd.Flags().BoolVar(&checkFix, "fix", false, "Auto-fix broken symlinks and stale records")

	rootCmd.AddCommand(checkCmd)
}

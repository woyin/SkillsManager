// cmd/check.go 实现 `sm check`：扫描已安装符号链接与项目记录，
// 报告失效链接、孤立链接、丢失项目；--fix 可自动修复。
//
// Input: fmt, os, path/filepath, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/home, github.com/woyin/skills-manager/internal/symlink, github.com/woyin/skills-manager/internal/tool
// Output: var checkCmd
// Pos: 控制层-check命令实现（安装完整性检查与自动修复）
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/home"
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
		symlinkIssues, err := checkSymlinks()
		if err != nil {
			return err
		}
		projectIssues, err := checkProjectRecords()
		if err != nil {
			return err
		}
		issues := symlinkIssues + projectIssues

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

// checkSymlinks 扫描所有工具技能目录下的符号链接，报告并可选修复
// 失效链接（broken）与指向 registry 之外的孤立链接（orphaned）。
// 返回发现的问题数。--fix 时直接移除问题链接。
func checkSymlinks() (int, error) {
	issues := 0
	for _, t := range tool.AllTools() {
		dir := filepath.Join(home.Dir(), t.SkillDir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return 0, err
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
	return issues, nil
}

// checkProjectRecords 校验数据库中记录的项目目录是否仍存在于磁盘。
// 返回丢失的项目数。--fix 时从数据库移除失效记录。
// 无数据库（未 init）时静默返回 0。
func checkProjectRecords() (int, error) {
	database, err := openDB()
	if err != nil {
		fmt.Printf("Note: No database found at %s\n", dbPath())
		return 0, nil
	}
	defer database.Close()

	projects, err := database.GetAllProjects()
	if err != nil {
		return 0, err
	}

	issues := 0
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
	return issues, nil
}

func init() {
	checkCmd.Flags().BoolVar(&checkFix, "fix", false, "Auto-fix broken symlinks and stale records")

	rootCmd.AddCommand(checkCmd)
}

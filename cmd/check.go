// cmd/check.go 实现 `sm check`：扫描已安装符号链接与项目记录，
// 报告失效链接、孤立链接、丢失项目；--fix 可自动修复。
// 扫描规则由 internal/health 提供，CLI 只负责呈现与修复，保证与 Web 同一判据。
//
// Input: fmt, os, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/health, github.com/woyin/skills-manager/internal/tool
// Output: var checkCmd
// Pos: 控制层-check命令实现（安装完整性检查与自动修复）
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/health"
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

// globalSkillDirs 返回所有 Agent 的全局技能目录，交给 internal/health 扫描。
func globalSkillDirs() []string {
	tools := tool.AllTools()
	dirs := make([]string, 0, len(tools))
	for _, t := range tools {
		dirs = append(dirs, tool.GetGlobalSkillDir(t))
	}
	return dirs
}

// checkSymlinks 扫描所有工具全局技能目录下的符号链接，报告并可选修复
// 失效链接（broken）与指向 registry 之外的孤立链接（orphaned）。
// 返回发现的问题数。--fix 时删除成功才报告 Removed，删除失败如实报告。
func checkSymlinks() (int, error) {
	report, err := health.Scan(globalSkillDirs(), RegistryDir, nil)
	if err != nil {
		return 0, err
	}
	for _, issue := range report.Issues {
		switch issue.Type {
		case health.IssueBrokenSymlink:
			fmt.Printf("⚠ Broken symlink: %s\n", issue.Path)
		case health.IssueOrphanedSymlink:
			fmt.Printf("⚠ Orphaned symlink outside registry: %s\n", issue.Path)
		default:
			continue
		}
		if !checkFix {
			continue
		}
		if err := os.Remove(issue.Path); err != nil {
			fmt.Printf("  → Remove failed: %v\n", err)
			continue
		}
		fmt.Printf("  → Removed\n")
	}
	return len(report.Issues), nil
}

// checkProjectRecords 校验数据库中记录的项目目录是否仍存在于磁盘。
// 返回丢失的项目数。--fix 时从数据库移除失效记录（失败如实报告）。
// 无数据库（未 init）时静默返回 0；数据库读取或目录 Stat 的其他错误返回 error。
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
	paths := make([]string, 0, len(projects))
	for _, p := range projects {
		paths = append(paths, p.Path)
	}

	report, err := health.Scan(nil, RegistryDir, paths)
	if err != nil {
		return 0, err
	}
	for _, issue := range report.Issues {
		if issue.Type != health.IssueMissingProject {
			continue
		}
		fmt.Printf("⚠ Project directory missing: %s\n", issue.Path)
		if !checkFix {
			continue
		}
		if err := database.RemoveProject(issue.Path); err != nil {
			fmt.Printf("  → Remove from database failed: %v\n", err)
			continue
		}
		fmt.Printf("  → Removed from database\n")
	}
	return len(report.Issues), nil
}

func init() {
	checkCmd.Flags().BoolVar(&checkFix, "fix", false, "Auto-fix broken symlinks and stale records")

	rootCmd.AddCommand(checkCmd)
}

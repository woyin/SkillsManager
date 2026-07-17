// cmd/add.go 实现 `sm add`：把技能/MCP 下载到本地注册表（registry）。
// 支持 GitHub 简写、完整 URL、SSH URL、本地路径。
// 注意：add 只负责下载/注册，不会安装到任何 agent 目录。
// 要安装到 agent 目录，请使用 `sm install <source> --agent ...`。
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/registry"
)

var (
	addFlags  = newSpecialFlags()
	addIsMCP  bool
	addList   bool
	addSkills []string
	addCopy   bool
)

var addCmd = &cobra.Command{
	Use:   "add <source> [category]",
	Short: "Download a skill or MCP into the local registry",
	Long: `Download a skill or MCP server definition into the local registry.

add does NOT install anything into agent skill directories.
To install downloaded skills into agent directories, use:
  sm install <source> --agent <agent> [--skill <name>]

Source formats:
  owner/repo                                   GitHub shorthand
  https://github.com/owner/repo                Full GitHub URL
  https://github.com/owner/repo/tree/main/skills/name  Direct skill path
  https://gitlab.com/org/repo                  GitLab URL
  git@github.com:owner/repo.git                SSH git URL
  ./my-local-skills                            Local path

Category is the directory name under registry/skills/ or registry/mcp/.
Use --global/--codex/--claude for special registry locations.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		source := args[0]
		reg := registry.New(RegistryDir)

		// --list: 仅发现并列出技能，不写入注册表
		if addList {
			return listSkillsFromSource(source)
		}

		// MCP 注册
		if addIsMCP {
			if err := reg.AddMCP(source); err != nil {
				return fmt.Errorf("adding MCP: %w", err)
			}
			fmt.Printf("✓ Added MCP %s\n", source)
			return nil
		}

		// 技能注册（下载到本地注册表）
		category := ""
		if len(args) > 1 {
			category = args[1]
		}
		special := addFlags.Resolve()

		added, err := reg.AddSkillWithOptions(source, category, special, addSkills, addCopy)
		if err != nil {
			return fmt.Errorf("adding skill: %w", err)
		}

		fmt.Printf("✓ Added skill(s) from %s\n", source)
		fmt.Println("  Run `sm list --skills` to see registered skills.")
		fmt.Println("  Run `sm install <source> --agent <agent>` to install into agent directories.")

		// 对入库技能做 frontmatter lint；问题以非阻塞警告形式输出到 stderr。
		printSkillLint(reg, added)
		return nil
	},
}

// printSkillLint 对刚入库的技能做 frontmatter lint，问题以非阻塞警告
// 形式输出到 stderr。lint 不影响 add 的退出码（始终 exit 0）。
func printSkillLint(reg *registry.Registry, added []string) {
	hasWarnings := false
	for _, rel := range added {
		res := reg.LintSkill(rel)
		if len(res.Findings) == 0 {
			continue
		}
		hasWarnings = true
		fmt.Fprintf(os.Stderr, "\n⚠ Lint warnings for %s:\n", rel)
		for _, line := range res.FormatLintFindings() {
			fmt.Fprintln(os.Stderr, line)
		}
	}
	if hasWarnings {
		fmt.Fprintf(os.Stderr, "\nThese issues do not block registration but may prevent the skill from being triggered by agents.\n")
	}
}

func init() {
	addFlags.Bind(addCmd, "Add to")
	addCmd.Flags().BoolVar(&addIsMCP, "mcp", false, "Add as MCP server definition")
	addCmd.Flags().BoolVarP(&addList, "list", "l", false, "List available skills in source without adding")
	addCmd.Flags().StringArrayVarP(&addSkills, "skill", "s", nil, "Add specific skills by name (use '*' for all)")
	addCmd.Flags().BoolVar(&addCopy, "copy", false, "Copy files into registry instead of symlinking")

	rootCmd.AddCommand(addCmd)
}

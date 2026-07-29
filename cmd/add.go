// cmd/add.go 实现 `sm add`：把技能/MCP 下载到本地注册表（registry）。
// 支持 GitHub 简写、完整 URL、SSH URL、本地路径。
// 注意：add 只负责下载/注册，不会安装到任何 agent 目录。
// 日常主路径是 `sm install <source>`（Direct Install）；add 仅注册。
//
// Input: fmt, os, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/registry
// Output: var addCmd, func printSkillLint
// Pos: 控制层-add命令实现（技能/MCP 注册到本地 registry）
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/lockfile"
	"github.com/woyin/skills-manager/internal/picker"
	"github.com/woyin/skills-manager/internal/registry"
	"golang.org/x/term"
)

var (
	addFlags     = newSpecialFlags()
	addIsMCP     bool
	addList      bool
	addSkills    []string
	addCopy      bool
	addFullDepth bool
)

var addCmd = &cobra.Command{
	Use:     "add <source> [category]",
	Aliases: []string{"a"},
	Short:   "Register a skill or MCP into the local registry",
	Long: `Register a skill or MCP into the local registry (does not install into agents).

Primary day-to-day path is Direct Install:
  sm install <source>

Use add when you only want registry originals (curation / later profile install).

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
		source := lockfile.ResolveAlias(args[0])
		reg := registry.New(RegistryDir)

		// --list: 仅发现并列出技能，不写入注册表
		if addList {
			return listSkillsFromSource(source, addFullDepth)
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

		// git 源且未指定 -s 时：发现仓库内容，若根无 SKILL.md 且发现到
		// skill，则让用户交互式选择要注册的技能（避免把整个集合仓库根
		// 目录当成单技能强塞，导致 missing SKILL.md）。单 skill 仓库直接
		// 落地。本地路径保持原有整体拷贝行为。
		if registry.IsGitURL(source) && len(addSkills) == 0 {
			picked, err := chooseSkillsFromGitSource(source)
			if err != nil {
				return err
			}
			if picked != nil {
				addSkills = picked
			}
		}

		added, err := reg.AddSkillWithOptions(source, category, special, addSkills, addCopy)
		if err != nil {
			return fmt.Errorf("adding skill: %w", err)
		}

		fmt.Printf("✓ Registered skill(s) from %s\n", source)
		fmt.Println("  Run `sm list --registry` to see registry contents.")
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
	addCmd.Flags().BoolVar(&addFullDepth, "full-depth", false, "Also discover SKILL.md outside standard skill dirs (e.g. examples/, tests/)")

	rootCmd.AddCommand(addCmd)
}

// chooseSkillsFromGitSource 针对未指定 -s 的 git 源，决定是否进入交互选择。
//
// 返回约定：
//   - (nil, nil)：无需交互，沿用原有 cloneAndAdd 流程（单 skill 仓库或带子路径的 tree URL）。
//   - (names, nil)：已选定若干技能名，交由 AddSkillWithOptions 走 cloneAndExtract 抽取。
//   - (_, err)：列名失败 / 取消 / 空选 / 非 TTY 集合仓库，应中止。
//
// 触发交互的条件：仓库根无 SKILL.md 且发现到 ≥1 个 skill。单 skill 合法仓库
// 不打扰用户；非 TTY 时列出可选项并提示用 -s 选择，不注册坏技能。
func chooseSkillsFromGitSource(source string) ([]string, error) {
	// 带 /tree/ 子路径的来源已指向具体技能目录，直接走原流程。
	if _, _, subPath, _ := registry.ParseTreeURL(source); subPath != "" {
		return nil, nil
	}

	cloneDest, tempDir, err := registry.CloneToTemp(source, "sm-add-discover-*")
	if err != nil {
		return nil, err
	}
	defer registry.RemoveCloneTemp(tempDir)

	// 根目录有 SKILL.md：整个仓库即一个合法 skill，直接走原流程。
	if _, err := os.Stat(filepath.Join(cloneDest, "SKILL.md")); err == nil {
		return nil, nil
	}

	discovered, err := registry.DiscoverSkillsWithOptions(cloneDest, registry.DiscoverOptions{FullDepth: addFullDepth, AutoFullDepth: true})
	if err != nil {
		return nil, fmt.Errorf("discovering skills: %w", err)
	}
	if len(discovered) == 0 {
		// 既无根 SKILL.md 也无子技能：原流程会注册一个坏技能，但不在此兜底，
		// 保持现有 lint 警告可见。交由后续流程处理。
		return nil, nil
	}

	// 非 TTY：列出可选项并提示用户用 -s 指定，避免注册坏技能。
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stderr, "Multiple skills found in %s. Use -s <name> to select:\n", source)
		for _, s := range discovered {
			if s.Description != "" {
				fmt.Fprintf(os.Stderr, "  - %s: %s\n", s.Name, s.Description)
			} else {
				fmt.Fprintf(os.Stderr, "  - %s\n", s.Name)
			}
		}
		return nil, fmt.Errorf("no skill selected (non-interactive terminal); rerun with -s <name>")
	}

	// 仅一个 skill：自动选中，不打扰用户。
	if len(discovered) == 1 {
		return []string{discovered[0].Name}, nil
	}

	items := make([]picker.Item, len(discovered))
	for i, s := range discovered {
		desc := truncate(s.Description, 60)
		items[i] = picker.Item{Label: s.Name, Detail: desc, Value: s.Name}
	}
	chosen, err := picker.PickMultiple("Select skills to register", items)
	if err != nil {
		return nil, err
	}
	if len(chosen) == 0 {
		return nil, fmt.Errorf("no skills selected")
	}
	return chosen, nil
}

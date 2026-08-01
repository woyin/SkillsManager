// cmd/find.go 实现 `sm find` 命令：按关键词搜索已安装的技能，
// 或在交互式终端中弹出 fzf 风格的选择器。
//
// 搜索范围包括：
//   - sm 注册表（registry/skills/*）
//   - 常见代理目录（~/.agents/skills、~/.claude/skills、~/.codex/skills）
//
// 性能要点：collectFindMatches 使用预计算的名称集合去重，
// 避免 O(n²) 的线性扫描；matchesQuery 把 query 仅做一次 ToLower。
//
// Input: fmt, os, path/filepath, strings, text/tabwriter, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/home, github.com/woyin/skills-manager/internal/picker, github.com/woyin/skills-manager/internal/registry, golang.org/x/term
// Output: var findCmd, type findMatch, func collectFindMatches, func runFind, func runFindPicker, func matchesQuery
// Pos: 控制层-find命令实现（关键词搜索/交互选择已安装技能）
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/picker"
	"github.com/woyin/skills-manager/internal/registry"
	"golang.org/x/term"
)

var findCmd = &cobra.Command{
	Use:     "find [query]",
	Aliases: []string{"search", "f", "s"},
	Short:   "Search for skills interactively or by keyword",
	Long: `Search the Registry (and installed agent dirs) interactively or by keyword.

Without arguments in an interactive terminal, shows an fzf-style picker
to browse and select skills. With a query, filters by keyword.

Examples:
  # Interactive picker (fzf-style browse)
  sm find

  # Search by keyword
  sm find typescript
  sm find "web design"
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		query := ""
		if len(args) > 0 {
			// 统一小写，便于后续无大小写敏感的匹配。
			query = strings.ToLower(strings.Join(args, " "))
		}
		return runFind(query)
	},
}

// findMatch 描述一条搜索命中的技能。
type findMatch struct {
	Name        string
	Category    string
	Description string
	Path        string
}

// collectFindMatches 收集所有可被 `sm find` 搜索到的技能。
// query 已预先小写化；空串表示收集全部。
//
// 用 seen map 做名称去重，把原本 O(n²) 的 alreadyFound 扫描改为
// O(n) 哈希查找 —— 当注册表与代理目录中存在大量同名技能时，
// 这一项在大输入下能带来数量级的提升。
func collectFindMatches(query string) ([]findMatch, error) {
	reg := registry.New(RegistryDir)

	// 先扫描注册表
	skills, err := reg.ListSkills()
	if err != nil {
		return nil, fmt.Errorf("listing skills: %w", err)
	}

	// 预分配合理容量；用 map 做 O(1) 去重。
	seen := make(map[string]bool)
	var matches []findMatch

	for category, names := range skills {
		for _, name := range names {
			skillPath := filepath.Join(RegistryDir, "skills", category, name)
			desc := readDescription(skillPath)
			seen[name] = true

			if query == "" || matchesQuery(name, desc, query) {
				matches = append(matches, findMatch{
					Name:        name,
					Category:    category,
					Description: desc,
					Path:        skillPath,
				})
			}
		}
	}

	// 再扫描常见代理目录中的已安装技能（去重）
	searchDirs := []string{
		filepath.Join(home.Dir(), ".agents", "skills"),
		filepath.Join(home.Dir(), ".claude", "skills"),
		filepath.Join(home.Dir(), ".codex", "skills"),
	}

	for _, dir := range searchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			// O(1) 去重：注册表里已经出现过就跳过。
			if seen[name] {
				continue
			}
			seen[name] = true

			skillPath := filepath.Join(dir, name)
			desc := readDescription(skillPath)

			if query == "" || matchesQuery(name, desc, query) {
				matches = append(matches, findMatch{
					Name:        name,
					Category:    filepath.Base(dir),
					Description: desc,
					Path:        skillPath,
				})
			}
		}
	}

	return matches, nil
}

// readDescription 读取 skillPath/SKILL.md 并返回其 frontmatter 中的
// description 字段；任何 I/O 或解析失败都返回空串（视为无描述）。
func readDescription(skillPath string) string {
	data, err := os.ReadFile(filepath.Join(skillPath, "SKILL.md"))
	if err != nil {
		return ""
	}
	return registry.ParseFrontmatterFromString(string(data))
}

func runFind(query string) error {
	matches, err := collectFindMatches(query)
	if err != nil {
		return err
	}

	if len(matches) == 0 {
		if query != "" {
			fmt.Printf("No skills found matching %q\n", query)
		} else {
			fmt.Println("No skills found. Use 'sm add' to add skills to the registry.")
		}
		return nil
	}

	// 交互式选择器：无关键词且处于 TTY 环境时启用。
	if query == "" && term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) {
		return runFindPicker(matches)
	}

	// 非交互或带关键词模式：输出表格。
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tCATEGORY\tDESCRIPTION")
	fmt.Fprintln(w, "----\t--------\t-----------")
	for _, m := range matches {
		desc := truncate(m.Description, 60)
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", m.Name, m.Category, desc)
	}
	w.Flush()
	fmt.Printf("\n%d skill(s) found\n", len(matches))
	return nil
}

func runFindPicker(matches []findMatch) error {
	items := make([]picker.Item, len(matches))
	for i, m := range matches {
		desc := m.Description
		if desc == "" {
			desc = "(no description)"
		}
		items[i] = picker.Item{
			Label:  m.Name,
			Detail: desc,
			Value:  m.Path,
		}
	}

	selected, err := picker.Pick("Browse Skills (enter to select, esc to quit)", items)
	if err != nil {
		// 用户取消
		return nil
	}

	// 打印选中技能的详情与完整 SKILL.md 内容。
	for _, m := range matches {
		if m.Path == selected {
			fmt.Printf("Name:        %s\n", m.Name)
			fmt.Printf("Category:    %s\n", m.Category)
			fmt.Printf("Path:        %s\n", m.Path)
			if m.Description != "" {
				fmt.Printf("Description: %s\n", m.Description)
			}
			skillMD := filepath.Join(m.Path, "SKILL.md")
			if content, err := os.ReadFile(skillMD); err == nil {
				fmt.Println()
				fmt.Println(string(content))
			}
			break
		}
	}
	return nil
}

// matchesQuery 判断技能名与描述是否匹配 query。
// name 与 desc 做一次 ToLower（源数据未规范化）。query 由调用方
// 预先小写化，但此函数对 query 也做防御性 ToLower 以保证直接调用
// 的测试与外部方不因大小写遗漏。多个关键词以空格分隔，全部命中
// 才返回 true（AND 语义）。
func matchesQuery(name, desc, query string) bool {
	name = strings.ToLower(name)
	desc = strings.ToLower(desc)
	query = strings.ToLower(query)
	for _, term := range strings.Fields(query) {
		if !strings.Contains(name, term) && !strings.Contains(desc, term) {
			return false
		}
	}
	return true
}

func init() {
	rootCmd.AddCommand(findCmd)
}

// cmd/lint.go 实现 `sm lint`：批量复审注册表 skills 的质量。
//
// 复用 registry.ScoreSkill（启发式评分）与 registry.LintSkill（frontmatter
// findings），不新增评分逻辑——纯组合与格式化。纯读，不改任何文件。
//
// Input: fmt, path/filepath, sort, strings, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/registry
// Output: var lintCmd, func summarizeFindings
// Pos: 控制层-lint命令实现
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/registry"
)

var lintStrict bool

var lintCmd = &cobra.Command{
	Use:   "lint [name...]",
	Short: "Lint and score all registered skills",
	Long: `Scan registered skills and report a heuristic quality score (0-100)
plus frontmatter findings for each.

Pass specific skill names to lint only those. With --strict, exit non-zero
when any skill has an error-level finding (useful in CI).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		reg := registry.New(RegistryDir)
		details, err := reg.ListSkillDetails()
		if err != nil {
			return fmt.Errorf("listing skills: %w", err)
		}

		// want 报告某 skill 是否在用户指定的 name 过滤集中（空集=全部）。
		want := func(name string) bool {
			if len(args) == 0 {
				return true
			}
			for _, a := range args {
				if name == a {
					return true
				}
			}
			return false
		}

		var b strings.Builder
		fmt.Fprintln(&b, "NAME\tSCORE\tSTATUS")
		fmt.Fprintln(&b, "----\t-----\t------")

		total, warnings, errors := 0, 0, 0
		categories := make([]string, 0, len(details))
		for c := range details {
			categories = append(categories, c)
		}
		sort.Strings(categories)
		for _, category := range categories {
			items := details[category]
			for _, d := range items {
				if !want(d.Name) {
					continue
				}
				total++
				rel := filepath.Join(category, d.Name)
				score := reg.ScoreSkill(rel)
				lint := reg.LintSkill(rel)

				status, detail := "✓", ""
				switch {
				case lint.HasErrors():
					status = "✗"
					errors++
				case len(lint.Findings) > 0:
					status = "⚠"
					warnings++
				}
				if len(lint.Findings) > 0 {
					detail = summarizeFindings(lint.Findings)
				} else if score.Total < 80 {
					detail = strings.Join(score.Notes, ", ")
				}

				fmt.Fprintf(&b, "%s\t%d\t%s %s\n", rel, score.Total, status, detail)
			}
		}

		fmt.Fprint(cmd.OutOrStdout(), b.String())
		fmt.Fprintf(cmd.OutOrStdout(), "\n%d skills, %d warnings, %d errors\n", total, warnings, errors)

		if lintStrict && errors > 0 {
			return fmt.Errorf("%d skill(s) have error-level findings", errors)
		}
		return nil
	},
}

// summarizeFindings 把 findings 折叠成一行简短描述（截断到 ~60 字符）。
func summarizeFindings(findings []registry.LintFinding) string {
	var msgs []string
	for _, f := range findings {
		msgs = append(msgs, f.Message)
	}
	s := strings.Join(msgs, "; ")
	if len([]rune(s)) > 60 {
		s = string([]rune(s)[:57]) + "..."
	}
	return s
}

func init() {
	lintCmd.Flags().BoolVar(&lintStrict, "strict", false, "Exit non-zero when any skill has error-level findings")
	rootCmd.AddCommand(lintCmd)
}

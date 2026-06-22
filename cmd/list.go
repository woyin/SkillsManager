// cmd/list.go 实现 `sm list`：列出注册表中的 skills 与 MCP，
// 支持 --skills / --mcp / --global / --agent 过滤。
// cmd/list.go
package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

var (
	listSkillsOnly bool
	listMCPOnly    bool
	listGlobal     bool
	listAgents     []string
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all skills and MCP in the registry",
	Long: `List all skills and MCP configurations in the registry.

With --global, lists only globally installed skills.
With --agent, filters by specific agent(s).

Examples:
  # List all
  sm list

  # List only skills
  sm list --skills

  # List only MCP
  sm list --mcp

  # List global skills
  sm list --global

  # Filter by specific agents
  sm list -a claude-code -a cursor
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Agent-specific listing
		if len(listAgents) > 0 {
			return listByAgent(cmd.OutOrStdout())
		}

		reg := registry.New(RegistryDir)
		return writeRegistryList(cmd.OutOrStdout(), reg, listSkillsOnly, listMCPOnly)
	},
}

func listByAgent(out io.Writer) error {
	targetTools := tool.ToolsByNames(listAgents)
	if len(targetTools) == 0 {
		return fmt.Errorf("no matching agents found for: %v", listAgents)
	}

	home, _ := os.UserHomeDir()
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	for _, t := range targetTools {
		dir := filepath.Join(home, t.SkillDir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			fmt.Fprintf(w, "%s: (not found)\n\n", t.Name)
			continue
		}

		fmt.Fprintf(w, "%s (%s):\n", t.Name, dir)
		fmt.Fprintln(w, "  NAME\tTYPE")
		fmt.Fprintln(w, "  ----\t----")

		count := 0
		for _, entry := range entries {
			if entry.Name() == ".gitkeep" {
				continue
			}
			linkPath := filepath.Join(dir, entry.Name())
			info, _ := os.Lstat(linkPath)
			if info == nil {
				continue
			}

			entryType := "dir"
			if info.Mode()&os.ModeSymlink != 0 {
				entryType = "symlink"
			}
			fmt.Fprintf(w, "  %s\t%s\n", entry.Name(), entryType)
			count++
		}

		if count == 0 {
			fmt.Fprintln(w, "  (no skills)")
		}
		fmt.Fprintln(w)
	}

	return w.Flush()
}

func writeRegistryList(out io.Writer, reg *registry.Registry, skillsOnly, mcpOnly bool) error {
	// Show skills unless --mcp was passed alone; show MCP unless --skills
	// was passed alone. When neither or both flags are set, show everything.
	showSkills := !mcpOnly
	showMCP := !skillsOnly

	skills, err := reg.ListSkills()
	if err != nil {
		return err
	}

	mcps, err := reg.ListMCP()
	if err != nil {
		return err
	}

	// Filter to global only if requested
	if listGlobal {
		filtered := make(map[string][]string)
		if names, ok := skills[registry.Global]; ok {
			filtered[registry.Global] = names
		}
		skills = filtered
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	if showSkills {
		fmt.Fprintln(w, "SKILLS:")
		fmt.Fprintln(w, "  CATEGORY\tNAME")
		fmt.Fprintln(w, "  --------\t----")
		for _, cat := range sortedSkillCategories(skills) {
			names := append([]string(nil), skills[cat]...)
			sort.Strings(names)
			for _, name := range names {
				special := ""
				if registry.IsSpecialDir(cat) {
					special = " *"
				}
				fmt.Fprintf(w, "  %s\t%s%s\n", cat, name, special)
			}
		}
	}

	if showMCP {
		if showSkills {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, "MCP:")
		fmt.Fprintln(w, "  NAME")
		fmt.Fprintln(w, "  ----")
		sort.Strings(mcps)
		for _, name := range mcps {
			fmt.Fprintf(w, "  %s\n", name)
		}
	}

	if showSkills && showMCP {
		fmt.Fprintf(w, "\nTotal: %d skills, %d MCP\n", countSkills(skills), len(mcps))
		fmt.Fprintln(w, "  (* = special directory with fixed install target)")
	} else if showSkills {
		fmt.Fprintf(w, "\nTotal: %d skills\n", countSkills(skills))
		fmt.Fprintln(w, "  (* = special directory with fixed install target)")
	} else if showMCP {
		fmt.Fprintf(w, "\nTotal: %d MCP\n", len(mcps))
	}

	return w.Flush()
}

func sortedSkillCategories(skills map[string][]string) []string {
	categories := make([]string, 0, len(skills))
	for cat := range skills {
		categories = append(categories, cat)
	}
	sort.Strings(categories)
	return categories
}

func countSkills(skills map[string][]string) int {
	count := 0
	for _, names := range skills {
		count += len(names)
	}
	return count
}

func init() {
	listCmd.Flags().BoolVar(&listSkillsOnly, "skills", false, "List only skills")
	listCmd.Flags().BoolVar(&listMCPOnly, "mcp", false, "List only MCP")
	listCmd.Flags().BoolVarP(&listGlobal, "global", "g", false, "List only global skills")
	listCmd.Flags().StringArrayVarP(&listAgents, "agent", "a", nil, "Filter by specific agents")

	rootCmd.AddCommand(listCmd)
}

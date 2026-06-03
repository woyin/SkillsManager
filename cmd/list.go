// cmd/list.go
package cmd

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/registry"
)

var (
	listSkillsOnly bool
	listMCPOnly    bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all skills and MCP in the registry",
	RunE: func(cmd *cobra.Command, args []string) error {
		reg := registry.New(RegistryDir)
		return writeRegistryList(cmd.OutOrStdout(), reg, listSkillsOnly, listMCPOnly)
	},
}

func writeRegistryList(out io.Writer, reg *registry.Registry, skillsOnly, mcpOnly bool) error {
	showSkills := !mcpOnly || skillsOnly
	showMCP := !skillsOnly || mcpOnly

	if skillsOnly && mcpOnly {
		showSkills = true
		showMCP = true
	}

	skills, err := reg.ListSkills()
	if err != nil {
		return err
	}

	mcps, err := reg.ListMCP()
	if err != nil {
		return err
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

	rootCmd.AddCommand(listCmd)
}

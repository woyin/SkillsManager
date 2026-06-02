// cmd/list.go
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/registry"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all skills and MCP in the registry",
	RunE: func(cmd *cobra.Command, args []string) error {
		reg := registry.New(RegistryDir)

		skills, err := reg.ListSkills()
		if err != nil {
			return err
		}

		mcps, err := reg.ListMCP()
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

		fmt.Fprintln(w, "SKILLS:")
		fmt.Fprintln(w, "  CATEGORY\tNAME")
		fmt.Fprintln(w, "  --------\t----")
		for cat, names := range skills {
			for _, name := range names {
				special := ""
				if registry.IsSpecialDir(cat) {
					special = " *"
				}
				fmt.Fprintf(w, "  %s\t%s%s\n", cat, name, special)
			}
		}

		fmt.Fprintln(w)
		fmt.Fprintln(w, "MCP:")
		fmt.Fprintln(w, "  NAME")
		fmt.Fprintln(w, "  ----")
		for _, name := range mcps {
			fmt.Fprintf(w, "  %s\n", name)
		}

		fmt.Fprintf(w, "\nTotal: %d skills, %d MCP\n", countSkills(skills), len(mcps))
		fmt.Fprintln(w, "  (* = special directory with fixed install target)")

		return w.Flush()
	},
}

func countSkills(skills map[string][]string) int {
	count := 0
	for _, names := range skills {
		count += len(names)
	}
	return count
}

func init() {
	rootCmd.AddCommand(listCmd)
}

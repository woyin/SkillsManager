// cmd/find.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/picker"
	"github.com/woyin/skills-manager/internal/registry"
	"golang.org/x/term"
)

var findCmd = &cobra.Command{
	Use:   "find [query]",
	Short: "Search for skills interactively or by keyword",
	Long: `Search for installed skills interactively or by keyword.

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
			query = strings.ToLower(strings.Join(args, " "))
		}
		return runFind(query)
	},
}

type findMatch struct {
	Name        string
	Category    string
	Description string
	Path        string
}

func collectFindMatches(query string) ([]findMatch, error) {
	reg := registry.New(RegistryDir)

	// Search the registry
	skills, err := reg.ListSkills()
	if err != nil {
		return nil, fmt.Errorf("listing skills: %w", err)
	}

	var matches []findMatch

	for category, names := range skills {
		for _, name := range names {
			skillPath := filepath.Join(RegistryDir, "skills", category, name)
			skillMD := filepath.Join(skillPath, "SKILL.md")
			desc := ""
			if data, err := os.ReadFile(skillMD); err == nil {
				desc = extractDescription(string(data))
			}

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

	// Also search any discovered skills from common directories
	home, _ := os.UserHomeDir()
	searchDirs := []string{
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".codex", "skills"),
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
			skillPath := filepath.Join(dir, name)
			skillMD := filepath.Join(skillPath, "SKILL.md")
			desc := ""
			if data, err := os.ReadFile(skillMD); err == nil {
				desc = extractDescription(string(data))
			}

			// Skip if already in matches
			alreadyFound := false
			for _, m := range matches {
				if m.Name == name {
					alreadyFound = true
					break
				}
			}
			if alreadyFound {
				continue
			}

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

	// Interactive picker mode: no query + interactive terminal
	if query == "" && term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) {
		return runFindPicker(matches)
	}

	// Non-interactive / keyword mode: print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tCATEGORY\tDESCRIPTION")
	fmt.Fprintln(w, "----\t--------\t-----------")
	for _, m := range matches {
		desc := m.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
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
		// User cancelled
		return nil
	}

	// Find the selected match and print details
	for _, m := range matches {
		if m.Path == selected {
			fmt.Printf("Name:        %s\n", m.Name)
			fmt.Printf("Category:    %s\n", m.Category)
			fmt.Printf("Path:        %s\n", m.Path)
			if m.Description != "" {
				fmt.Printf("Description: %s\n", m.Description)
			}
			// Print the full SKILL.md content
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

func matchesQuery(name, desc, query string) bool {
	name = strings.ToLower(name)
	desc = strings.ToLower(desc)
	query = strings.ToLower(query)
	terms := strings.Fields(query)
	for _, term := range terms {
		if !strings.Contains(name, term) && !strings.Contains(desc, term) {
			return false
		}
	}
	return true
}

func extractDescription(content string) string {
	if len(content) < 6 || content[:3] != "---" {
		return ""
	}
	rest := content[3:]
	endIdx := strings.Index(rest, "---")
	if endIdx < 0 {
		return ""
	}
	fm := rest[:endIdx]
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "description:") {
			desc := strings.TrimPrefix(line, "description:")
			desc = strings.TrimSpace(desc)
			if len(desc) >= 2 && (desc[0] == '"' || desc[0] == '\'') && desc[len(desc)-1] == desc[0] {
				desc = desc[1 : len(desc)-1]
			}
			return desc
		}
	}
	return ""
}

func init() {
	rootCmd.AddCommand(findCmd)
}

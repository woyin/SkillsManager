// cmd/status.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/aivo"
	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
)

var statusDir string

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show what's installed in the current project",
	Long: `Display the current project's .sm.json configuration and
the symlinks installed in each tool's skills directory.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := statusDir
		if projectDir == "" {
			var err error
			projectDir, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
		}

		// Load project config
		pm := project.NewManager(projectDir)
		config, err := pm.Load()
		if err != nil {
			return fmt.Errorf("loading project config: %w", err)
		}

		fmt.Printf("Project: %s\n", projectDir)
		if config.Profile != "" {
			fmt.Printf("Profile: %s\n", config.Profile)
		} else {
			fmt.Println("Profile: (none)")
		}
		if len(config.Skills) > 0 {
			fmt.Printf("Extra skills: %v\n", config.Skills)
		}
		if len(config.MCP) > 0 {
			fmt.Printf("Extra MCP: %v\n", config.MCP)
		}

		// Show installed symlinks
		home, _ := os.UserHomeDir()
		absRegistry, _ := filepath.Abs(RegistryDir)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "TOOL\tINSTALLED SKILLS")
		fmt.Fprintln(w, "----\t----------------")

		for _, t := range tool.AllTools() {
			dir := filepath.Join(home, t.SkillDir)
			entries, err := os.ReadDir(dir)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				continue
			}

			var installed []string
			for _, entry := range entries {
				linkPath := filepath.Join(dir, entry.Name())
				if !symlink.IsSymlink(linkPath) {
					continue
				}
				target, err := os.Readlink(linkPath)
				if err != nil {
					continue
				}
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(linkPath), target)
				}
				absTarget, _ := filepath.Abs(target)
				rel, err := filepath.Rel(absRegistry, absTarget)
				if err == nil && rel != ".." && len(rel) > 0 && rel[0] != '.' {
					installed = append(installed, entry.Name())
				}
			}

			if len(installed) > 0 {
				for i, name := range installed {
					if i == 0 {
						fmt.Fprintf(w, "%s\t%s\n", t.Name, name)
					} else {
						fmt.Fprintf(w, "\t%s\n", name)
					}
				}
			}
		}

		w.Flush()

		// Show aivo status
		printAivoStatus()

		return nil
	},
}

func printAivoStatus() {
	info := aivo.Detect()
	if !info.Installed {
		return
	}

	fmt.Println()
	fmt.Printf("aivo: %s (%s)\n", info.Path, info.Version)

	active := aivo.GetActiveKey()
	if active != nil {
		fmt.Printf("  Active key: %s → %s\n", active.Name, active.BaseURL)
	}

	stats := aivo.GetStats()
	if stats != nil {
		fmt.Printf("  Usage: %s tokens, %d sessions, %d models\n",
			formatTokenCount(stats.TotalTokens), stats.Sessions, stats.Models)
	}
}

func formatTokenCount(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func init() {
	statusCmd.Flags().StringVar(&statusDir, "dir", "", "Project directory (default: current dir)")
	rootCmd.AddCommand(statusCmd)
}

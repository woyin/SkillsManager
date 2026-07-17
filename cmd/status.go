// cmd/status.go 实现 `sm status`：项目健康一页纸——
// profile、项目/全局已装技能、断链/orphan 问题与修复提示，以及 aivo 状态。
package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/aivo"
	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
)

var statusDir string

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Project health: installed skills, issues, next steps",
	Long: `Show a one-page health view for the current project:

  • Project path and profile (if any)
  • Installed skills in project scope (./<agent>/skills)
  • Global installed skills summary (~/<agent>/skills)
  • Issues: broken symlinks, orphan skills (not updatable)
  • Suggested next commands

Also prints aivo status when installed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := statusDir
		if projectDir == "" {
			var err error
			projectDir, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
		}
		return writeProjectStatus(os.Stdout, projectDir)
	},
}

type skillIssue struct {
	kind string // broken | orphan
	name string
	path string
	hint string
}

type installedEntry struct {
	agent string
	scope string // project | global
	name  string
	kind  string // symlink | copy | broken
	path  string
}

func writeProjectStatus(out io.Writer, projectDir string) error {
	pm := project.NewManager(projectDir)
	config, err := pm.Load()
	if err != nil {
		return fmt.Errorf("loading project config: %w", err)
	}

	fmt.Fprintf(out, "Project: %s\n", projectDir)
	if config.Profile != "" {
		fmt.Fprintf(out, "Profile: %s\n", config.Profile)
	} else {
		fmt.Fprintln(out, "Profile: (none)")
	}
	if len(config.Skills) > 0 {
		fmt.Fprintf(out, "Extra skills (.sm.json): %v\n", config.Skills)
	}
	if len(config.MCP) > 0 {
		fmt.Fprintf(out, "Extra MCP (.sm.json): %v\n", config.MCP)
	}

	// Prefer detected agents for readability; fall back to all tools if none.
	agents := tool.DetectInstalled(tool.AllTools())
	if len(agents) == 0 {
		agents = tool.AllTools()
	}

	var projectEntries, globalEntries []installedEntry
	var issues []skillIssue
	seenOrphan := map[string]bool{}

	scan := func(t tool.Tool, scope, dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.Name() == ".gitkeep" {
				continue
			}
			linkPath := filepath.Join(dir, e.Name())
			info, err := os.Lstat(linkPath)
			if err != nil {
				continue
			}
			ent := installedEntry{agent: t.Name, scope: scope, name: e.Name(), path: linkPath}

			if info.Mode()&os.ModeSymlink != 0 {
				// broken if target missing
				if _, err := os.Stat(linkPath); err != nil {
					ent.kind = "broken"
					issues = append(issues, skillIssue{
						kind: "broken", name: e.Name(), path: linkPath,
						hint: "remove with: sm uninstall -s " + e.Name(),
					})
				} else {
					ent.kind = "symlink"
					// orphan if registry target has no git and no origin
					if symlink.PointInside(linkPath, RegistryDir) {
						if target, err := filepath.EvalSymlinks(linkPath); err == nil {
							if isOrphanSkillPath(target) && !seenOrphan[e.Name()] {
								seenOrphan[e.Name()] = true
								issues = append(issues, skillIssue{
									kind: "orphan", name: e.Name(), path: target,
									hint: "reinstall from source to enable update: sm install <source> -s " + e.Name(),
								})
							}
						}
					}
				}
			} else {
				ent.kind = "copy"
				// copy install: check registry same name for orphan
				if regPath, _ := registry.New(RegistryDir).FindSkillDir(e.Name()); regPath != "" {
					if isOrphanSkillPath(regPath) && !seenOrphan[e.Name()] {
						seenOrphan[e.Name()] = true
						issues = append(issues, skillIssue{
							kind: "orphan", name: e.Name(), path: regPath,
							hint: "reinstall from source to enable update: sm install <source> -s " + e.Name(),
						})
					}
				}
			}

			if scope == "project" {
				projectEntries = append(projectEntries, ent)
			} else {
				globalEntries = append(globalEntries, ent)
			}
		}
	}

	for _, t := range agents {
		if d := tool.GetProjectSkillDir(t, projectDir); d != "" {
			scan(t, "project", d)
		}
		if t.SkillDir != "" {
			scan(t, "global", filepath.Join(home.Dir(), t.SkillDir))
		}
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "INSTALLED (project)")
	fmt.Fprintln(w, "AGENT\tSKILL\tTYPE")
	fmt.Fprintln(w, "-----\t-----\t----")
	if len(projectEntries) == 0 {
		fmt.Fprintln(w, "(none)\t\t")
	} else {
		sort.Slice(projectEntries, func(i, j int) bool {
			if projectEntries[i].agent != projectEntries[j].agent {
				return projectEntries[i].agent < projectEntries[j].agent
			}
			return projectEntries[i].name < projectEntries[j].name
		})
		for _, e := range projectEntries {
			fmt.Fprintf(w, "%s\t%s\t%s\n", e.agent, e.name, e.kind)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "INSTALLED (global summary)")
	fmt.Fprintln(w, "AGENT\tCOUNT")
	fmt.Fprintln(w, "-----\t-----")
	globalCounts := map[string]int{}
	for _, e := range globalEntries {
		globalCounts[e.agent]++
	}
	if len(globalCounts) == 0 {
		fmt.Fprintln(w, "(none)\t")
	} else {
		names := make([]string, 0, len(globalCounts))
		for n := range globalCounts {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintf(w, "%s\t%d\n", n, globalCounts[n])
		}
	}
	_ = w.Flush()

	fmt.Fprintln(out)
	if len(issues) == 0 {
		fmt.Fprintln(out, "Issues: none")
	} else {
		fmt.Fprintf(out, "Issues: %d\n", len(issues))
		for _, iss := range issues {
			fmt.Fprintf(out, "  [%s] %s (%s)\n", iss.kind, iss.name, iss.path)
			if iss.hint != "" {
				fmt.Fprintf(out, "         → %s\n", iss.hint)
			}
		}
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Next:")
	fmt.Fprintln(out, "  sm install <source>     # Direct Install into this project")
	fmt.Fprintln(out, "  sm update               # refresh installed sources")
	fmt.Fprintln(out, "  sm list                 # list installed skills")
	fmt.Fprintln(out, "  sm doctor               # environment check")

	printAivoStatusTo(out)
	return nil
}

// isOrphanSkillPath reports registry skill abs path with neither .git nor .sm-origin.json.
func isOrphanSkillPath(skillAbs string) bool {
	if nearestGitRepo(skillAbs, RegistryDir) != "" {
		return false
	}
	if _, ok := readSkillOrigin(skillAbs); ok {
		return false
	}
	// only count paths inside registry skills tree
	return pathInside(skillAbs, filepath.Join(RegistryDir, "skills"))
}

func printAivoStatus() {
	printAivoStatusTo(os.Stdout)
}

func printAivoStatusTo(out io.Writer) {
	info := aivo.Detect()
	if !info.Installed {
		return
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "aivo: %s (%s)\n", info.Path, info.Version)

	active := aivo.GetActiveKey()
	if active != nil {
		fmt.Fprintf(out, "  Active key: %s → %s\n", active.Name, active.BaseURL)
	}

	stats := aivo.GetStats()
	if stats != nil {
		fmt.Fprintf(out, "  Usage: %s tokens, %d sessions, %d models\n",
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

// cmd/list.go 实现 `sm list`：默认列出已安装技能（Installed Skills）；
// 支持 --registry 看注册表，--global/--project/--agent 过滤安装位置。
//
// Input: fmt, io, os, path/filepath, sort, strings, text/tabwriter, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/home, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/internal/tool
// Output: var listCmd, func listInstalled, func resolveListAgents, func listByAgent, func writeRegistryList, func writeMCPRow, func summarizeTransports
// Pos: 控制层-list命令实现
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/lockfile"
	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

var (
	listSkillsOnly bool
	listMCPOnly    bool
	listGlobal     bool
	listAgents     []string
	listProject    bool   // --project: 仅扫项目级目录
	listDir        string // --dir: 项目根
	listRegistry   bool   // --registry: 列出注册表内容（旧默认）
	listJSON       bool   // --json: 输出 JSON 格式
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List installed skills (default) or registry contents",
	Long: `List Installed Skills by default (what agents can load).

Default: scan agent skill directories for the current project (./<agent>/skills)
and, unless --project is set, also global (~/<agent>/skills).
Use --agent to limit agents; default agents are those detected on PATH
(or all tools if none detected when listing).

Use --registry to list skill originals and MCP stored in the local registry
(the lifecycle store). --skills / --mcp apply to --registry mode.

Examples:
  # Installed skills (project + global)
  sm list

  # Only project-level installs
  sm list --project

  # Only global installs
  sm list --global

  # Filter by agent
  sm list -a claude

  # Registry inventory
  sm list --registry
  sm list --registry --mcp
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if listRegistry || listMCPOnly {
			// MCP 只存在于 registry；--mcp 隐式走 registry 视图
			reg := registry.New(RegistryDir)
			return writeRegistryList(cmd.OutOrStdout(), reg, listSkillsOnly, listMCPOnly)
		}
		if listJSON {
			return listInstalledJSON(cmd.OutOrStdout())
		}
		return listInstalled(cmd.OutOrStdout())
	},
}

// listInstalled 扫描 agent 技能目录，输出已安装技能。
func listInstalled(out io.Writer) error {
	targetTools, err := resolveListAgents()
	if err != nil {
		return err
	}

	projectDir := listDir
	if projectDir == "" {
		if wd, err := os.Getwd(); err == nil {
			projectDir = wd
		}
	}

	// 范围：默认项目+全局；--project 仅项目；--global 仅全局
	scanProject := true
	scanGlobal := true
	if listProject && !listGlobal {
		scanProject, scanGlobal = true, false
	}
	if listGlobal && !listProject {
		scanProject, scanGlobal = false, true
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	total := 0

	for _, t := range targetTools {
		type scanLoc struct {
			label string
			dir   string
		}
		var locs []scanLoc
		if scanProject && projectDir != "" {
			if d := tool.GetProjectSkillDir(t, projectDir); d != "" {
				locs = append(locs, scanLoc{"project", d})
			}
		}
		if scanGlobal {
			locs = append(locs, scanLoc{"global", filepath.Join(home.Dir(), t.SkillDir)})
		}

		for _, loc := range locs {
			entries, err := os.ReadDir(loc.dir)
			if err != nil {
				continue
			}
			names := make([]string, 0, len(entries))
			types := make(map[string]string, len(entries))
			for _, entry := range entries {
				if entry.Name() == ".gitkeep" {
					continue
				}
				linkPath := filepath.Join(loc.dir, entry.Name())
				info, err := os.Lstat(linkPath)
				if err != nil {
					continue
				}
				entryType := "dir"
				if info.Mode()&os.ModeSymlink != 0 {
					entryType = "symlink"
				}
				names = append(names, entry.Name())
				types[entry.Name()] = entryType
			}
			if len(names) == 0 {
				continue
			}
			sort.Strings(names)
			fmt.Fprintf(w, "%s [%s] (%s):\n", t.Name, loc.label, loc.dir)
			fmt.Fprintln(w, "  NAME\tTYPE")
			fmt.Fprintln(w, "  ----\t----")
			for _, name := range names {
				fmt.Fprintf(w, "  %s\t%s\n", name, types[name])
				total++
			}
			fmt.Fprintln(w)
		}
	}

	if total == 0 {
		fmt.Fprintln(w, "No installed skills found.")
		fmt.Fprintln(w, "  Try: sm install <source>")
		fmt.Fprintln(w, "  Or:  sm list --registry")
	} else {
		fmt.Fprintf(w, "Total: %d installed skill entr(y/ies)\n", total)
	}
	return w.Flush()
}

// jsonSkillEntry is the JSON representation of an installed skill for --json output.
type jsonSkillEntry struct {
	Name       string   `json:"name"`
	Path       string   `json:"path"`
	Scope      string   `json:"scope"`
	Agents     []string `json:"agents"`
	Type       string   `json:"type"`
	Source     *string  `json:"source"`
	SourceURL  *string  `json:"sourceUrl"`
	SourceType *string  `json:"sourceType"`
}

// listInstalledJSON scans agent skill directories and outputs installed skills as JSON.
// Output is a flat array; each entry has name, path, scope, agents, and source
// provenance from skills-lock.json (when available).
func listInstalledJSON(out io.Writer) error {
	targetTools, err := resolveListAgents()
	if err != nil {
		return err
	}

	projectDir := listDir
	if projectDir == "" {
		if wd, err := os.Getwd(); err == nil {
			projectDir = wd
		}
	}

	scanProject := true
	scanGlobal := true
	if listProject && !listGlobal {
		scanProject, scanGlobal = true, false
	}
	if listGlobal && !listProject {
		scanProject, scanGlobal = false, true
	}

	// Load project lockfile for source enrichment.
	var lock *lockfile.LocalLock
	if scanProject && projectDir != "" {
		lm := lockfile.NewManager(projectDir)
		if lm.Exists() {
			lock, _ = lm.Load()
		}
	}

	// Collect per-skill data: name → entry (merging agent info across tools).
	type collected struct {
		path   string
		scope  string
		agents []string
		typ    string
	}
	byName := make(map[string]*collected)

	for _, t := range targetTools {
		var locs []struct {
			label string
			dir   string
		}
		if scanProject && projectDir != "" {
			if d := tool.GetProjectSkillDir(t, projectDir); d != "" {
				locs = append(locs, struct {
					label string
					dir   string
				}{"project", d})
			}
		}
		if scanGlobal {
			locs = append(locs, struct {
				label string
				dir   string
			}{"global", filepath.Join(home.Dir(), t.SkillDir)})
		}

		for _, loc := range locs {
			entries, err := os.ReadDir(loc.dir)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if entry.Name() == ".gitkeep" {
					continue
				}
				linkPath := filepath.Join(loc.dir, entry.Name())
				info, err := os.Lstat(linkPath)
				if err != nil {
					continue
				}
				entryType := "dir"
				if info.Mode()&os.ModeSymlink != 0 {
					entryType = "symlink"
				}
				key := entry.Name() + "|" + loc.label
				c := byName[key]
				if c == nil {
					c = &collected{path: linkPath, scope: loc.label, typ: entryType}
					byName[key] = c
				}
				c.agents = append(c.agents, t.Name)
			}
		}
	}

	// Build sorted output.
	keys := make([]string, 0, len(byName))
	for k := range byName {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]jsonSkillEntry, 0, len(keys))
	for _, k := range keys {
		c := byName[k]
		// Extract skill name (before |).
		name := k
		if idx := strings.Index(k, "|"); idx >= 0 {
			name = k[:idx]
		}

		entry := jsonSkillEntry{
			Name:   name,
			Path:   c.path,
			Scope:  c.scope,
			Agents: c.agents,
			Type:   c.typ,
		}

		// Enrich with lockfile source.
		if lock != nil {
			if le := lock.Skills[name]; le != nil {
				entry.Source = &le.Source
				entry.SourceURL = &le.SourceURL
				entry.SourceType = &le.SourceType
			}
		}

		result = append(result, entry)
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(out, string(data))
	return nil
}

// resolveListAgents 决定 list 扫描哪些 agent。
// 显式 -a 优先；否则 Detected Agents；若无检测结果则扫全部工具（list 只读，不造目录）。
func resolveListAgents() ([]tool.Tool, error) {
	if len(listAgents) > 0 {
		targetTools := tool.ToolsByNames(listAgents)
		if len(targetTools) == 0 {
			return nil, fmt.Errorf("no matching agents found for: %v", listAgents)
		}
		return targetTools, nil
	}
	detected := tool.DetectInstalled(tool.AllTools())
	if len(detected) > 0 {
		return detected, nil
	}
	return tool.AllTools(), nil
}

// listByAgent 按 --agent 指定的代理列出其技能目录内容（含类型：dir/symlink）。
// 保留供既有测试使用。
func listByAgent(out io.Writer) error {
	targetTools := tool.ToolsByNames(listAgents)
	if len(targetTools) == 0 {
		return fmt.Errorf("no matching agents found for: %v", listAgents)
	}

	projectDir := ""
	if listProject {
		var err error
		projectDir, err = project.ResolveProjectDir(listDir)
		if err != nil {
			return err
		}
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	for _, t := range targetTools {
		var dir string
		if listProject {
			dir = tool.GetProjectSkillDir(t, projectDir)
			if dir == "" {
				fmt.Fprintf(w, "%s: (no project skill dir)\n\n", t.Name)
				continue
			}
		} else {
			dir = filepath.Join(home.Dir(), t.SkillDir)
		}
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

// writeRegistryList 把注册表中的 skills/MCP 写入 out。
func writeRegistryList(out io.Writer, reg *registry.Registry, skillsOnly, mcpOnly bool) error {
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
		fmt.Fprintln(w, "  NAME\tSERVERS\tTRANSPORT")
		fmt.Fprintln(w, "  ----\t-------\t---------")
		sort.Strings(mcps)
		for _, name := range mcps {
			writeMCPRow(w, reg, name)
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

func writeMCPRow(w io.Writer, reg *registry.Registry, name string) {
	transports, err := registry.MCPServerTransports(reg.GetMCPPath(name))
	if err != nil {
		fmt.Fprintf(w, "  %s\t1?\t(parse error)\n", name)
		return
	}
	if len(transports) == 0 {
		fmt.Fprintf(w, "  %s\t0\t-\n", name)
		return
	}
	summary := summarizeTransports(transports)
	fmt.Fprintf(w, "  %s\t%d\t%s\n", name, len(transports), summary)
	for _, t := range transports {
		detail := t.Detail
		if detail == "" {
			detail = "-"
		}
		fmt.Fprintf(w, "  \t· %s\t%s (%s)\n", t.Server, t.Transport, detail)
	}
}

func summarizeTransports(ts []registry.ServerTransport) string {
	seen := make(map[string]bool, len(ts))
	var order []string
	for _, t := range ts {
		if !seen[t.Transport] {
			seen[t.Transport] = true
			order = append(order, t.Transport)
		}
	}
	return strings.Join(order, ", ")
}

func init() {
	listCmd.Flags().BoolVar(&listSkillsOnly, "skills", false, "List only skills")
	listCmd.Flags().BoolVar(&listMCPOnly, "mcp", false, "List only MCP (registry view)")
	listCmd.Flags().BoolVarP(&listGlobal, "global", "g", false, "List only global installed skills")
	listCmd.Flags().StringArrayVarP(&listAgents, "agent", "a", nil, "Filter by specific agents")
	listCmd.Flags().BoolVar(&listProject, "project", false,
		"List only project-level installed skills (./<agent>/skills)")
	listCmd.Flags().StringVar(&listDir, "dir", "", "Project root (default: current dir)")
	listCmd.Flags().BoolVar(&listRegistry, "registry", false, "List registry contents instead of installed skills")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")

	rootCmd.AddCommand(listCmd)
}

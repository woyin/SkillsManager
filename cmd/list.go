// cmd/list.go 实现 `sm list`：默认显示个人 Registry 清单（ADR 0015）；
// --installed 列出 Installed Skills，并支持 --global/--project/--agent/--dir
// 过滤安装位置。--registry 是默认 Registry 视图的弃用别名。
//
// Input: fmt, io, os, path/filepath, sort, strings, text/tabwriter, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/home, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/internal/tool
// Output: var listCmd, func printInstalledSkills, func listInstalledJSON, func resolveListAgents, func listByAgent, func writeRegistryList, func writeMCPRow, func summarizeTransports, func scanSkillDir, type installedScanScope, func newInstalledScanScope, func lockfileProvenance
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
	listRegistry   bool   // --registry: 弃用 alias，等价于默认（Registry 视图）
	listInstalled  bool   // --installed: 列出已安装技能（翻转后的新显式 flag）
	listJSON       bool   // --json: 输出 JSON 格式
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List the Registry inventory (default) or installed skills",
	Long: `Show the personal Registry inventory by default (your reusable skill originals).
Use --installed to list Installed Skills (what agents can load in agent dirs).

--installed composes with --project, --global, --agent, and --dir filters.
Use --mcp to show only MCP definitions (Registry view).
--registry is a deprecated alias for the default Registry view.

Default agents for --installed are those detected on PATH (or all tools if none).

Examples:
  # Registry inventory (default)
  sm list

  # Installed skills
  sm list --installed

  # Only project-level installs
  sm list --installed --project

  # Only global installs
  sm list --installed --global

  # Filter by agent
  sm list --installed -a claude

  # Registry MCP definitions
  sm list --mcp
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// --registry 现为弃用 alias（默认即 Registry 视图）。
		if listRegistry {
			fmt.Fprintln(cmd.ErrOrStderr(), "warning: --registry is deprecated; the default view is now the Registry inventory")
		}
		// 默认 = Registry 视图；--installed 切到 Installed Skills。
		// --mcp 隐式走 Registry 视图（MCP 只存在于 registry）。
		if !listInstalled {
			reg := registry.New(RegistryDir)
			return writeRegistryList(cmd.OutOrStdout(), reg, listSkillsOnly, listMCPOnly)
		}
		if listJSON {
			return listInstalledJSON(cmd.OutOrStdout())
		}
		return printInstalledSkills(cmd.OutOrStdout())
	},
}

// printInstalledSkills 扫描 agent 技能目录，输出已安装技能。
func printInstalledSkills(out io.Writer) error {
	targetTools, err := resolveListAgents()
	if err != nil {
		return err
	}

	scope := newInstalledScanScope()

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	total := 0

	for _, g := range scope.scanGroups(targetTools) {
		entries := scanSkillDir(g.dir)
		if len(entries) == 0 {
			continue
		}
		fmt.Fprintf(w, "%s [%s] (%s):\n", g.agent, g.label, g.dir)
		fmt.Fprintln(w, "  NAME\tTYPE\tPLUGIN\tSOURCE")
		fmt.Fprintln(w, "  ----\t----\t------\t------")
		for _, e := range entries {
			source, plugin := lockfileProvenance(scope.lock, e.name)
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", e.name, e.typ, plugin, source)
			total++
		}
		fmt.Fprintln(w)
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

// scannedEntry 是技能目录下扫描出的一条安装项。
type scannedEntry struct {
	name string
	typ  string // "dir" 或 "symlink"
}

// scanSkillDir 读取 dir 下的已安装技能项（跳过 .gitkeep 与无法 Lstat 的项），
// 返回按名称升序的列表。目录不可读时返回 nil。
func scanSkillDir(dir string) []scannedEntry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []scannedEntry
	for _, entry := range entries {
		if entry.Name() == ".gitkeep" {
			continue
		}
		info, err := os.Lstat(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		typ := "dir"
		if info.Mode()&os.ModeSymlink != 0 {
			typ = "symlink"
		}
		out = append(out, scannedEntry{name: entry.Name(), typ: typ})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// agentSkillGroup 是一次扫描的单元：某个 agent 名下的一个技能目录。
type agentSkillGroup struct {
	agent string // 显示名（工具名或 eve:<subagent>）
	label string // 范围标签：project / global
	dir   string
}

// installedScanScope 描述 --installed 扫描的范围与上下文，
// 由 --project/--global/--dir flags 解析而来。
type installedScanScope struct {
	projectDir  string
	scanProject bool
	scanGlobal  bool
	lock        *lockfile.LocalLock // 项目级 skills-lock.json（存在时）
}

// newInstalledScanScope 依据 list flags 解析扫描范围，
// 并在项目范围内预加载 lockfile 供 source 标签使用。
func newInstalledScanScope() installedScanScope {
	projectDir := listDir
	if projectDir == "" {
		if wd, err := os.Getwd(); err == nil {
			projectDir = wd
		}
	}

	// 范围：默认项目+全局；--project 仅项目；--global 仅全局
	scanProject, scanGlobal := true, true
	if listProject && !listGlobal {
		scanProject, scanGlobal = true, false
	}
	if listGlobal && !listProject {
		scanProject, scanGlobal = false, true
	}

	s := installedScanScope{
		projectDir:  projectDir,
		scanProject: scanProject,
		scanGlobal:  scanGlobal,
	}
	if scanProject && projectDir != "" {
		lm := lockfile.NewManager(projectDir)
		if lm.Exists() {
			s.lock, _ = lm.Load()
		}
	}
	return s
}

// toolLocations 返回工具 t 的扫描位置（先 project 后 global）。
func (s installedScanScope) toolLocations(t tool.Tool) []agentSkillGroup {
	var locs []agentSkillGroup
	if s.scanProject && s.projectDir != "" {
		if d := tool.GetProjectSkillDir(t, s.projectDir); d != "" {
			locs = append(locs, agentSkillGroup{agent: t.Name, label: "project", dir: d})
		}
	}
	if s.scanGlobal {
		locs = append(locs, agentSkillGroup{
			agent: t.Name,
			label: "global",
			dir:   tool.GetGlobalSkillDir(t),
		})
	}
	return locs
}

// scanGroups 返回全部待扫描的 (agent, 范围, 目录) 分组，顺序与输出一致：
// 先各工具的 project/global 位置，最后是 Eve 子代理目录
// （agent/subagents/<name>/skills，不在 tool.AllTools() 扫描范围内，
// 对齐 npx skills 对 Eve 子代理的可见性）。
func (s installedScanScope) scanGroups(targetTools []tool.Tool) []agentSkillGroup {
	var groups []agentSkillGroup
	for _, t := range targetTools {
		groups = append(groups, s.toolLocations(t)...)
	}
	if s.scanProject && s.projectDir != "" {
		for _, pair := range listEveSubagentDirs(s.projectDir) {
			groups = append(groups, agentSkillGroup{agent: pair[0], label: "project", dir: pair[1]})
		}
	}
	return groups
}

// lockfileSkill 返回 name 在 lockfile 中的记录；lock 为 nil 时返回 nil。
func lockfileSkill(lock *lockfile.LocalLock, name string) *lockfile.SkillEntry {
	if lock == nil {
		return nil
	}
	return lock.Skills[name]
}

// lockfileProvenance 返回 name 在项目 lockfile 中的 source/plugin 标签；
// 无记录时 source 回退为 "local"。
func lockfileProvenance(lock *lockfile.LocalLock, name string) (source, plugin string) {
	source = "local"
	if le := lockfileSkill(lock, name); le != nil {
		if le.Source != "" {
			source = le.Source
		}
		plugin = le.PluginName
	}
	return source, plugin
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

	scope := newInstalledScanScope()

	// Collect per-skill data: name → entry (merging agent info across tools).
	type collected struct {
		path   string
		scope  string
		agents []string
		typ    string
	}
	byName := make(map[string]*collected)

	for _, g := range scope.scanGroups(targetTools) {
		for _, e := range scanSkillDir(g.dir) {
			key := e.name + "|" + g.label
			c := byName[key]
			if c == nil {
				c = &collected{path: filepath.Join(g.dir, e.name), scope: g.label, typ: e.typ}
				byName[key] = c
			}
			c.agents = append(c.agents, g.agent)
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
		if le := lockfileSkill(scope.lock, name); le != nil {
			entry.Source = &le.Source
			entry.SourceURL = &le.SourceURL
			entry.SourceType = &le.SourceType
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

// listEveSubagentDirs discovers Eve subagent skill directories under
// <projectDir>/agent/subagents/*/skills and returns each as (label, dir).
// These are outside the per-tool skill-dir scan, so list must include them
// explicitly to surface skills installed via --subagent. Mirrors npx skills,
// which scans getEveSubagentSkillsDir for every subagent.
func listEveSubagentDirs(projectDir string) [][2]string {
	if projectDir == "" {
		return nil
	}
	root := filepath.Join(projectDir, "agent", "subagents")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out [][2]string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sd := filepath.Join(root, e.Name(), "skills")
		if info, err := os.Stat(sd); err == nil && info.IsDir() {
			out = append(out, [2]string{"eve:" + e.Name(), sd})
		}
	}
	return out
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
			dir = tool.GetGlobalSkillDir(t)
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
		skills = filterGlobalSkills(skills)
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	if showSkills {
		writeSkillsList(w, skills)
	}
	if showMCP {
		if showSkills {
			fmt.Fprintln(w)
		}
		writeMCPList(w, reg, mcps)
	}

	writeRegistrySummary(w, showSkills, showMCP, countSkills(skills), len(mcps))
	return w.Flush()
}

// filterGlobalSkills 保留仅 global 分类的技能列表。
func filterGlobalSkills(skills map[string][]string) map[string][]string {
	filtered := make(map[string][]string)
	if names, ok := skills[registry.Global]; ok {
		filtered[registry.Global] = names
	}
	return filtered
}

// writeSkillsList 渲染 skills 分类表。
func writeSkillsList(w io.Writer, skills map[string][]string) {
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

// writeMCPList 渲染 MCP 表。
func writeMCPList(w io.Writer, reg *registry.Registry, mcps []string) {
	fmt.Fprintln(w, "MCP:")
	fmt.Fprintln(w, "  NAME\tSERVERS\tTRANSPORT")
	fmt.Fprintln(w, "  ----\t-------\t---------")
	sort.Strings(mcps)
	for _, name := range mcps {
		writeMCPRow(w, reg, name)
	}
}

// writeRegistrySummary 渲染尾部统计行。
func writeRegistrySummary(w io.Writer, showSkills, showMCP bool, skillCount, mcpCount int) {
	switch {
	case showSkills && showMCP:
		fmt.Fprintf(w, "\nTotal: %d skills, %d MCP\n", skillCount, mcpCount)
		fmt.Fprintln(w, "  (* = special directory with fixed install target)")
	case showSkills:
		fmt.Fprintf(w, "\nTotal: %d skills\n", skillCount)
		fmt.Fprintln(w, "  (* = special directory with fixed install target)")
	case showMCP:
		fmt.Fprintf(w, "\nTotal: %d MCP\n", mcpCount)
	}
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
	listCmd.Flags().BoolVar(&listInstalled, "installed", false, "List Installed Skills (agent dirs) instead of the Registry inventory")
	listCmd.Flags().BoolVarP(&listGlobal, "global", "g", false, "List only global installed skills")
	listCmd.Flags().StringArrayVarP(&listAgents, "agent", "a", nil, "Filter by specific agents")
	listCmd.Flags().BoolVar(&listProject, "project", false,
		"List only project-level installed skills (./<agent>/skills)")
	listCmd.Flags().StringVar(&listDir, "dir", "", "Project root (default: current dir)")
	listCmd.Flags().BoolVar(&listRegistry, "registry", false, "Deprecated alias for the default Registry view")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")

	rootCmd.AddCommand(listCmd)
}

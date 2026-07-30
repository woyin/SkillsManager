// cmd/install.go 实现 `sm install`：
//   - 无 source：把 profile 与额外 skills/MCP 安装到当前项目（创建符号链接 + 合并 .mcp.json），并写入数据库。
//   - 带 source（主路径 Direct Install）：从来源发现技能，写入 registry 原件，再 symlink/copy 到 agent 技能目录。
//     默认 Project Scope + Detected Agents；--global 装全局；无 -a 且本机无代理则失败。
//
// Input: fmt, os, path/filepath, runtime, sync, text/tabwriter, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/fsutil, github.com/woyin/skills-manager/internal/home, github.com/woyin/skills-manager/internal/installer, github.com/woyin/skills-manager/internal/picker, github.com/woyin/skills-manager/internal/project, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/internal/tool, golang.org/x/term
// Output: var installCmd, type installJob, func installFromRegistry, func installFromSource, func listSkillsFromSource, func installSkillsToAgents, func installSkillsConcurrently, func resolveInstallAgents
// Pos: 控制层-install命令实现（Direct Install/Registry Install/profile 安装技能到 agent 目录）
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/concurrency"
	"github.com/woyin/skills-manager/internal/fsutil"
	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/installer"
	"github.com/woyin/skills-manager/internal/lockfile"
	"github.com/woyin/skills-manager/internal/picker"
	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
	"golang.org/x/term"
)

var (
	installProfile string
	installDir     string

	// source-based install flags
	installList      bool
	installSkills    []string
	installAgents    []string
	installCopy      bool
	installYes       bool
	installAll       bool
	installGlobal    bool // --global: 装到全局 ~/<agent>/skills；默认项目级 ./<agent>/skills
	installRef       string
	installOffline   bool
	installFullDepth bool
	installFromReg   bool   // --from-registry: 按名从本地 registry 装，不 clone source
	installCategory  string // --category: Registry Install disambiguation
	installFromLock  bool   // --from-lock: restore from skills-lock.json
)

var installCmd = &cobra.Command{
	Use:     "install [source]",
	Aliases: []string{"i"},
	Short:   "Install skills into agent dirs (default: current project)",
	Long: `Install skills and MCP configurations.

Direct Install (primary path) — with a source argument:
  Discovers skills in the source (GitHub shorthand, URL, SSH, or local path),
  stores originals in the local registry, and symlinks them into agent skill dirs.

  Defaults:
    • Scope: project (./<agent>/skills under current directory). Use --global for ~/.
    • Agents: detected on this machine (CLI on PATH). Use --agent to override.
    • Skills: single skill installs immediately; multiple skills prompt in a TTY,
      or install all in non-TTY / with -y / --all.

  Examples:
    sm install owner/repo
    sm install owner/repo --global -a claude
    sm install ./my-skills -s foo -s bar
    sm install owner/repo -l

Registry Install — with a name and --from-registry:
  Installs already-registered skills by name from the local registry, with no
  source clone (fast). The skill must be in the registry (run sm add first) or
  it errors with no fallback. If a name matches multiple categories, pass
  --category <cat> to disambiguate.

  Examples:
    sm install my-skill --from-registry
    sm install my-skill --from-registry --copy
    sm install my-skill --from-registry --category codex-only
    sm install foo,bar --from-registry --global

Without a source argument (profile mode):
  Reads .sm.json if present, or uses --profile flag. Installs into the current
  project by default (./<agent>/skills); use --global for ~/<agent>/skills.
  Creates symlinks (or copies with --copy) and writes .mcp.json.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Registry Install：按名从本地 registry 装（不 clone）。
		if installFromReg {
			if len(args) != 1 {
				return fmt.Errorf("--from-registry requires exactly one skill name (or comma-separated names)")
			}
			return installFromRegistry(args[0])
		}

		// Lock Restore：从 skills-lock.json 恢复项目技能。
		if installFromLock {
			return installFromLockFile(args)
		}

		// source-based mode
		if len(args) == 1 {
			return installFromSource(args[0])
		}

		// profile/project mode (original behavior)
		projectDir, err := project.ResolveProjectDir(installDir)
		if err != nil {
			return err
		}

		pm := project.NewManager(projectDir)
		config, err := pm.Load()
		if err != nil {
			return fmt.Errorf("loading project config: %w", err)
		}

		profileName := installProfile
		if profileName == "" {
			profileName = config.Profile
		}

		extraSkills := config.Skills
		extraMCP := config.MCP

		if profileName == "" && len(extraSkills) == 0 && len(extraMCP) == 0 {
			return fmt.Errorf("nothing to install: create .sm.json with a profile, or use --profile flag")
		}

		tools := tool.DetectInstalled(tool.AllTools())
		if len(tools) == 0 {
			tools = tool.DefaultTools()
		}

		inst, err := installer.New(RegistryDir, ProfilesDir, tools)
		if err != nil {
			return fmt.Errorf("creating installer: %w", err)
		}
		// Profile Install 默认项目 scope（breaking：原为全局）；--global 切全局。
		inst.SetScope(projectDir, installGlobal)
		if installCopy {
			inst.SetCopyMode(true)
		}

		result, err := inst.Install(projectDir, profileName, extraSkills, extraMCP)
		if err != nil {
			return fmt.Errorf("install failed: %w", err)
		}

		database, err := openDB()
		if err != nil {
			return fmt.Errorf("opening database: %w", err)
		}
		defer database.Close()

		if err := database.RecordInstallation(projectDir, profileName, result.Skills, result.MCP); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to record installation: %v\n", err)
		}

		if err := database.UpsertProject(projectDir, profileName, extraSkills, extraMCP); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to update project record: %v\n", err)
		}

		fmt.Printf("✓ Installed to %s\n", projectDir)
		if profileName != "" {
			fmt.Printf("  Profile: %s\n", profileName)
		}
		if len(result.Skills) > 0 {
			fmt.Printf("  Skills: %d symlinks created\n", len(result.Skills))
			for _, s := range result.Skills {
				fmt.Printf("    → %s\n", s)
			}
		}
		if len(result.MCP) > 0 {
			fmt.Printf("  MCP: %v\n", result.MCP)
		}

		return nil
	},
}

// installFromRegistry handles `sm install <name> --from-registry`: install
// already-registered skills by name directly from the local registry, without
// cloning any source. Names are comma-separated. Scope defaults to project
// (--global for global); --copy produces Copy Install entities (with origin).
func installFromRegistry(namesArg string) error {
	names := splitAndTrim(namesArg)
	if len(names) == 0 {
		return fmt.Errorf("--from-registry requires at least one skill name")
	}

	projectDir, err := project.ResolveProjectDir(installDir)
	if err != nil {
		return err
	}

	tools := tool.DetectInstalled(tool.AllTools())
	if len(tools) == 0 {
		tools = tool.DefaultTools()
	}

	inst, err := installer.New(RegistryDir, ProfilesDir, tools)
	if err != nil {
		return fmt.Errorf("creating installer: %w", err)
	}
	inst.SetScope(projectDir, installGlobal)
	if installCopy {
		inst.SetCopyMode(true)
	}

	result, err := inst.InstallFromRegistry(names, installCategory)
	if err != nil {
		return fmt.Errorf("registry install failed: %w", err)
	}

	// 记录安装历史（与 profile 模式一致；Direct Install 路径当前不写 db）。
	if db, err := openDB(); err == nil {
		defer db.Close()
		scope := "project"
		if installGlobal {
			scope = "global"
		}
		profileLabel := fmt.Sprintf("registry-install/%s", scope)
		if rerr := db.RecordInstallation(projectDir, profileLabel, result.Skills, nil); rerr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to record installation: %v\n", rerr)
		}
	} else {
		fmt.Fprintf(os.Stderr, "warning: opening database: %v\n", err)
	}

	fmt.Printf("✓ Installed from registry to %s\n", projectDir)
	if len(result.Skills) > 0 {
		mode := "symlink"
		if installCopy {
			mode = "copy"
		}
		fmt.Printf("  Skills: %d %s(s) created\n", len(result.Skills), mode)
		for _, s := range result.Skills {
			fmt.Printf("    → %s\n", s)
		}
	}
	return nil
}

// installFromSource handles `sm install <source>`: Direct Install into agent dirs
// with registry originals. Default scope is project; default agents are Detected Agents.
func installFromSource(source string) error {
	parsed := registry.ParseSource(lockfile.ResolveAlias(source))
	source = parsed.Source()
	if parsed.SkillFilter != "" && len(installSkills) == 0 {
		installSkills = []string{parsed.SkillFilter}
	}
	if parsed.Ref != "" && installRef == "" {
		installRef = parsed.Ref
	}
	if installRef != "" && !registry.IsGitURL(source) {
		return fmt.Errorf("--ref requires a remote Git source")
	}
	// --list: discover only, no install
	if installList {
		return listSkillsFromSource(source, installFullDepth)
	}

	if installAll {
		installSkills = []string{"*"}
		installAgents = []string{"*"}
		installYes = true
	}

	// 默认 Project Scope；--global 切到全局。
	projectScope := !installGlobal
	projectDir := ""
	if projectScope {
		var err error
		projectDir, err = project.ResolveProjectDir(installDir)
		if err != nil {
			return err
		}
	}

	return installSkillsToAgents(source, installAgents, installSkills, installCopy, projectScope, projectDir)
}

// listSkillsFromSource clones (if needed) and lists discoverable skills.
// fullDepth 为 true 时同时发现标准技能目录之外的 SKILL.md（--full-depth）。
// 始终启用 AutoFullDepth：标准位置无技能时自动递归，对齐 npx skills。
func listSkillsFromSource(source string, fullDepth bool) error {
	opts := registry.DiscoverOptions{FullDepth: fullDepth, AutoFullDepth: true}
	if !registry.IsGitURL(source) {
		skills, err := registry.DiscoverSkillsWithOptions(source, opts)
		if err != nil {
			return fmt.Errorf("discovering skills: %w", err)
		}
		printDiscoveredSkills(skills)
		return nil
	}

	cloneDest, err := cachedGitSource(source, installRef, installOffline)
	if err != nil {
		return err
	}

	_, _, subPath, _ := registry.ParseTreeURL(source)
	subPath = registry.SanitizeSubpath(subPath)
	if subPath != "" {
		cloneDest = filepath.Join(cloneDest, subPath)
	}
	skills, err := registry.DiscoverSkillsWithOptions(cloneDest, opts)
	if err != nil {
		return fmt.Errorf("discovering skills: %w", err)
	}
	printDiscoveredSkills(skills)
	return nil
}

func printDiscoveredSkills(skills []registry.DiscoveredSkill) {
	if len(skills) == 0 {
		fmt.Println("No skills found in source.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION")
	fmt.Fprintln(w, "----\t-----------")
	for _, s := range skills {
		desc := truncate(s.Description, 60)
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Fprintf(w, "%s\t%s\n", s.Name, desc)
	}
	w.Flush()
	fmt.Printf("\n%d skill(s) found\n", len(skills))
}

// resolveInstallAgents 解析目标代理：显式 -a 优先；否则 Detected Agents。
// 无 Detected Agents 时回退到 DefaultTools（{Claude, Codex, Pi}），保证
// 在一个本机没有任何 agent CLI 的环境仍能落地到常见三件套目录。
func resolveInstallAgents(agentNames []string) ([]tool.Tool, error) {
	if len(agentNames) > 0 {
		targetTools := tool.ToolsByNames(agentNames)
		if len(targetTools) == 0 {
			return nil, fmt.Errorf("no matching agents found for: %v", agentNames)
		}
		return targetTools, nil
	}

	detected := tool.DetectInstalled(tool.AllTools())
	if len(detected) == 0 {
		return tool.DefaultTools(), nil
	}
	return detected, nil
}

// discoverSkillsFromSource 从来源发现技能（git 走缓存克隆，本地直接扫）。
// sourceRoot 在 git 源时为缓存克隆根目录，供写 .sm-origin.json；本地源为空。
func discoverSkillsFromSource(source string) (skills []registry.DiscoveredSkill, sourceRoot string, err error) {
	opts := registry.DiscoverOptions{FullDepth: installFullDepth, AutoFullDepth: true}
	if registry.IsGitURL(source) {
		cloneDest, err := cachedGitSource(source, installRef, installOffline)
		if err != nil {
			return nil, "", err
		}

		_, _, subPath, _ := registry.ParseTreeURL(source)
		subPath = registry.SanitizeSubpath(subPath)
		if subPath != "" {
			skillDir := filepath.Join(cloneDest, subPath)
			skillMD := filepath.Join(skillDir, "SKILL.md")
			name := filepath.Base(subPath)
			desc := ""
			if _, err := os.Stat(skillMD); err == nil {
				desc = registry.ParseFrontmatterDescription(skillMD)
			}
			return []registry.DiscoveredSkill{{
				Name: name, Description: desc, Path: skillDir, SkillMDPath: skillMD,
			}}, cloneDest, nil
		}
		discovered, err := registry.DiscoverSkillsWithOptions(cloneDest, opts)
		return discovered, cloneDest, err
	}
	discovered, err := registry.DiscoverSkillsWithOptions(source, opts)
	return discovered, "", err
}

// selectSkillsForInstall 按 skillNames / TTY / -y 规则选出要装的技能。
// - 显式 -s / *：按 filterSkills
// - 单个 skill：直接装
// - 多个 skill：TTY 交互多选；非 TTY 或 -y/--all 全装
func selectSkillsForInstall(discovered []registry.DiscoveredSkill, skillNames []string) ([]registry.DiscoveredSkill, error) {
	if len(skillNames) > 0 {
		filtered := filterSkills(discovered, skillNames)
		if len(filtered) == 0 {
			return nil, fmt.Errorf("no matching skills found in source")
		}
		return filtered, nil
	}
	if len(discovered) == 0 {
		return nil, fmt.Errorf("no matching skills found in source")
	}
	if len(discovered) == 1 {
		return discovered, nil
	}

	// 多 skill：-y 或非 TTY → 全装；TTY → 交互多选
	if installYes || !term.IsTerminal(int(os.Stdin.Fd())) {
		return discovered, nil
	}

	items := make([]picker.Item, len(discovered))
	for i, s := range discovered {
		desc := truncate(s.Description, 60)
		items[i] = picker.Item{Label: s.Name, Detail: desc, Value: s.Name}
	}
	chosen, err := picker.PickMultiple("Select skills to install", items)
	if err != nil {
		return nil, err
	}
	if len(chosen) == 0 {
		return nil, fmt.Errorf("no skills selected")
	}
	return filterSkills(discovered, chosen), nil
}

// ensureSkillsInRegistry 把选中技能写入 registry/global/。
// 已存在则用当前源内容覆盖（重装即刷新），并在 git 源时写入 .sm-origin.json
// 以便 sm update 能通过 source cache 回写。
// originSource/originRef/sourceRoot：git 安装时传入；本地安装 sourceRoot 为空则不写 origin。
func ensureSkillsInRegistry(skills []registry.DiscoveredSkill, originSource, originRef, sourceRoot string) (map[string]string, error) {
	reg := registry.New(RegistryDir)
	paths := make(map[string]string, len(skills))
	commit := ""
	if sourceRoot != "" {
		commit = gitHeadHash(sourceRoot)
	}

	for _, s := range skills {
		var regPath string
		if existing, err := reg.FindSkillDir(s.Name); err == nil && existing != "" {
			// 同名覆盖：警告旧路径与旧 origin（若有）
			oldHint := existing
			if old, ok := readSkillOrigin(existing); ok {
				oldHint = fmt.Sprintf("%s (was from %s)", existing, old.Source)
			}
			fmt.Fprintf(os.Stderr, "warning: overwriting existing registry skill %q at %s\n", s.Name, oldHint)
			if _, err := replaceSkillDir(s.Path, existing, false); err != nil {
				return nil, fmt.Errorf("refreshing skill %q in registry: %w", s.Name, err)
			}
			regPath = existing
		} else {
			added, err := reg.AddSkillWithOptions(s.Path, registry.Global, "", nil, true)
			if err != nil {
				return nil, fmt.Errorf("registering skill %q: %w", s.Name, err)
			}
			if len(added) > 0 {
				regPath = filepath.Join(RegistryDir, "skills", added[0])
			} else if p, _ := reg.FindSkillDir(s.Name); p != "" {
				regPath = p
			} else {
				return nil, fmt.Errorf("registered skill %q but could not resolve path", s.Name)
			}
		}

		if sourceRoot != "" && originSource != "" {
			rel := "."
			if r, err := filepath.Rel(sourceRoot, s.Path); err == nil {
				rel = r
			}
			if err := writeSkillOrigin(regPath, skillOrigin{
				Source:  originSource,
				Ref:     originRef,
				RelPath: rel,
				Commit:  commit,
			}); err != nil {
				return nil, fmt.Errorf("writing origin for %q: %w", s.Name, err)
			}
		}
		paths[s.Name] = regPath
	}
	return paths, nil
}

// installSkillsToAgents installs discovered skills into each target agent's skill dir.
// 流程：发现 → 选择 → 写入 registry 原件 → symlink/copy 到 agent 目录。
func installSkillsToAgents(source string, agentNames, skillNames []string, copyMode bool, project bool, projectDir string) error {
	targetTools, err := resolveInstallAgents(agentNames)
	if err != nil {
		return err
	}

	discovered, sourceRoot, err := discoverSkillsFromSource(source)
	if err != nil {
		return fmt.Errorf("discovering skills: %w", err)
	}

	skillsToInstall, err := selectSkillsForInstall(discovered, skillNames)
	if err != nil {
		return err
	}

	// 写入 registry 原件；git 源记录 .sm-origin.json 以便 update 回写
	originSource, originRef := "", ""
	if sourceRoot != "" {
		originSource, originRef = source, installRef
	}
	regPaths, err := ensureSkillsInRegistry(skillsToInstall, originSource, originRef, sourceRoot)
	if err != nil {
		return err
	}

	// 用 registry 路径替换 skill.Path，保证 symlink 指向 registry
	for i := range skillsToInstall {
		if p, ok := regPaths[skillsToInstall[i].Name]; ok {
			skillsToInstall[i].Path = p
		}
	}

	jobs := make([]installJob, 0, len(targetTools)*len(skillsToInstall))
	for _, t := range targetTools {
		var agentSkillDir string
		if project {
			agentSkillDir = tool.GetProjectSkillDir(t, projectDir)
			if agentSkillDir == "" {
				continue // 该工具无项目级目录，跳过
			}
		} else {
			agentSkillDir = filepath.Join(home.Dir(), t.SkillDir)
		}
		for _, skill := range skillsToInstall {
			jobs = append(jobs, installJob{
				tool:     t,
				skill:    skill,
				dest:     filepath.Join(agentSkillDir, skill.Name),
				agentDir: agentSkillDir,
			})
		}
	}

	if len(jobs) == 0 {
		return fmt.Errorf("no installable agent skill directories for the selected agents (project scope needs ProjectSkillDir)")
	}

	results := installSkillsConcurrently(jobs, copyMode)

	installed := 0
	for _, ok := range results {
		if ok {
			installed++
		}
	}

	fmt.Printf("\n✓ Installed %d skill(s) to %d agent(s)", installed, len(targetTools))
	if project {
		fmt.Printf(" [project: %s]", projectDir)
		// Project-scope Direct Install: write skills-lock.json for reproducibility.
		writeProjectLock(source, sourceRoot, skillsToInstall, projectDir)
	} else {
		fmt.Print(" [global]")
	}
	fmt.Println()
	return nil
}

// installJob is one (agent, skill) install target.
type installJob struct {
	tool     tool.Tool
	skill    registry.DiscoveredSkill
	dest     string
	agentDir string
}

// installSkillsConcurrently 通过有限 worker 池并发安装技能。
// 每个 job 写入不同目标路径，彼此独立；MkdirAll 是幂等的，
// 对同一 agent 目录的并发调用是安全的。
func installSkillsConcurrently(jobs []installJob, copyMode bool) []bool {
	results := make([]bool, len(jobs))
	if len(jobs) == 0 {
		return results
	}

	var outMu sync.Mutex // 序列化进度输出，避免行交错

	// doInstall 处理单个安装任务，结果写回 results[i]。
	doInstall := func(i int) {
		j := jobs[i]
		var err error
		if copyMode {
			err = copySkillDir(j.skill.Path, j.dest)
		} else {
			if mkErr := os.MkdirAll(j.agentDir, 0755); mkErr != nil {
				outMu.Lock()
				fmt.Fprintf(os.Stderr, "warning: creating dir for %s: %v\n", j.tool.Name, mkErr)
				outMu.Unlock()
				return
			}
			absSrc, _ := filepath.Abs(j.skill.Path)
			os.Remove(j.dest)
			err = os.Symlink(absSrc, j.dest)
		}
		if err != nil {
			outMu.Lock()
			fmt.Fprintf(os.Stderr, "warning: install %s for %s: %v\n", j.skill.Name, j.tool.Name, err)
			outMu.Unlock()
			return
		}
		outMu.Lock()
		fmt.Printf("  ✓ %s → %s\n", j.skill.Name, j.dest)
		outMu.Unlock()
		results[i] = true
	}

	// 并发上限：I/O 密集型允许真实并行，封顶 8 避免压垮主机。
	concurrency.RunIndexed(len(jobs), 8, doInstall)
	return results
}

// filterSkills keeps skills whose names match; "*" returns all.
func filterSkills(discovered []registry.DiscoveredSkill, names []string) []registry.DiscoveredSkill {
	if len(names) == 0 {
		return discovered
	}
	for _, n := range names {
		if n == "*" {
			return discovered
		}
	}
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	var filtered []registry.DiscoveredSkill
	for _, s := range discovered {
		if nameSet[s.Name] {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// writeProjectLock writes skills-lock.json entries for project-scope Direct Install.
// Each installed skill gets an entry with source metadata and content hash.
func writeProjectLock(source, sourceRoot string, skills []registry.DiscoveredSkill, projectDir string) {
	lm := lockfile.NewManager(projectDir)
	meta := lockfile.SourceMeta{}
	resolvedSource := lockfile.ResolveAlias(source)
	if sourceRoot != "" {
		meta = lockfile.ClassifySource(resolvedSource)
	} else {
		// local path source
		meta = lockfile.SourceMeta{SourceType: "local", SourceURL: ""}
	}

	entries := make(map[string]*lockfile.SkillEntry, len(skills))
	for _, s := range skills {
		skillPath := ""
		if sourceRoot != "" {
			if rel, err := filepath.Rel(sourceRoot, s.Path); err == nil {
				skillPath = filepath.ToSlash(filepath.Join(rel, "SKILL.md"))
			}
		}
		hash, err := lockfile.ComputeHash(s.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: hashing %s for lockfile: %v\n", s.Name, err)
			continue
		}
		entries[s.Name] = &lockfile.SkillEntry{
			Source:       resolvedSource,
			SourceType:   meta.SourceType,
			SourceURL:    meta.SourceURL,
			SkillPath:    skillPath,
			Ref:          installRef,
			ComputedHash: hash,
			PluginName:   s.PluginName,
		}
	}

	if err := lm.UpsertMany(entries); err != nil {
		fmt.Fprintf(os.Stderr, "warning: writing skills-lock.json: %v\n", err)
	}
}

// installFromLockFile restores project skills from skills-lock.json.
// Each lock entry's source is re-installed, reproducing the exact skill set.
func installFromLockFile(args []string) error {
	projectDir, err := project.ResolveProjectDir(installDir)
	if err != nil {
		return err
	}

	lm := lockfile.NewManager(projectDir)
	if !lm.Exists() {
		return fmt.Errorf("no skills-lock.json found in %s", projectDir)
	}

	lock, err := lm.Load()
	if err != nil {
		return fmt.Errorf("reading skills-lock.json: %w", err)
	}

	names := lock.SortedNames()
	if len(names) == 0 {
		fmt.Println("No skills found in skills-lock.json.")
		return nil
	}

	fmt.Printf("Restoring %d skill(s) from skills-lock.json into %s\n", len(names), projectDir)

	// Group skills by source for batch install (one clone per source).
	type sourceGroup struct {
		source string
		names  []string
	}
	groups := make(map[string]*sourceGroup)
	var order []string // preserve stable output

	for _, name := range names {
		entry := lock.Skills[name]
		// local sources can't be re-installed from lock alone.
		if entry.SourceType == "local" || entry.Source == "local" {
			fmt.Fprintf(os.Stderr, "  ✗ %s: local source, cannot restore from lock\n", name)
			continue
		}
		installSource := entry.Source
		if entry.SourceURL != "" {
			installSource = entry.SourceURL
		}
		if g, ok := groups[installSource]; ok {
			g.names = append(g.names, name)
		} else {
			groups[installSource] = &sourceGroup{source: installSource, names: []string{name}}
			order = append(order, installSource)
		}
	}

	totalInstalled := 0
	for _, src := range order {
		g := groups[src]
		// Install each source group: re-discover and install the locked skills.
		fmt.Printf("  → %s (%d skill(s))\n", src, len(g.names))

		// Temporarily set installRef from lock entry if available.
		firstEntry := lock.Skills[g.names[0]]
		savedRef := installRef
		if firstEntry.Ref != "" {
			installRef = firstEntry.Ref
		}

		err := installSkillsToAgents(src, installAgents, g.names, installCopy, true, projectDir)
		installRef = savedRef
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", src, err)
			continue
		}
		totalInstalled += len(g.names)
	}

	fmt.Printf("\n✓ Restored %d skill(s) from skills-lock.json\n", totalInstalled)
	return nil
}

// copySkillDir copies a skill dir, overwriting an existing destination.
func copySkillDir(src, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		os.RemoveAll(dest)
	}
	return fsutil.CopyDir(src, dest)
}

func init() {
	installCmd.Flags().StringVar(&installProfile, "profile", "", "Profile name to install")
	installCmd.Flags().StringVar(&installDir, "dir", "", "Project directory (default: current dir)")

	// source-based install flags
	installCmd.Flags().BoolVarP(&installList, "list", "l", false, "List available skills in source without installing")
	installCmd.Flags().StringArrayVarP(&installSkills, "skill", "s", nil, "Install specific skills by name (use '*' for all)")
	installCmd.Flags().StringArrayVarP(&installAgents, "agent", "a", nil, "Target specific agents (default: detected on PATH; '*' = all)")
	installCmd.Flags().BoolVar(&installCopy, "copy", false, "Copy files instead of symlinking into agent dirs")
	installCmd.Flags().BoolVarP(&installYes, "yes", "y", false, "Skip confirmation prompts (install all skills when multiple found)")
	installCmd.Flags().BoolVar(&installAll, "all", false, "Install all skills to all agents without prompts")
	installCmd.Flags().BoolVarP(&installGlobal, "global", "g", false,
		"Install into global skill dirs (~/<agent>/skills) instead of project (./<agent>/skills)")
	// --project 保留为 no-op 兼容旧脚本：默认已是项目级
	installCmd.Flags().BoolP("project", "p", false, "Install into project-level skill dirs (default; kept for compatibility)")
	_ = installCmd.Flags().MarkHidden("project")
	installCmd.Flags().StringVar(&installRef, "ref", "", "Snapshot remote source at a Git branch, tag, or commit (use commit for reproducibility)")
	installCmd.Flags().BoolVar(&installFullDepth, "full-depth", false, "Also discover SKILL.md outside standard skill dirs (e.g. examples/, tests/)")
	installCmd.Flags().BoolVar(&installOffline, "offline", false, "Use exact cached source/ref without network access")
	installCmd.Flags().BoolVar(&installFromReg, "from-registry", false, "Install skill(s) by name from the local registry (no source clone)")
	installCmd.Flags().StringVar(&installCategory, "category", "", "With --from-registry: pick this category when the name matches several")
	installCmd.Flags().BoolVar(&installFromLock, "from-lock", false, "Restore project skills from skills-lock.json (reproducible install)")

	rootCmd.AddCommand(installCmd)
}

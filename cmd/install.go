// cmd/install.go 实现 `sm install`：
//   - 无 source：把 profile 与额外 skills/MCP 安装到当前项目（创建符号链接 + 合并 .mcp.json），并写入数据库。
//   - 裸名称（Registry Install）：按全局唯一名称从 Registry 安装，不联网。
//   - 带 source（Direct Install）：从来源发现技能，写入 registry 原件，再 symlink/copy 到 agent 技能目录。
//     默认 Project Scope + Detected Agents；--global 装全局；无 -a 且本机无代理则失败。
//
// Input: context, errors, fmt, os, path/filepath, runtime, sort, strings, time, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/installer, github.com/woyin/skills-manager/internal/lockfile, github.com/woyin/skills-manager/internal/picker, github.com/woyin/skills-manager/internal/project, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/internal/tool, github.com/woyin/skills-manager/internal/wellknown, golang.org/x/term
// Output: var installCmd, type installJob, func installFromRegistry, func installFromSource, func listSkillsFromSource, func installSkillsToAgents, func installSkillsConcurrently, func resolveInstallAgents, func printDiscoveredSkills, func kebabToTitle
// Pos: 控制层-install命令实现（Direct Install/Registry Install/profile 安装技能到 agent 目录）
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/installer"
	"github.com/woyin/skills-manager/internal/lockfile"
	"github.com/woyin/skills-manager/internal/picker"
	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
	"github.com/woyin/skills-manager/internal/wellknown"
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
	installFromReg   bool     // --from-registry: 按名从本地 registry 装，不 clone source
	installCategory  string   // --category: Registry Install disambiguation
	installFromLock  bool     // --from-lock: restore from skills-lock.json
	installSubagents []string // --subagent: Eve 子代理名（可重复），装到 agent/subagents/<name>/skills
)

// fetchWellKnownSkills keeps Well-Known Source acquisition at one command
// boundary. Production uses the protocol client; tests replace it with a
// deterministic in-memory source so install behavior is verified without
// external network I/O.
var fetchWellKnownSkills = wellknown.FetchAll

var installCmd = &cobra.Command{
	Use:     "install [source]",
	Aliases: []string{"i"},
	Short:   "Install skills into agent dirs (default: current project)",
	Long: `Install skills and MCP configurations.

Registry Install (primary reuse) — with a bare skill name:
  Installs an already-registered skill by name from the local Registry, with no
  network access. The skill must be in the Registry (run ` + "`sm add`" + ` first) or it
  errors with a Register hint and never falls back to a clone.

  Examples:
    sm install my-skill
    sm install my-skill --global -a claude
    sm install foo,bar

Direct Install — with a source argument (repository, URL, or local path):
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


Without a source argument (profile mode):
  Reads .sm.json if present, or uses --profile flag. Installs into the current
  project by default (./<agent>/skills); use --global for ~/<agent>/skills.
  Creates symlinks (or copies with --copy) and writes .mcp.json.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// --from-registry 现为弃用兼容 alias：裸名称已默认走 Registry Install。
		if installFromReg {
			fmt.Fprintln(os.Stderr, "warning: --from-registry is deprecated; a bare skill name now selects Registry Install by default")
		}
		mode, arg, err := classifyInstallRequest(args, installFromReg, installFromLock)
		if err != nil {
			return err
		}
		switch mode {
		case registryInstallMode:
			return installFromRegistry(arg)
		case directInstallMode:
			return installFromSource(arg)
		case lockRestoreMode:
			return installFromLockFile(args)
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
		return fmt.Errorf("registry install requires at least one skill name")
	}
	// --category 不再用于身份歧义（全局唯一）；保留为弃用诊断 flag。
	if installCategory != "" {
		fmt.Fprintln(os.Stderr, "warning: --category is deprecated for identity disambiguation; skill names are globally unique in the Registry")
	}

	// 预检：每个名称必须存在且唯一，否则零副作用（绝不联网）。
	reg := registry.New(RegistryDir)
	for _, name := range names {
		if _, err := reg.ResolveUniqueSkill(name); err != nil {
			return fmt.Errorf("registry install: %w", err)
		}
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
	if wellknown.IsSource(source) {
		return installFromWellKnownSource(source)
	}
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

func installFromWellKnownSource(source string) error {
	if installRef != "" || installOffline {
		return fmt.Errorf("--ref and --offline are not supported for a Well-Known Source")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	skills, err := fetchWellKnownSkills(ctx, source)
	if err != nil {
		return fmt.Errorf("discovering Well-Known Source: %w", err)
	}
	if installList {
		discovered := make([]registry.DiscoveredSkill, 0, len(skills))
		for _, skill := range skills {
			discovered = append(discovered, registry.DiscoveredSkill{Name: skill.InstallName, Description: skill.Description})
		}
		printDiscoveredSkills(discovered)
		return nil
	}
	if installAll {
		installSkills = []string{"*"}
		installAgents = []string{"*"}
		installYes = true
	}
	projectScope := !installGlobal
	projectDir := ""
	if projectScope {
		projectDir, err = project.ResolveProjectDir(installDir)
		if err != nil {
			return err
		}
	}
	return installSkillsToAgents(source, installAgents, installSkills, installCopy, projectScope, projectDir)
}

func materializeWellKnownSkills(skills []wellknown.Skill) ([]registry.DiscoveredSkill, string, error) {
	tempRoot, err := os.MkdirTemp("", "sm-well-known-*")
	if err != nil {
		return nil, "", fmt.Errorf("creating Well-Known Source workspace: %w", err)
	}
	cleanup := func(err error) ([]registry.DiscoveredSkill, string, error) {
		_ = os.RemoveAll(tempRoot)
		return nil, "", err
	}
	discovered := make([]registry.DiscoveredSkill, 0, len(skills))
	for _, skill := range skills {
		skillDir := filepath.Join(tempRoot, skill.InstallName)
		for filePath, content := range skill.Files {
			path := filepath.Join(skillDir, filepath.FromSlash(filePath))
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return cleanup(fmt.Errorf("creating Well-Known Source skill directory: %w", err))
			}
			if err := os.WriteFile(path, content, 0644); err != nil {
				return cleanup(fmt.Errorf("writing Well-Known Source file: %w", err))
			}
		}
		discovered = append(discovered, registry.DiscoveredSkill{
			Name:        skill.InstallName,
			Description: skill.Description,
			Path:        skillDir,
			SkillMDPath: filepath.Join(skillDir, "SKILL.md"),
		})
	}
	return discovered, tempRoot, nil
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

	// Group by pluginName (aligned with npx skills --list output): skills
	// declared by a plugin manifest are shown under a title-cased group
	// header; ungrouped skills appear last under "General" when any groups
	// exist, or flat otherwise.
	grouped := map[string][]registry.DiscoveredSkill{}
	var ungrouped []registry.DiscoveredSkill
	for _, s := range skills {
		if s.PluginName != "" {
			grouped[s.PluginName] = append(grouped[s.PluginName], s)
		} else {
			ungrouped = append(ungrouped, s)
		}
	}
	groups := make([]string, 0, len(grouped))
	for g := range grouped {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	hasGroups := len(groups) > 0

	printSkill := func(s registry.DiscoveredSkill) {
		fmt.Printf("  %s\n", s.Name)
		desc := truncate(s.Description, 60)
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Printf("    %s\n", desc)
	}

	for _, g := range groups {
		fmt.Println(kebabToTitle(g))
		for _, s := range grouped[g] {
			printSkill(s)
		}
		fmt.Println()
	}
	if len(ungrouped) > 0 {
		if hasGroups {
			fmt.Println("General")
		}
		for _, s := range ungrouped {
			printSkill(s)
		}
		if hasGroups {
			fmt.Println()
		}
	}
	fmt.Printf("\n%d skill(s) found\n", len(skills))
}

// kebabToTitle converts a kebab-case plugin name to Title Case for display
// (e.g. "cloudflare-workers" -> "Cloudflare Workers"), matching npx skills.
func kebabToTitle(s string) string {
	parts := strings.Split(s, "-")
	for i, w := range parts {
		if w == "" {
			continue
		}
		parts[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(parts, " ")
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
	if wellknown.IsSource(source) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		fetched, err := fetchWellKnownSkills(ctx, source)
		if err != nil {
			return nil, "", err
		}
		return materializeWellKnownSkills(fetched)
	}

	// 当用户通过 --skill 明确选择技能时，允许内部技能可见，对齐 npx
	// skills 的 includeInternal 选择器语义。
	opts := registry.DiscoverOptions{
		FullDepth:       installFullDepth,
		AutoFullDepth:   true,
		IncludeInternal: len(installSkills) > 0,
	}
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
	// 解析 ref kind（Direct Install 也记录 ref kind，ADR 0014）。
	refKind := registry.RefDefaultBranch
	commit := ""
	if sourceRoot != "" {
		commit = gitHeadHash(sourceRoot)
		_, rk, rerr := registry.ResolveRefKind(originRef, sourceRoot)
		if rerr == nil {
			refKind = rk
		}
	}

	for _, s := range skills {
		// 用统一 Register 原语：写入前验证、跨来源保护、ref-kind origin。
		origin := registry.SkillOrigin{SubPath: "."}
		if sourceRoot != "" && originSource != "" {
			rel := "."
			if r, err := filepath.Rel(sourceRoot, s.Path); err == nil {
				rel = r
			}
			origin = registry.SkillOrigin{
				SourceKind: registry.SourceGit,
				Source:     originSource,
				Ref:        originRef,
				RefKind:    refKind,
				SubPath:    rel,
				Commit:     commit,
			}
		} else if originSource != "" && registry.IsGitURL(originSource) {
			origin = registry.SkillOrigin{
				SourceKind: registry.SourceGit,
				Source:     originSource,
				Ref:        originRef,
				RefKind:    refKind,
				SubPath:    ".",
			}
		} else {
			// 本地来源：Snapshot。
			origin = registry.SkillOrigin{
				SourceKind: registry.SourceLocalSnapshot,
				Source:     originSource,
				SubPath:    ".",
			}
		}
		// Direct Install 对同名不同源默认覆盖（历史行为），但提示影响。
		res, err := reg.Register(s.Path, registry.Global, origin, true)
		if err != nil {
			return nil, fmt.Errorf("registering skill %q: %w", s.Name, err)
		}
		paths[s.Name] = res.Path
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

	// --subagent implies Eve: ensure the Eve agent is a target so subagent
	// installs work even without an explicit -a eve. Mirrors npx skills,
	// which adds "eve" to targetAgents when --subagent is supplied.
	if len(installSubagents) > 0 && !containsToolName(targetTools, "eve") {
		if eve := tool.ToolByName("eve"); eve != nil {
			targetTools = append(targetTools, *eve)
		}
	}

	discovered, sourceRoot, err := discoverSkillsFromSource(source)
	if err != nil {
		return fmt.Errorf("discovering skills: %w", err)
	}
	if wellknown.IsSource(source) {
		defer os.RemoveAll(sourceRoot)
	}

	skillsToInstall, err := selectSkillsForInstall(discovered, skillNames)
	if err != nil {
		return err
	}

	// 写入 registry 原件；git 源记录 .sm-origin.json 以便 update 回写
	originSource, originRef := "", ""
	if sourceRoot != "" && registry.IsGitURL(source) {
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

	// Eve subagent targets: --subagent is repeatable. "root"/"." means the
	// root Eve agent (plain agent/skills dir, no override); any other name
	// installs into agent/subagents/<name>/skills. Mirrors npx skills, which
	// builds one install target per (skill × eve subagent) and maps root/.
	// to the root agent.
	jobs := make([]installJob, 0, len(targetTools)*len(skillsToInstall))
	for _, t := range targetTools {
		if t.Name == "eve" && len(installSubagents) > 0 {
			for _, sub := range installSubagents {
				var dir string
				if sub == "root" || sub == "." || sub == "" {
					dir = tool.GetProjectSkillDir(t, projectDir)
					if dir == "" {
						continue
					}
				} else {
					dir = filepath.Join(projectDir, "agent", "subagents", sub, "skills")
				}
				for _, skill := range skillsToInstall {
					jobs = append(jobs, installJob{
						tool:  t,
						skill: skill,
						dest:  filepath.Join(dir, skill.Name),
					})
				}
			}
			continue
		}
		scope := installer.ProjectScope
		if !project {
			scope = installer.GlobalScope
		}
		for _, target := range installer.TargetDirectories([]tool.Tool{t}, projectDir, scope) {
			for _, skill := range skillsToInstall {
				jobs = append(jobs, installJob{
					tool:  t,
					skill: skill,
					dest:  filepath.Join(target.Directory, skill.Name),
				})
			}
		}
	}

	if len(jobs) == 0 {
		return fmt.Errorf("no installable agent skill directories for the selected agents (project scope needs ProjectSkillDir)")
	}

	// Deduplicate jobs by destination path. Multiple agents may share a
	// single skill directory in the same scope (e.g. project-scope codex,
	// gemini, cursor and other "universal" agents all resolve to
	// .agents/skills). Without dedup the concurrent installer would race on
	// the same destination (os.Remove + os.Symlink from several goroutines),
	// causing failures and redundant work. Keep the first job per unique dest
	// so each skill lands in each directory exactly once. Mirrors npx skills,
	// where universal agents collapse onto a single canonical skills dir.
	jobs = dedupeJobsByDest(jobs)

	results := installSkillsConcurrently(jobs, copyMode)

	installed := 0
	installedAgents := make(map[string]bool)
	for i, j := range jobs {
		if results[i] {
			installed++
			installedAgents[j.tool.Name] = true
		}
	}

	fmt.Printf("\n✓ Installed %d skill(s) to %d agent(s)", installed, len(installedAgents))
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
	tool  tool.Tool
	skill registry.DiscoveredSkill
	dest  string
}

// containsToolName reports whether tools contains an entry with the given name.
func containsToolName(tools []tool.Tool, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

// dedupeJobsByDest keeps the first installJob for each unique destination
// path, dropping later duplicates. Several agents can resolve to the same
// destination dir in a given scope (notably the .agents/skills "universal"
// project-scope dir shared by codex, gemini, cursor and others); installing
// the same skill to the same path more than once is wasted work and, under
// the concurrent installer, a genuine race (overlapping os.Remove + os.Symlink
// on one path from multiple goroutines).
func dedupeJobsByDest(jobs []installJob) []installJob {
	if len(jobs) <= 1 {
		return jobs
	}
	seen := make(map[string]bool, len(jobs))
	out := jobs[:0:0]
	for _, j := range jobs {
		if seen[j.dest] {
			continue
		}
		seen[j.dest] = true
		out = append(out, j)
	}
	return out
}

// installSkillsConcurrently delegates all filesystem work to the shared
// installer.Placement engine while retaining the command's ordered result
// slice and progress output.
func installSkillsConcurrently(jobs []installJob, copyMode bool) []bool {
	results := make([]bool, len(jobs))
	if len(jobs) == 0 {
		return results
	}
	mode := installer.SymlinkMode
	fallback := installer.CopyOnSymlinkFailure
	conflict := installer.ReplaceOnConflict
	if copyMode {
		mode = installer.CopyMode
		fallback = installer.NoSymlinkFallback
	}
	placer := installer.NewPlacement(installer.PlacementOptions{
		Mode:          mode,
		Fallback:      fallback,
		Conflict:      conflict,
		RejectOverlap: true,
	})
	requests := make([]installer.PlacementRequest, len(jobs))
	for i, job := range jobs {
		requests[i] = installer.PlacementRequest{
			Source:      job.skill.Path,
			Destination: job.dest,
			Label:       job.skill.Name,
		}
	}
	for i, outcome := range placer.PlaceMany(requests, 8) {
		job := jobs[i]
		if outcome.Err != nil {
			var overlap *installer.SourceDestinationOverlapError
			if errors.As(outcome.Err, &overlap) {
				fmt.Fprintf(os.Stderr, "warning: skipping %s for %s: source overlaps destination\n", job.skill.Name, job.tool.Name)
			} else {
				fmt.Fprintf(os.Stderr, "warning: install %s for %s: %v\n", job.skill.Name, job.tool.Name, outcome.Err)
			}
			continue
		}
		if outcome.Result == nil || !outcome.Result.Applied {
			continue
		}
		if err := outcome.Result.Commit(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: install %s for %s: %v\n", job.skill.Name, job.tool.Name, err)
			continue
		}
		fmt.Printf("  ✓ %s → %s\n", job.skill.Name, job.dest)
		results[i] = true
	}
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
	// Case-insensitive matching, matching npx skills filterSkills.
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[strings.ToLower(n)] = true
	}
	var filtered []registry.DiscoveredSkill
	for _, s := range discovered {
		if nameSet[strings.ToLower(s.Name)] {
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
	if wellknown.IsSource(source) {
		meta = lockfile.SourceMeta{SourceType: "well-known", SourceURL: source}
	} else if sourceRoot != "" {
		meta = lockfile.ClassifySource(resolvedSource)
	} else {
		// local path source
		meta = lockfile.SourceMeta{SourceType: "local", SourceURL: ""}
	}

	entries := make(map[string]*lockfile.SkillEntry, len(skills))
	for _, s := range skills {
		skillPath := ""
		if sourceRoot != "" && !wellknown.IsSource(source) {
			if rel, err := filepath.Rel(sourceRoot, s.Path); err == nil {
				skillPath = filepath.ToSlash(filepath.Join(rel, "SKILL.md"))
			}
		}
		hash, err := lockfile.ComputeHash(s.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: hashing %s for lockfile: %v\n", s.Name, err)
			continue
		}
		entry := &lockfile.SkillEntry{
			Source:       resolvedSource,
			SourceType:   meta.SourceType,
			SourceURL:    meta.SourceURL,
			SkillPath:    skillPath,
			Ref:          installRef,
			ComputedHash: hash,
			PluginName:   s.PluginName,
		}
		// Record Eve subagent targets so --from-lock can reproduce them.
		// writeProjectLock is only called for project-scope installs.
		if len(installSubagents) > 0 {
			entry.Subagents = append([]string(nil), installSubagents...)
		}
		entries[s.Name] = entry
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

		// Temporarily set installRef and Eve subagent targets from the lock entry.
		firstEntry := lock.Skills[g.names[0]]
		savedRef := installRef
		if firstEntry.Ref != "" {
			installRef = firstEntry.Ref
		}
		savedSubagents := installSubagents
		if len(firstEntry.Subagents) > 0 {
			installSubagents = append([]string(nil), firstEntry.Subagents...)
		} else {
			installSubagents = nil
		}

		err := installSkillsToAgents(src, installAgents, g.names, installCopy, true, projectDir)
		installRef = savedRef
		installSubagents = savedSubagents
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
	return installer.CopySkill(src, dest)
}

// pathsOverlap reports whether one path contains (or equals) the other after
// resolving to absolute form. Used to guard against self-referential installs
// where the skill source is at or inside the install destination.
func pathsOverlap(a, b string) bool {
	return installer.PathsOverlap(a, b)
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
	installCmd.Flags().BoolVar(&installFromReg, "from-registry", false, "Deprecated alias: a bare skill name already selects Registry Install (no source clone)")
	installCmd.Flags().StringVar(&installCategory, "category", "", "Deprecated: names are globally unique; kept only for legacy diagnostics")
	installCmd.Flags().BoolVar(&installFromLock, "from-lock", false, "Restore project skills from skills-lock.json (reproducible install)")
	installCmd.Flags().StringArrayVar(&installSubagents, "subagent", nil, "Eve subagent name (repeatable; use 'root' or '.' for the root agent): install into agent/subagents/<name>/skills")

	rootCmd.AddCommand(installCmd)
}

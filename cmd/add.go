// cmd/add.go 实现 `sm add`：把技能注册到本地 Registry（不安装到 agent 目录）。
//
// 新契约（ADR 0007-0017）：
//   - 默认写入 global category（不再要求显式 category/special）；
//   - 写入前用 Register 原语校验 name/description（先验证再写）；
//   - 支持 --all（多 Skill 集合全部注册）、--force（跨来源同名替换）；
//   - Git 与本地目录使用相同发现规则（根 SKILL.md = 单 Skill；否则集合发现）；
//   - 本地单 SKILL.md 文件物化为标准目录；
//   - 任何 Git 形态都写 Origin（含抽取 Skill），ref kind 解析后记录；
//   - 本地目录/文件 = Snapshot（source_kind=local-snapshot）。
//
// Input: fmt, os, path/filepath, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/lockfile, github.com/woyin/skills-manager/internal/picker, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/internal/sourcecache, golang.org/x/term
// Output: var addCmd, func printSkillLint, func registerFromSource, func chooseSkillsFromGitSource
// Pos: 控制层-add命令实现（技能注册到本地 Registry）
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/lockfile"
	"github.com/woyin/skills-manager/internal/picker"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/sourcecache"
	"golang.org/x/term"
)

var (
	addFlags     = newSpecialFlags()
	addIsMCP     bool
	addList      bool
	addSkills    []string
	addAll       bool   // --all: 注册集合中全部技能
	addForce     bool   // --force: 跨来源同名替换
	addRef       string // --ref: 指定 git ref
	addCopy      bool
	addFullDepth bool
)

var addCmd = &cobra.Command{
	Use:     "add <source> [category]",
	Aliases: []string{"a"},
	Short:   "Register a skill or MCP into the local Registry (default: global category)",
	Long: `Register a skill or MCP into your personal, cross-project Registry (does not
install into agent dirs — use ` + "`sm install <name>`" + ` to deploy from the Registry).

The Registry is your user-owned library of skill originals. Registered skills
are reused by name across projects and Profiles, and refreshed by a single
` + "`sm update`" + `.

Source formats:
  owner/repo                                   GitHub shorthand
  https://github.com/owner/repo                Full GitHub URL
  https://github.com/owner/repo/tree/main/skills/name  Direct skill path
  https://gitlab.com/org/repo                  GitLab URL
  git@github.com:owner/repo.git                SSH git URL
  ./my-local-skills                            Local directory
  ./SKILL.md                                   Single skill file (materialized)

Defaults:
  • Category: global (all agents). Use --global/--codex/--claude for narrower targets.
  • Git source with a single root SKILL.md: registered directly.
  • Multi-skill source: TTY prompts to select; non-TTY requires --skill or --all.

A skill name is globally unique in the Registry (its frontmatter ` + "`name`" + `).
Re-registering the same name from the same source refreshes it; replacing a
name from a different source requires --force (all Link Installs are affected).`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		parsed := registry.ParseSource(lockfile.ResolveAlias(args[0]))
		source := parsed.Source()
		if parsed.SkillFilter != "" && len(addSkills) == 0 {
			addSkills = []string{parsed.SkillFilter}
		}
		if parsed.Ref != "" && addRef == "" {
			addRef = parsed.Ref
		}
		reg := registry.New(RegistryDir)

		// --list: 仅发现并列出技能，不写入注册表
		if addList {
			return listSkillsFromSource(source, addFullDepth)
		}

		// MCP 注册
		if addIsMCP {
			if err := reg.AddMCP(source); err != nil {
				return fmt.Errorf("adding MCP: %w", err)
			}
			fmt.Printf("✓ Added MCP %s\n", source)
			return nil
		}

		// category：显式位置参数 > special flag > global 默认。
		category := ""
		if len(args) > 1 {
			category = args[1]
		}
		if special := addFlags.Resolve(); special != "" {
			category = special
		}
		// --all 转为 skill 选择器 *。
		if addAll {
			addSkills = []string{"*"}
		}
		return registerFromSource(reg, source, category, addSkills, addForce, addRef, addFullDepth)
	},
}

// registerFromSource 是 `sm add` 的核心：发现 → 选择 → 注册（写入前验证）。
// Git 与本地目录使用相同发现规则；Git 形态写 Origin（含抽取 Skill）。
func registerFromSource(reg *registry.Registry, source, category string, skillNames []string, force bool, ref string, fullDepth bool) error {
	isGit := registry.IsGitURL(source)

	// ── 发现 + 选择 ──
	// 单文件来源（SKILL.md）：直接注册，无需发现。
	if isSingleSkillMDFile(source) {
		origin := registry.SkillOrigin{
			SourceKind: registry.SourceLocalSnapshot,
			Source:     source,
			SubPath:    ".",
		}
		res, err := reg.Register(source, category, origin, force)
		if err != nil {
			return fmt.Errorf("registering skill: %w", err)
		}
		printRegisterResult(source, res, force)
		return nil
	}

	// 本地目录来源。
	if !isGit {
		return registerFromLocalDir(reg, source, category, skillNames, force, fullDepth)
	}

	// Git 来源：克隆到 source cache（持久），发现，选择，注册（写 Origin）。
	return registerFromGit(reg, source, category, skillNames, force, ref, fullDepth)
}

// registerFromLocalDir 处理本地目录来源：根 SKILL.md = 单 Skill；否则集合发现。
func registerFromLocalDir(reg *registry.Registry, source, category string, skillNames []string, force bool, fullDepth bool) error {
	// 根 SKILL.md：整个目录是一个 Skill。
	if _, err := os.Stat(filepath.Join(source, "SKILL.md")); err == nil {
		origin := registry.SkillOrigin{
			SourceKind: registry.SourceLocalSnapshot,
			Source:     source,
			SubPath:    ".",
		}
		res, err := reg.Register(source, category, origin, force)
		if err != nil {
			return fmt.Errorf("registering skill: %w", err)
		}
		printRegisterResult(source, res, force)
		return nil
	}

	// 集合发现。
	discovered, err := registry.DiscoverSkillsWithOptions(source, registry.DiscoverOptions{
		FullDepth:     fullDepth,
		AutoFullDepth: true,
	})
	if err != nil {
		return fmt.Errorf("discovering skills: %w", err)
	}
	if len(discovered) == 0 {
		return fmt.Errorf("no SKILL.md found in %s", source)
	}

	selected, err := selectSkillsToRegister(source, discovered, skillNames)
	if err != nil {
		return err
	}

	for _, s := range selected {
		origin := registry.SkillOrigin{
			SourceKind: registry.SourceLocalSnapshot,
			Source:     source,
			SubPath:    relOrDot(source, s.Path),
		}
		res, err := reg.Register(s.Path, category, origin, force)
		if err != nil {
			return fmt.Errorf("registering skill %q: %w", s.Name, err)
		}
		printRegisterResult(source, res, force)
		// lint warnings (non-blocking)
		if rel := skillRelForLint(res.Path); rel != "" {
			printSkillLint(reg, []string{rel})
		}
	}
	return nil
}

// registerFromGit 处理 Git 来源：克隆到 source cache，发现，选择，注册并写 Origin。
func registerFromGit(reg *registry.Registry, source, category string, skillNames []string, force bool, ref string, fullDepth bool) error {
	// 克隆到持久 source cache（与 install/update 共享），便于后续 update 复用。
	acquired, err := sourcecache.New(DataDir).Acquire(source, ref, false)
	if err != nil {
		return fmt.Errorf("acquiring source: %w", err)
	}
	cloneRoot := acquired.Path

	// 解析 ref kind（查询克隆仓库以区分 branch/tag/commit）。
	normalizedRef, refKind, rerr := registry.ResolveRefKind(ref, cloneRoot)
	if rerr != nil {
		return fmt.Errorf("resolving ref: %w", rerr)
	}
	commit := acquired.Metadata.Commit

	// tree URL 子路径：直接注册该子目录（单 Skill）。
	_, _, subPath, _ := registry.ParseTreeURL(source)
	subPath = registry.SanitizeSubpath(subPath)
	if subPath != "" {
		skillDir := filepath.Join(cloneRoot, subPath)
		if _, err := os.Stat(skillDir); err != nil {
			return fmt.Errorf("path %q not found in repository: %w", subPath, err)
		}
		if _, err := registerSingleGitSkill(reg, source, category, skillDir, subPath, normalizedRef, refKind, commit, force); err != nil {
			return err
		}
		return nil
	}

	// 根 SKILL.md：整个仓库是一个 Skill。
	if _, err := os.Stat(filepath.Join(cloneRoot, "SKILL.md")); err == nil {
		_, err := registerSingleGitSkill(reg, source, category, cloneRoot, ".", normalizedRef, refKind, commit, force)
		return err
	}

	// 集合发现 + 选择。
	discovered, err := registry.DiscoverSkillsWithOptions(cloneRoot, registry.DiscoverOptions{
		FullDepth:     fullDepth,
		AutoFullDepth: true,
	})
	if err != nil {
		return fmt.Errorf("discovering skills: %w", err)
	}
	if len(discovered) == 0 {
		return fmt.Errorf("no skills found in %s", source)
	}

	selected, err := selectSkillsToRegister(source, discovered, skillNames)
	if err != nil {
		return err
	}

	for _, s := range selected {
		res, err := registerSingleGitSkill(reg, source, category, s.Path, relOrDot(cloneRoot, s.Path), normalizedRef, refKind, commit, force)
		if err != nil {
			return err
		}
		if rel := skillRelForLint(res.Path); rel != "" {
			printSkillLint(reg, []string{rel})
		}
	}
	return nil
}

// registerSingleGitSkill 以 git origin 注册单个技能目录并打印结果，
// 返回注册结果（供调用方取 registry 内路径做后续检查）。
func registerSingleGitSkill(reg *registry.Registry, source, category, skillDir, subPath, normalizedRef string, refKind registry.RefKind, commit string, force bool) (*registry.RegisteredSkill, error) {
	origin := registry.SkillOrigin{
		SourceKind: registry.SourceGit,
		Source:     source,
		Ref:        normalizedRef,
		RefKind:    refKind,
		SubPath:    subPath,
		Commit:     commit,
	}
	res, err := reg.Register(skillDir, category, origin, force)
	if err != nil {
		return nil, fmt.Errorf("registering skill: %w", err)
	}
	printRegisterResult(source, res, force)
	return res, nil
}

// selectSkillsToRegister 按 skillNames / TTY / 非交互规则选出要注册的技能。
//   - 显式 skillNames（含 *）：按 filterSkills 匹配；
//   - 单 skill：直接选；
//   - 多 skill：TTY 交互多选；非 TTY 失败（零写入），要求 --skill 或 --all。
func selectSkillsToRegister(source string, discovered []registry.DiscoveredSkill, skillNames []string) ([]registry.DiscoveredSkill, error) {
	if len(skillNames) > 0 {
		filtered := filterSkills(discovered, skillNames)
		if len(filtered) == 0 {
			return nil, fmt.Errorf("no matching skills found in %s", source)
		}
		return filtered, nil
	}
	if len(discovered) == 1 {
		return discovered, nil
	}
	// 多 skill：TTY 交互；非 TTY 失败（零写入）。
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stderr, "Multiple skills found in %s. Use --skill <name> or --all to select:\n", source)
		for _, s := range discovered {
			if s.Description != "" {
				fmt.Fprintf(os.Stderr, "  - %s: %s\n", s.Name, s.Description)
			} else {
				fmt.Fprintf(os.Stderr, "  - %s\n", s.Name)
			}
		}
		return nil, fmt.Errorf("no skill selected (non-interactive terminal); rerun with --skill <name> or --all")
	}
	items := make([]picker.Item, len(discovered))
	for i, s := range discovered {
		items[i] = picker.Item{Label: s.Name, Detail: truncate(s.Description, 60), Value: s.Name}
	}
	chosen, err := picker.PickMultiple("Select skills to register", items)
	if err != nil {
		return nil, err
	}
	if len(chosen) == 0 {
		return nil, fmt.Errorf("no skills selected")
	}
	return filterSkills(discovered, chosen), nil
}

// printRegisterResult 打印单次注册结果与后续提示。
func printRegisterResult(source string, res *registry.RegisteredSkill, force bool) {
	switch res.Outcome {
	case registry.OutcomeCreated:
		fmt.Printf("✓ Registered %q (from %s) → %s\n", res.Name, source, res.Path)
	case registry.OutcomeRefreshed:
		fmt.Printf("✓ Refreshed %q (re-registered from %s)\n", res.Name, source)
	case registry.OutcomeReplaced:
		fmt.Printf("✓ Replaced %q from a different source (%s); all Link Installs now see the new original\n", res.Name, source)
	}
	fmt.Println("  Run `sm list` to see the Registry.")
	fmt.Println("  Run `sm install <name>` to deploy into a project.")
}

// printSkillLint 对刚入库的技能做 frontmatter lint，问题以非阻塞警告
// 形式输出到 stderr。lint 不影响 add 的退出码（始终 exit 0）。
func printSkillLint(reg *registry.Registry, added []string) {
	hasWarnings := false
	for _, rel := range added {
		res := reg.LintSkill(rel)
		if len(res.Findings) == 0 {
			continue
		}
		hasWarnings = true
		fmt.Fprintf(os.Stderr, "\n⚠ Lint warnings for %s:\n", rel)
		for _, line := range res.FormatLintFindings() {
			fmt.Fprintln(os.Stderr, line)
		}
	}
	if hasWarnings {
		fmt.Fprintf(os.Stderr, "\nThese issues do not block registration but may prevent the skill from being triggered by agents.\n")
	}
}

// relOrDot 返回 path 相对于 root 的路径；失败返回 "."。
func relOrDot(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return rel
	}
	return "."
}

// isSingleSkillMDFile 判断 source 是否为单个 SKILL.md 文件（非目录）。
func isSingleSkillMDFile(source string) bool {
	info, err := os.Stat(source)
	if err != nil || info.IsDir() {
		return false
	}
	return filepath.Base(source) == "SKILL.md"
}

func init() {
	addFlags.Bind(addCmd, "Register into")
	addCmd.Flags().BoolVar(&addIsMCP, "mcp", false, "Register as MCP server definition")
	addCmd.Flags().BoolVarP(&addList, "list", "l", false, "List available skills in source without registering")
	addCmd.Flags().StringArrayVarP(&addSkills, "skill", "s", nil, "Register specific skills by name (use '*' or --all for all)")
	addCmd.Flags().BoolVar(&addAll, "all", false, "Register all skills discovered in a multi-skill source")
	addCmd.Flags().BoolVar(&addForce, "force", false, "Force replace when the skill name is registered from a different source")
	addCmd.Flags().StringVar(&addRef, "ref", "", "Git branch, tag, or commit to register from (resolved and recorded)")
	addCmd.Flags().BoolVar(&addCopy, "copy", false, "Compatibility alias (registration always copies originals into the Registry)")
	addCmd.Flags().BoolVar(&addFullDepth, "full-depth", false, "Also discover SKILL.md outside standard skill dirs (e.g. examples/, tests/)")

	rootCmd.AddCommand(addCmd)
}

// chooseSkillsFromGitSource 保留供既有测试引用；新路径由 registerFromGit 统一处理。
// 等价于 registerFromGit 的发现+选择阶段，但不写入。
func chooseSkillsFromGitSource(source string) ([]string, error) {
	cloneDest, tempDir, err := registry.CloneToTemp(source, "sm-add-discover-*")
	if err != nil {
		return nil, err
	}
	defer registry.RemoveCloneTemp(tempDir)

	if _, err := os.Stat(filepath.Join(cloneDest, "SKILL.md")); err == nil {
		return nil, nil
	}
	discovered, err := registry.DiscoverSkillsWithOptions(cloneDest, registry.DiscoverOptions{FullDepth: addFullDepth, AutoFullDepth: true})
	if err != nil {
		return nil, fmt.Errorf("discovering skills: %w", err)
	}
	if len(discovered) == 0 {
		return nil, nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stderr, "Multiple skills found in %s. Use -s <name> or --all to select:\n", source)
		for _, s := range discovered {
			if s.Description != "" {
				fmt.Fprintf(os.Stderr, "  - %s: %s\n", s.Name, s.Description)
			} else {
				fmt.Fprintf(os.Stderr, "  - %s\n", s.Name)
			}
		}
		return nil, fmt.Errorf("no skill selected (non-interactive terminal); rerun with -s <name> or --all")
	}
	if len(discovered) == 1 {
		return []string{discovered[0].Name}, nil
	}
	items := make([]picker.Item, len(discovered))
	for i, s := range discovered {
		items[i] = picker.Item{Label: s.Name, Detail: truncate(s.Description, 60), Value: s.Name}
	}
	chosen, err := picker.PickMultiple("Select skills to register", items)
	if err != nil {
		return nil, err
	}
	if len(chosen) == 0 {
		return nil, fmt.Errorf("no skills selected")
	}
	return chosen, nil
}

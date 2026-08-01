// cmd/rm.go 实现 `sm rm`：删除 Registry 原件（ADR 0017）。默认拒绝在仍有
// 已知项目/全局引用时删除并列出引用；--force 先清理所有已知 Link Installs 与
// lock entries 再删除原件。仅删 Installed Skill 请用 `sm uninstall`。
//
// Input: fmt, os, path/filepath, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/home, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/internal/symlink, github.com/woyin/skills-manager/internal/tool
// Output: var rmCmd, func removeMCP, func removeAll, func removeFromAgents, func removeSkill, func countReferencesTo, func rmScanDirs
// Pos: 控制层-rm命令实现
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/lockfile"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
)

var (
	rmFlags   = newSpecialFlags()
	rmIsMCP   bool
	rmAll     bool
	rmAgents  []string
	rmSkills  []string
	rmYes     bool
	rmProject bool   // --project: 仅项目级（默认项目+全局）
	rmDir     string // --dir: 项目根
	rmForce   bool   // --force: 强制删除 Registry 原件，先清理所有已知引用
)

var rmCmd = &cobra.Command{
	Use:     "rm <name> [category]",
	Aliases: []string{"remove", "r"},
	Short:   "Uninstall a skill and remove registry original if unused",
	Long: `Uninstall a skill from agent skill dirs and remove the registry original
when nothing else references it.

Remove a skill original from the Registry (ADR 0017). Refuses while any known
project or global installation references the original, and lists those
references. Use --force to remove all known installs and lock entries first,
then delete the original. Inaccessible historical projects are reported.

To remove only the Installed Skill (keeping the Registry original), use
` + "`sm uninstall <name>`" + ` instead.

Examples:
  sm rm my-skill
  sm rm my-skill --force
  sm rm --agent claude my-skill
  sm rm --all
  sm rm cloudflare --mcp
`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if rmIsMCP {
			if len(args) == 0 {
				return fmt.Errorf("MCP name required")
			}
			return removeMCP(args[0])
		}

		if rmAll {
			return removeAll()
		}

		// --agent 模式：只清 agent 目录链接（兼容旧行为）
		if len(rmAgents) > 0 {
			return removeFromAgents(args)
		}

		if len(args) == 0 {
			return fmt.Errorf("skill name required")
		}

		name := args[0]
		return removeSkill(name, args)
	},
}

func removeMCP(name string) error {
	reg := registry.New(RegistryDir)
	if err := reg.RemoveMCP(name); err != nil {
		return fmt.Errorf("removing MCP: %w", err)
	}
	fmt.Printf("✓ Removed MCP %q\n", name)
	return nil
}

func removeAll() error {
	reg := registry.New(RegistryDir)
	skills, err := reg.ListSkills()
	if err != nil {
		return fmt.Errorf("listing skills: %w", err)
	}

	if !rmYes {
		count := 0
		for _, names := range skills {
			count += len(names)
		}
		fmt.Printf("This will remove %d skill(s) and all their symlinks. Continue? [y/N]: ", count)
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	removed := 0
	for category, names := range skills {
		for _, name := range names {
			skillPath, _ := reg.GetSkillPath(name, category, "")
			if skillPath != "" {
				for _, t := range tool.AllTools() {
					for _, dir := range rmScanDirs(t) {
						links, _ := symlink.FindPointingTo(dir, skillPath)
						for _, link := range links {
							os.Remove(link)
						}
						// also remove same-name entries (copy installs)
						os.RemoveAll(filepath.Join(dir, name))
					}
				}
			}
			reg.RemoveSkill(name, category, "")
			removeFromProjectLock(name)
			removed++
		}
	}

	fmt.Printf("✓ Removed %d skill(s)\n", removed)
	return nil
}

func removeFromAgents(args []string) error {
	targetTools := tool.ToolsByNames(rmAgents)
	if len(targetTools) == 0 {
		return fmt.Errorf("no matching agents found for: %v", rmAgents)
	}

	skillsToRemove := rmSkills
	if len(args) > 0 && len(skillsToRemove) == 0 {
		skillsToRemove = args
	}

	removed := 0

	for _, t := range targetTools {
		for _, agentDir := range rmScanDirs(t) {
			entries, err := os.ReadDir(agentDir)
			if err != nil {
				continue
			}

			for _, entry := range entries {
				name := entry.Name()

				if len(skillsToRemove) > 0 {
					match := false
					for _, s := range skillsToRemove {
						if s == "*" || s == name {
							match = true
							break
						}
					}
					if !match {
						continue
					}
				}

				linkPath := filepath.Join(agentDir, name)
				if err := os.RemoveAll(linkPath); err == nil {
					fmt.Printf("  ✓ Removed %s from %s\n", name, t.Name)
					removed++
				}
			}
		}
	}

	fmt.Printf("\n✓ Removed %d skill(s) from %d agent(s)\n", removed, len(targetTools))
	return nil
}

// removeSkill 卸装并在无引用时删 registry 原件。
func removeSkill(name string, args []string) error {
	// ADR 0017: sm rm = 删除 Registry 原件。先检查全局引用（DB 枚举所有已知项目），
	// 有引用则拒绝并列出（除非 --force）。--force 清所有已知链接与 lock entries。
	// 保留旧 --agent / --project 路径用于兼容"仅卸装"语义。
	if !rmForce && len(args) == 1 {
		return removeRegistryOriginal(name)
	}
	if rmForce {
		return removeRegistryOriginal(name)
	}
	return removeSkillLegacy(name, args)
}

// removeRegistryOriginal 实现 ADR 0017 的 `sm rm <name>`：删除 Registry 原件。
//   - 解析全局唯一 name（不存在 → 报错）；
//   - 枚举所有已知 project/global 安装引用（DB + 文件系统扫描）；
//   - 有引用且未 --force → 拒绝并列出引用路径；
//   - --force → 先清所有可访问链接与 lock entries，报告不可访问项目，再删原件。
func removeRegistryOriginal(name string) error {
	reg := registry.New(RegistryDir)
	res, err := reg.ResolveUniqueSkill(name)
	if err != nil {
		// 不在 registry：清理任何残留安装（兼容旧行为：仅卸装无原件的技能）。
		if _, ok := err.(*registry.NameNotFoundError); ok {
			removed := cleanupInstallsByName(name)
			if removed == 0 {
				return fmt.Errorf("skill %q is not in the registry and no installs found", name)
			}
			fmt.Printf("✓ Cleaned %d install(s) of %q (no registry original)\n", removed, name)
			return nil
		}
		return err
	}

	// 枚举引用：DB 已知项目 + 全局 agent 目录。
	refs := listRegistryReferences(name, res.Path)

	if len(refs) > 0 && !rmForce {
		fmt.Fprintf(os.Stderr, "Cannot remove %q: %d known install(s) still reference it:\n", name, len(refs))
		for _, r := range refs {
			fmt.Fprintf(os.Stderr, "  - %s\n", r)
		}
		fmt.Fprintf(os.Stderr, "Run `sm rm %s --force` to remove all known installs and the registry original.\n", name)
		return fmt.Errorf("skill %q is still referenced by %d install(s)", name, len(refs))
	}

	// --force 或无引用：清理所有可访问链接 + lock entries。
	if rmForce && len(refs) > 0 {
		fmt.Printf("Force-removing %q: cleaning %d known install(s) first...\n", name, len(refs))
	}
	removed := cleanupInstallsByName(name)
	if removed > 0 {
		fmt.Printf("  Cleaned %d install location(s).\n", removed)
	}
	if rmForce {
		reportInaccessibleProjects(name)
	}

	// 删除 Registry 原件。
	if err := reg.RemoveSkill(name, res.Category, ""); err != nil {
		return fmt.Errorf("removing registry original: %w", err)
	}
	fmt.Printf("✓ Removed registry original %q (category: %s)\n", name, res.Category)
	return nil
}

// reportInaccessibleProjects 在 --force 删除前报告数据库中已知但当前不可访问的
// 历史项目（ADR 0017）：这些项目里可能残留指向该原件的安装，sm 无法访问确认。
func reportInaccessibleProjects(skillName string) {
	if db, err := openDB(); err == nil {
		defer db.Close()
		if projects, perr := db.GetAllProjects(); perr == nil {
			var inaccessible []string
			for _, p := range projects {
				if _, statErr := os.Stat(p.Path); statErr != nil {
					inaccessible = append(inaccessible, p.Path)
				}
			}
			if len(inaccessible) > 0 {
				fmt.Fprintf(os.Stderr, "⚠ %d historical project(s) are inaccessible; installs of %q there cannot be verified/cleaned:\n", len(inaccessible), skillName)
				for _, path := range inaccessible {
					fmt.Fprintf(os.Stderr, "  - %s\n", path)
				}
				fmt.Fprintln(os.Stderr, "  If those projects return, run `sm uninstall --project --dir <path>` to remove stale installs.")
			}
		}
	}
}

// listRegistryReferences 枚举所有已知引用 skillName/skillPath 的安装位置。
// 覆盖：DB 记录的所有已知项目 + 全局 agent 目录 + 当前项目。
func listRegistryReferences(skillName, skillPath string) []string {
	seen := map[string]bool{}
	var refs []string

	addRef := func(path string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		refs = append(refs, path)
	}

	// 文件系统扫描：全局 agent 目录 + 当前项目。
	for _, t := range tool.AllTools() {
		// 全局
		gDir := filepath.Join(home.Dir(), t.SkillDir)
		link := filepath.Join(gDir, skillName)
		if _, err := os.Lstat(link); err == nil {
			addRef(link)
		}
		if skillPath != "" {
			if links, _ := symlink.FindPointingTo(gDir, skillPath); len(links) > 0 {
				for _, l := range links {
					addRef(l)
				}
			}
		}
	}
	// 当前项目
	pd := rmProjectDir()
	for _, t := range tool.AllTools() {
		if d := tool.GetProjectSkillDir(t, pd); d != "" {
			link := filepath.Join(d, skillName)
			if _, err := os.Lstat(link); err == nil {
				addRef(link)
			}
		}
	}

	// DB 枚举：所有已知项目（报告历史项目，即使当前不可访问）。
	if db, err := openDB(); err == nil {
		defer db.Close()
		if projects, perr := db.GetAllProjects(); perr == nil {
			for _, p := range projects {
				for _, t := range tool.AllTools() {
					if d := tool.GetProjectSkillDir(t, p.Path); d != "" {
						link := filepath.Join(d, skillName)
						if _, err := os.Lstat(link); err == nil {
							addRef(link)
						}
					}
				}
			}
		}
	}
	return refs
}

// cleanupInstallsByName 删除所有可访问项目/全局中 skillName 的安装条目，
// 并清理对应 lock entries。返回清理的数量。
func cleanupInstallsByName(skillName string) int {
	removed := 0
	// 全局 + 当前项目 agent 目录。
	projectsToClean := map[string]bool{rmProjectDir(): true}
	// 加入 DB 已知项目。
	if db, err := openDB(); err == nil {
		if projects, perr := db.GetAllProjects(); perr == nil {
			for _, p := range projects {
				projectsToClean[p.Path] = true
			}
		}
		db.Close()
	}
	for projectPath := range projectsToClean {
		for _, t := range tool.AllTools() {
			// 全局
			if projectPath == rmProjectDir() {
				gLink := filepath.Join(home.Dir(), t.SkillDir, skillName)
				if _, err := os.Lstat(gLink); err == nil {
					if rerr := os.RemoveAll(gLink); rerr == nil {
						removed++
					}
				}
			}
			if d := tool.GetProjectSkillDir(t, projectPath); d != "" {
				link := filepath.Join(d, skillName)
				if _, err := os.Lstat(link); err == nil {
					if rerr := os.RemoveAll(link); rerr == nil {
						removed++
					}
				}
			}
		}
		// 清 lock entry（仅对存在的 lockfile）。
		lm := lockfile.NewManager(projectPath)
		if lm.Exists() {
			_ = lm.Remove(skillName)
		}
	}
	// Eve 子代理目录（agent/subagents/<name>/skills）。
	for _, dir := range eveSubagentSkillDirsFor(rmProjectDir()) {
		link := filepath.Join(dir, skillName)
		if _, err := os.Lstat(link); err == nil {
			if rerr := os.RemoveAll(link); rerr == nil {
				removed++
			}
		}
	}
	return removed
}

// removeSkillLegacy 保留旧"卸装+条件删除"行为，供 --agent/--project 兼容路径。
func removeSkillLegacy(name string, args []string) error {
	reg := registry.New(RegistryDir)
	special := rmFlags.Resolve()

	category := ""
	if len(args) > 1 {
		category = args[1]
	}

	skillPath, _ := reg.GetSkillPath(name, category, special)

	// 1) 卸装：清默认范围内 agent 目录中的同名条目
	uninstalled := 0
	for _, t := range tool.AllTools() {
		for _, dir := range rmScanDirs(t) {
			linkPath := filepath.Join(dir, name)
			if _, err := os.Lstat(linkPath); err != nil {
				continue
			}
			if err := os.RemoveAll(linkPath); err == nil {
				fmt.Printf("  Uninstalled: %s\n", linkPath)
				uninstalled++
			}
		}
		// 也清指向 registry 原件的其它名字链接（极少见）
		if skillPath != "" {
			for _, dir := range rmScanDirs(t) {
				links, _ := symlink.FindPointingTo(dir, skillPath)
				for _, link := range links {
					os.Remove(link)
					fmt.Printf("  Removed symlink: %s\n", link)
					uninstalled++
				}
			}
		}
	}

	// Eve 子代理目录（agent/subagents/<name>/skills）不在 tool.AllTools() 的
	// 扫描范围内，单独清理，对齐 npx skills remove 对 Eve 子代理的覆盖。
	for _, dir := range eveSubagentSkillDirs() {
		linkPath := filepath.Join(dir, name)
		if _, err := os.Lstat(linkPath); err != nil {
			continue
		}
		if skillPath != "" {
			links, _ := symlink.FindPointingTo(dir, skillPath)
			for _, link := range links {
				os.Remove(link)
				fmt.Printf("  Removed symlink: %s\n", link)
				uninstalled++
			}
		}
		if err := os.RemoveAll(linkPath); err == nil {
			fmt.Printf("  Uninstalled: %s\n", linkPath)
			uninstalled++
		}
	}

	// 卸装成功后，同步移除 skills-lock.json 条目（保持锁文件一致）。
	if uninstalled > 0 {
		removeFromProjectLock(name)
	}

	// 2) 若 registry 中还有其它 agent 引用该原件，则保留 registry
	if skillPath != "" {
		if remaining := countReferencesTo(skillPath, name); remaining > 0 {
			fmt.Printf("✓ Uninstalled skill %q (%d agent link(s) remain elsewhere; registry kept)\n", name, remaining)
			return nil
		}
		if err := reg.RemoveSkill(name, category, special); err != nil {
			// 已卸装但 registry 删除失败：报告但不回滚卸装
			return fmt.Errorf("uninstalled from agents, but removing registry original failed: %w", err)
		}
		fmt.Printf("✓ Removed skill %q (uninstalled %d, registry original deleted)\n", name, uninstalled)
		return nil
	}

	// 无 registry 原件：仅卸装
	if uninstalled == 0 {
		return fmt.Errorf("skill %q not found in agent dirs or registry", name)
	}
	fmt.Printf("✓ Uninstalled skill %q (%d location(s); no registry original)\n", name, uninstalled)
	return nil
}

// countReferencesTo 统计仍指向 skillPath 或同名的 agent 安装条目数（全工具、全局+项目）。
func countReferencesTo(skillPath, name string) int {
	count := 0
	for _, t := range tool.AllTools() {
		dirs := []string{filepath.Join(home.Dir(), t.SkillDir)}
		if pd := tool.GetProjectSkillDir(t, rmProjectDir()); pd != "" {
			dirs = append(dirs, pd)
		}
		for _, dir := range dirs {
			linkPath := filepath.Join(dir, name)
			if _, err := os.Lstat(linkPath); err == nil {
				count++
				continue
			}
			if skillPath != "" {
				links, _ := symlink.FindPointingTo(dir, skillPath)
				count += len(links)
			}
		}
	}
	return count
}

// eveSubagentSkillDirs discovers Eve subagent skill directories under
// <projectDir>/agent/subagents/*/skills and returns their absolute paths.
// npx skills remove scans these directories too; sm install --subagent writes
// into them, so rm must clean them to avoid leaving stale installs behind.
func eveSubagentSkillDirs() []string {
	projectDir := rmProjectDir()
	return eveSubagentSkillDirsFor(projectDir)
}

// eveSubagentSkillDirsFor returns Eve subagent skill dirs under projectDir.
func eveSubagentSkillDirsFor(projectDir string) []string {
	if projectDir == "" {
		return nil
	}
	subagentsRoot := filepath.Join(projectDir, "agent", "subagents")
	entries, err := os.ReadDir(subagentsRoot)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sd := filepath.Join(subagentsRoot, e.Name(), "skills")
		if info, err := os.Stat(sd); err == nil && info.IsDir() {
			dirs = append(dirs, sd)
		}
	}
	return dirs
}

// rmScanDirs 返回应清理的技能目录：默认项目+全局；--project 仅项目。
// specialFlags 的 --global 表示 registry 分类，不收窄扫目录。
func rmScanDirs(t tool.Tool) []string {
	var dirs []string
	if !rmProject {
		dirs = append(dirs, filepath.Join(home.Dir(), t.SkillDir))
	}
	if pd := tool.GetProjectSkillDir(t, rmProjectDir()); pd != "" {
		dirs = append(dirs, pd)
	}
	return dirs
}

func rmProjectDir() string {
	if rmDir != "" {
		return rmDir
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

// removeFromProjectLock deletes a skill entry from the project's
// skills-lock.json (if present), keeping the lockfile in sync after an
// uninstall. Mirrors npx skills removeSkillFromLocalLock.
func removeFromProjectLock(name string) {
	dir := rmProjectDir()
	if dir == "" {
		return
	}
	lm := lockfile.NewManager(dir)
	if !lm.Exists() {
		return
	}
	if err := lm.Remove(name); err != nil {
		fmt.Fprintf(os.Stderr, "warning: removing %q from skills-lock.json: %v\n", name, err)
	}
}

func init() {
	rmFlags.Bind(rmCmd, "Remove from")
	rmCmd.Flags().BoolVar(&rmIsMCP, "mcp", false, "Remove MCP server definition")
	rmCmd.Flags().BoolVar(&rmAll, "all", false, "Shorthand for --skill '*' --agent '*' -y")
	rmCmd.Flags().StringArrayVarP(&rmAgents, "agent", "a", nil, "Remove from specific agents (use '*' for all)")
	rmCmd.Flags().StringArrayVarP(&rmSkills, "skill", "s", nil, "Specify skills to remove (use '*' for all)")
	rmCmd.Flags().BoolVarP(&rmYes, "yes", "y", false, "Skip confirmation prompts")
	rmCmd.Flags().BoolVar(&rmProject, "project", false,
		"Only clean project-level installs (./<agent>/skills)")
	rmCmd.Flags().StringVar(&rmDir, "dir", "", "Project root (default: current dir)")
	rmCmd.Flags().BoolVar(&rmForce, "force", false, "Force-remove the Registry original: clean all known Link Installs and lock entries first")

	rootCmd.AddCommand(rmCmd)
}

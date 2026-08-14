// cmd/update.go 实现 `sm update`：默认刷新整个个人 Registry 中所有可更新原件
// （ADR 0008/0013/0014）；支持按技能名、--project/--global 安装范围过滤，以及
// --in-place 就地刷新 Copy Install。按 Source 隔离，任一失败退出码非零。
//
// Input: fmt, os, os/exec, path/filepath, runtime, strings, sync, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/home, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/internal/symlink, github.com/woyin/skills-manager/internal/tool, github.com/woyin/skills-manager/internal/updater
// Output: var updateCmd, type installedUpdateTargets, type originSkillTarget, type updateSummary, type pullResult, type namedRepo, func updateAllSkills, func updateCollectedTargets, func pullReposConcurrently, func updateGitRepos
// Pos: 控制层-update命令实现
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/concurrency"
	"github.com/woyin/skills-manager/internal/lockfile"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
	"github.com/woyin/skills-manager/internal/updater"
)

var (
	updateGlobal   bool
	updateProject  bool
	updateYes      bool
	updateDir      string // --dir: 项目根（默认 cwd）；扫已安装
	updateRegistry bool   // --registry: 更新整个 registry（旧默认）
	updateInPlace  bool   // --in-place: 就地刷新项目内 Copy Install 实体，不动 registry
)

var updateCmd = &cobra.Command{
	Use:     "update [skills...]",
	Aliases: []string{"up", "upgrade"},
	Short:   "Update Registry originals (default) or installed-scope skills",
	Long: `Refresh every updatable original in the personal Registry by default, so all
Link Installs across projects observe the refreshed content without visiting each
project (ADR 0008).

Scope:
  sm update                         entire Registry (default)
  sm update foo bar                 named Registry Skills only
  sm update --project [--dir PATH]  Registry Skills referenced by that project
  sm update --global                Registry Skills referenced by global Agent installs
  sm update --project --global      the union of both scopes

Tracking Git Skills (default branch or named branch) are updated; pinned tag/commit
Skills and local Snapshot Skills are healthy skips; Orphan Skills (damaged provenance)
are errors. Sources update independently — a failed source keeps its prior valid
originals while other sources continue, and any failure makes the exit nonzero.

  sm update --in-place              refresh project Copy Install entities from their
                                   own origin in place (no Registry change; Link
                                   Installs are no-ops; missing cache → run sm update)

The former --registry flag is a deprecated alias for the bare default.
Non-interactive: sm update -y
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// --registry 现为弃用 alias（默认即整个 Registry）。
		if updateRegistry {
			fmt.Fprintln(cmd.ErrOrStderr(), "warning: --registry is deprecated; the default now refreshes the entire Registry")
		}
		// --in-place：就地刷新项目内 Copy Install 实体（独立路径，不走 registry 刷新）。
		if updateInPlace {
			dir := updateDir
			if dir == "" {
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("getting working directory: %w", err)
				}
				dir = wd
			}
			return updateInPlaceInstalls(dir)
		}
		// 默认 + --registry alias + 无 scope flags：刷新整个 Registry。
		// named skills：按名过滤整个 Registry。
		// --project/--global：按已安装 scope 过滤。
		if updateProject || updateGlobal {
			dir := updateDir
			if dir == "" {
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("getting working directory: %w", err)
				}
				dir = wd
			}
			if len(args) > 0 {
				return updateInstalledNamed(dir, args)
			}
			return updateInstalledSources(dir)
		}
		if len(args) > 0 {
			return updateSpecificSkills(args)
		}
		return updateAllSkills()
	},
}

// 更新注册表中所有 git 管理的条目（并发 git pull）。
// --global/--project 可限定范围。

func updateAllSkills() error {
	reg := registry.New(RegistryDir)
	originals, err := reg.ListAllOriginals()
	if err != nil {
		return fmt.Errorf("listing registry originals: %w", err)
	}
	var summary updateSummary
	type srcKey struct{ source, ref string }
	groups := map[srcKey][]registry.RegistryOriginal{}
	for _, o := range originals {
		switch o.Class {
		case registry.ClassPinned:
			fmt.Printf("SKIPPED: %s (pinned)\n", o.Name)
			summary.Skipped++
		case registry.ClassSnapshot:
			fmt.Printf("SKIPPED: %s (snapshot)\n", o.Name)
			summary.Skipped++
		case registry.ClassOrphan:
			fmt.Fprintf(os.Stderr, "ERROR: %s (orphan: no valid provenance)\n", o.Name)
			summary.Errors++
		case registry.ClassTracking:
			k := srcKey{o.Origin.Source, o.Origin.Ref}
			groups[k] = append(groups[k], o)
		}
	}
	for k, group := range groups {
		_ = k
		u, s, e := refreshRegistryGroup(group)
		summary.Updated += u
		summary.Skipped += s
		summary.Errors += e
	}
	fmt.Printf("\nSummary: %d updated, %d pinned/skipped, %d errors\n", summary.Updated, summary.Skipped, summary.Errors)
	if summary.Errors > 0 {
		return fmt.Errorf("%d error(s) during registry update", summary.Errors)
	}
	return nil
}

// refreshRegistryGroup 刷新一组同 Source 的 tracking originals。
func refreshRegistryGroup(group []registry.RegistryOriginal) (updated, skipped, errors int) {
	if len(group) == 0 {
		return 0, 0, 0
	}
	first := group[0]
	targets := make([]originSkillTarget, 0, len(group))
	for _, o := range group {
		ro := o.Origin
		targets = append(targets, originSkillTarget{
			skillDir:  o.Path,
			name:      o.Name,
			origin:    skillOriginFromRegistry(o.Origin),
			regOrigin: &ro,
		})
	}
	return refreshOriginGroup(originGroup{
		source:  first.Origin.Source,
		ref:     first.Origin.Ref,
		refKind: first.Origin.RefKind,
		skills:  targets,
	})
}

// skillOriginFromRegistry 把 registry.SkillOrigin 转成 cmd 层旧 skillOrigin。
func skillOriginFromRegistry(o registry.SkillOrigin) skillOrigin {
	return skillOrigin{
		Source:  o.Source,
		Ref:     o.Ref,
		RelPath: o.SubPath,
		Commit:  o.Commit,
	}
}

// updateAllSkillsLegacyDirs 是旧的 git-repo-dirs 路径，保留供兼容引用。
func updateAllSkillsLegacyDirs() error {
	dirs := managedGitRepoDirs(RegistryDir, DataDir)

	// 当指定 --global 或 --project 时应用范围过滤。
	if updateGlobal || updateProject {
		dirs = filterRepoDirsByScope(dirs)
	}

	repos := make([]namedRepo, len(dirs))
	for i, d := range dirs {
		repos[i] = namedRepo{path: d, label: d, skillRel: skillRelFromPath(RegistryDir, d)}
	}

	var summary updateSummary
	for _, r := range pullReposConcurrently(repos) {
		if r.skipped {
			summary.Skipped++
		} else if r.ok {
			summary.Updated++
		} else {
			summary.Errors++
		}
	}

	fmt.Printf("\nSummary: %d updated, %d pinned, %d errors\n", summary.Updated, summary.Skipped, summary.Errors)
	if summary.Errors > 0 {
		return fmt.Errorf("%d error(s) during update", summary.Errors)
	}
	return nil
}

// filterRepoDirsByScope 按 --global/--project 过滤 registry 的 skills 仓库：
// --global 只留特殊目录（.curated 等），--project 只留普通分类目录。
func filterRepoDirsByScope(dirs []string) []string {
	skillsRoot := filepath.Join(RegistryDir, "skills")
	filtered := dirs[:0]
	for _, d := range dirs {
		rel, err := filepath.Rel(skillsRoot, d)
		if err != nil {
			continue
		}
		category := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		isSpecial := registry.IsSpecialDir(category)
		if updateGlobal && isSpecial {
			filtered = append(filtered, d)
		} else if updateProject && !isSpecial {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// updateProjectSources 只更新 projectDir 下已安装技能反查到的 registry 源。
func updateProjectSources(names []string) error {
	return updateInstalledSourcesFiltered(updateDir, names, true, false)
}

// updateInstalledSources 更新当前项目 + 全局已安装技能对应的源。
func updateInstalledSources(projectDir string) error {
	return updateInstalledSourcesFiltered(projectDir, nil, true, true)
}

// updateInstalledNamed 按名称过滤已安装源；若无匹配则回退 registry 按名更新。
func updateInstalledNamed(projectDir string, names []string) error {
	targets := collectInstalledUpdateTargets(projectDir, names, true, true)
	if len(targets.gitRepos) == 0 && len(targets.originSkills) == 0 && len(targets.wellKnownSkills) == 0 {
		return updateSpecificSkills(names)
	}
	return updateCollectedTargets(targets, projectDir)
}

// updateInstalledSourcesFiltered 收集已安装源并更新。
func updateInstalledSourcesFiltered(projectDir string, names []string, includeProject, includeGlobal bool) error {
	targets := collectInstalledUpdateTargets(projectDir, names, includeProject, includeGlobal)
	return updateCollectedTargets(targets, projectDir)
}

// installedUpdateTargets 是已安装技能反查到的可更新对象：
//   - gitRepos：registry 内带 .git 的条目，或 agent 直接链到 source cache 的仓库
//   - originSkills：copy 入库但带 .sm-origin.json 的 registry 技能（需拉 cache 再回写）
//   - orphans：既无 .git 也无 origin 的已装技能名（update 无法刷新，仅提示）
type installedUpdateTargets struct {
	gitRepos        []string
	originSkills    []originSkillTarget
	wellKnownSkills []wellKnownSkillTarget
	orphans         []string
}

type originSkillTarget struct {
	skillDir  string
	name      string
	origin    skillOrigin
	regOrigin *registry.SkillOrigin // 非 nil：回写用 registry schema（保留 ref kind/source kind）
}

// wellKnownSkillTarget is a project-installed skill whose lock entry points
// to a Well-Known Source endpoint. It is refreshed by re-fetching the endpoint
// and reinstalling only the locked skill names.
type wellKnownSkillTarget struct {
	name   string
	source string
}

func pullSourceList(sources []string) error {
	// pullSourceList 仅处理纯 git 仓库 pull，不涉及 origin-backed 技能，
	// 也不写项目锁文件，故 projectDir 传空。
	return updateCollectedTargets(installedUpdateTargets{gitRepos: sources}, "")
}

// updateCollectedTargets 执行收集到的更新目标。projectDir 非空时，
// 在 origin-backed 技能回写后刷新项目 skills-lock.json 中对应条目的哈希，
// 保证锁文件与刷新后的内容一致。
func updateCollectedTargets(targets installedUpdateTargets, projectDir string) error {
	var summary updateSummary

	summary.add(updateGitTargets(targets.gitRepos))

	// origin-backed skills：按 Source+Ref 分组 → 刷新 cache → 回写 registry。
	// 回写后刷新项目 skills-lock.json 中对应条目的 computedHash，避免
	// update 拉取新内容后锁文件哈希过期。
	if len(targets.originSkills) > 0 {
		groups := groupOriginSkills(targets.originSkills)
		fmt.Printf("Refreshing %d origin-backed skill group(s)\n", len(groups))
		for _, g := range groups {
			u, s, e := refreshOriginGroup(g)
			summary.addCounts(u, s, e)
		}
		if projectDir != "" {
			refreshProjectLockAfterUpdate(projectDir, targets.originSkills)
		}
	}

	// Well-Known Source skills: their Registry originals deliberately have
	// no git/origin metadata, so refresh through the project lock source.
	if len(targets.wellKnownSkills) > 0 {
		summary.add(updateWellKnownTargets(targets.wellKnownSkills, projectDir))
	}

	// orphan：无法更新，提示重装以写入 origin。
	if len(targets.orphans) > 0 {
		warnOrphans(targets.orphans)
		summary.Skipped += len(targets.orphans)
	}

	if targets.empty() {
		fmt.Println("No installed skills with updatable sources found; nothing to update")
		fmt.Println("  Tip: sm update refreshes the entire Registry by default")
		return nil
	}

	fmt.Printf("\nSummary: %d updated, %d pinned/skipped, %d errors\n", summary.Updated, summary.Skipped, summary.Errors)
	if summary.Errors > 0 {
		return fmt.Errorf("%d error(s) during update", summary.Errors)
	}
	return nil
}

// updateGitTargets 并发 pull 纯 git 仓库，返回更新汇总。
func updateGitTargets(gitRepos []string) updateSummary {
	var summary updateSummary
	if len(gitRepos) == 0 {
		return summary
	}
	repos := make([]namedRepo, 0, len(gitRepos))
	for _, src := range gitRepos {
		repos = append(repos, namedRepo{path: src, label: src, skillRel: skillRelFromPath(RegistryDir, src)})
	}
	fmt.Printf("Updating %d git source(s)\n", len(repos))
	for _, r := range pullReposConcurrently(repos) {
		switch {
		case r.skipped:
			summary.Skipped++
		case r.ok:
			summary.Updated++
		default:
			summary.Errors++
		}
	}
	return summary
}

// updateWellKnownTargets 重新拉取 Well-Known Source 并重装锁定技能名。
func updateWellKnownTargets(skills []wellKnownSkillTarget, projectDir string) updateSummary {
	var summary updateSummary
	groups := groupWellKnownSkills(skills)
	fmt.Printf("Refreshing %d Well-Known Source group(s)\n", len(groups))
	for source, names := range groups {
		fmt.Printf("Updating Well-Known Source %s ... ", source)
		if err := installSkillsToAgents(source, nil, names, false, true, projectDir); err != nil {
			fmt.Printf("ERROR: %v\n", err)
			summary.Errors += len(names)
			continue
		}
		fmt.Printf("OK (%d skill(s))\n", len(names))
		summary.Updated += len(names)
	}
	return summary
}

// warnOrphans 打印无法更新的 orphan 技能提示。
func warnOrphans(orphans []string) {
	fmt.Fprintf(os.Stderr, "\n%d skill(s) cannot be updated (no git metadata and no .sm-origin.json):\n", len(orphans))
	for _, name := range orphans {
		fmt.Fprintf(os.Stderr, "  - %s\n", name)
	}
	fmt.Fprintf(os.Stderr, "  Tip: reinstall from source to record origin, e.g. sm install <source> -s %s\n", orphans[0])
}

// empty 报告是否没有任何可更新的目标。
func (t installedUpdateTargets) empty() bool {
	return len(t.gitRepos) == 0 && len(t.originSkills) == 0 &&
		len(t.wellKnownSkills) == 0 && len(t.orphans) == 0
}

func groupWellKnownSkills(skills []wellKnownSkillTarget) map[string][]string {
	groups := make(map[string][]string)
	for _, skill := range skills {
		groups[skill.source] = append(groups[skill.source], skill.name)
	}
	return groups
}

type originGroup struct {
	source  string
	ref     string
	refKind registry.RefKind
	skills  []originSkillTarget
}

// isPinned 报告该组是否 pinned（不自动前进）。
// tag/commit → pinned；default-branch/branch → tracking；
// 旧 metadata（refKind 未知）→ 非空 ref 保守视为 pinned（保留旧行为）。
func (g originGroup) isPinned() bool {
	switch g.refKind {
	case registry.RefTag, registry.RefCommit:
		return true
	case registry.RefBranch, registry.RefDefaultBranch:
		return false
	default:
		return g.ref != ""
	}
}

func groupOriginSkills(skills []originSkillTarget) []originGroup {
	index := map[string]int{}
	var groups []originGroup
	for _, s := range skills {
		key := s.origin.Source + "\x00" + s.origin.Ref
		if i, ok := index[key]; ok {
			groups[i].skills = append(groups[i].skills, s)
			continue
		}
		index[key] = len(groups)
		groups = append(groups, originGroup{
			source: s.origin.Source,
			ref:    s.origin.Ref,
			skills: []originSkillTarget{s},
		})
	}
	return groups
}

// refreshOriginGroup 刷新一组同 source+ref 的技能：
// - 无 ref（tracking）：pull source cache，再回写
// - 有 ref（pinned）：不改 cache HEAD，只确保 cache 存在并回写当前 pin 内容
func refreshOriginGroup(g originGroup) (updated, skipped, errors int) {
	label := g.source
	if g.ref != "" {
		label += "@" + g.ref
	}
	fmt.Printf("Updating origin %s ... ", label)

	// pinned：不自动前进；只保证 cache 在，并回写（内容应已是 pin）。
	if g.isPinned() {
		cacheDir, err := cachedGitSource(g.source, g.ref)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			return 0, 0, len(g.skills)
		}
		okN, errN := rewriteOriginSkills(cacheDir, g.skills)
		if errN > 0 {
			fmt.Printf("SKIPPED(pinned) rewrote %d, failed %d at %s\n", okN, errN, shortHash(gitHeadHash(cacheDir)))
		} else {
			fmt.Printf("SKIPPED: pinned at %s (rewrote %d)\n", shortHash(gitHeadHash(cacheDir)), okN)
		}
		// pinned 计为 skipped；rewrite 失败另计 errors。
		return 0, len(g.skills) - errN, errN
	}

	// tracking：default-branch 用空 ref cache key；显式 branch 用 branch ref cache key。
	cacheRef := ""
	if g.refKind == registry.RefBranch {
		cacheRef = g.ref
	}
	cacheDir, before, after, pinned, err := refreshTrackingCache(g.source, cacheRef)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return 0, 0, len(g.skills)
	}
	if pinned {
		okN, errN := rewriteOriginSkills(cacheDir, g.skills)
		fmt.Printf("SKIPPED: pinned at %s (rewrote %d)\n", shortHash(before), okN)
		return 0, len(g.skills) - errN, errN
	}

	okN, errN := rewriteOriginSkills(cacheDir, g.skills)
	if errN > 0 {
		fmt.Printf("ERROR: rewrote %d, failed %d\n", okN, errN)
		return okN, 0, errN
	}
	if before != after {
		fmt.Printf("OK (%s → %s), rewrote %d skill(s)\n", shortHash(before), shortHash(after), okN)
	} else {
		fmt.Printf("OK (already up to date), rewrote %d skill(s)\n", okN)
	}
	return okN, 0, 0
}

// refreshTrackingCache 拉取 tracking 分支的 source cache，返回（cacheDir、
// 拉取前后 HEAD）。detached HEAD 时不动 cache 并以 pinned=true 返回
// （等价 pinned，由调用方按 SKIPPED 处理）。
func refreshTrackingCache(source, cacheRef string) (cacheDir, before, after string, pinned bool, err error) {
	cacheDir, err = cachedGitSource(source, cacheRef)
	if err != nil {
		return "", "", "", false, err
	}
	before = gitHeadHash(cacheDir)

	if detached, derr := gitDetached(cacheDir); derr != nil {
		return "", "", "", false, derr
	} else if detached {
		return cacheDir, before, before, true, nil
	}
	if dirty, statusErr := gitDirty(cacheDir); statusErr != nil {
		return "", "", "", false, statusErr
	} else if dirty {
		return "", "", "", false, fmt.Errorf("local changes present in source cache")
	}
	if out, pullErr := exec.Command("git", "-C", cacheDir, "pull", "--ff-only").CombinedOutput(); pullErr != nil {
		return "", "", "", false, fmt.Errorf("%v\n%s", pullErr, out)
	}
	after = gitHeadHash(cacheDir)

	_, metaPath := sourceCachePaths(source, "")
	meta := readSourceCacheMetadata(metaPath)
	meta.Source = source
	meta.Commit = after
	_ = writeSourceCacheMetadata(metaPath, meta)
	return cacheDir, before, after, false, nil
}

// rewriteOriginSkills 从 cacheDir 按 origin.RelPath 覆盖 registry 技能目录。
// 同一 Source 的所有技能先统一 stage + lint，再一次性提交；任一技能失败
// 时整个 Source 保持旧内容（包括 .sm-origin.json），避免只回滚失败项。
func rewriteOriginSkills(cacheDir string, skills []originSkillTarget) (okN, errN int) {
	if len(skills) == 0 {
		return 0, 0
	}
	commit := gitHeadHash(cacheDir)
	reg := registry.New(RegistryDir)

	targets, infos, preflightErrs := prepareRewriteTargets(cacheDir, skills)
	if preflightErrs > 0 {
		return 0, len(skills)
	}

	_, err := updater.Apply(targets, updater.Hooks{
		Prepare: func(target updater.Target, staged string) error {
			info := infos[target.Destination]
			// Source caches should not contribute registry provenance. Write the
			// preserved registry schema only after staging the new content.
			_ = os.Remove(filepath.Join(staged, registry.OriginFile))
			origin := toRegistryOrigin(info.skill.origin)
			if info.skill.regOrigin != nil {
				origin = *info.skill.regOrigin
			}
			origin.Commit = commit
			if err := reg.WriteOrigin(staged, origin); err != nil {
				return fmt.Errorf("origin write %s: %w", info.skill.name, err)
			}
			return nil
		},
		Validate: func(target updater.Target, staged string) error {
			info := infos[target.Destination]
			rel := skillRelForLint(staged)
			if rel == "" {
				return fmt.Errorf("staging path %q is outside registry", staged)
			}
			if info.beforeHadErrs {
				return nil
			}
			if reg.LintSkill(rel).HasErrors() {
				return fmt.Errorf("updated skill %s failed validation", info.skill.name)
			}
			return nil
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: source update aborted; no Registry originals changed: %v\n", err)
		return 0, len(skills)
	}

	for _, s := range skills {
		warnCopyInstallsStale(s.name)
	}
	return len(skills), 0
}

// rewriteInfo 记录单个技能回写所需的 preflight 上下文。
type rewriteInfo struct {
	skill         originSkillTarget
	beforeHadErrs bool
}

// prepareRewriteTargets 把 skills 转成 updater 目标，并预检 cache 内
// 源路径存在性。返回（目标列表、按目标路径索引的上下文、预检失败数）。
// 任一源路径缺失时整体中止（保持 Source 旧内容）。
func prepareRewriteTargets(cacheDir string, skills []originSkillTarget) ([]updater.Target, map[string]rewriteInfo, int) {
	reg := registry.New(RegistryDir)
	targets := make([]updater.Target, 0, len(skills))
	infos := make(map[string]rewriteInfo, len(skills))
	preflightErrs := 0
	for _, s := range skills {
		src := cacheDir
		if s.origin.RelPath != "" && s.origin.RelPath != "." {
			src = filepath.Join(cacheDir, s.origin.RelPath)
		}
		if _, err := os.Stat(src); err != nil {
			fmt.Fprintf(os.Stderr, "warning: skill path %q missing in cache for %s: %v\n", s.origin.RelPath, s.name, err)
			preflightErrs++
		}

		rel := skillRelForLint(s.skillDir)
		beforeHadErrors := false
		if rel != "" {
			beforeHadErrors = reg.LintSkill(rel).HasErrors()
		}
		targets = append(targets, updater.Target{
			Name:        s.name,
			SourceDir:   src,
			Destination: s.skillDir,
		})
		infos[s.skillDir] = rewriteInfo{skill: s, beforeHadErrs: beforeHadErrors}
	}
	return targets, infos, preflightErrs
}

// toRegistryOrigin 把 cmd 层旧 skillOrigin 转成 registry.SkillOrigin。
// 旧 schema 无 source_kind/ref_kind，按 ReadOrigin 的兼容规则推断：
// source 非空 → git；ref 空 → default-branch；ref 非空 → pinned（RefUnknown 使 IsPinned true）。
func toRegistryOrigin(o skillOrigin) registry.SkillOrigin {
	ro := registry.SkillOrigin{
		Source:  o.Source,
		Ref:     o.Ref,
		SubPath: o.RelPath,
		Commit:  o.Commit,
	}
	if ro.SubPath == "" {
		ro.SubPath = "."
	}
	if ro.Source == "" {
		return ro
	}
	if ro.SourceKind == "" {
		ro.SourceKind = registry.SourceGit
	}
	if ro.RefKind == registry.RefUnknown {
		if ro.Ref == "" {
			ro.RefKind = registry.RefDefaultBranch
		}
		// 非空 ref：保留 RefUnknown（IsPinned() 返回 true，不前进）。
	}
	return ro
}

// refreshProjectLockAfterUpdate 在 origin-backed 技能被回写后，
// 重新计算其 registry 目录的内容哈希并更新项目 skills-lock.json
// 中同名条目的 computedHash（以及 ref）。仅更新已存在的条目；
// 锁文件不存在或无对应条目时静默跳过。
func refreshProjectLockAfterUpdate(projectDir string, skills []originSkillTarget) {
	lm := lockfile.NewManager(projectDir)
	if !lm.Exists() {
		return
	}
	lock, err := lm.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: reading skills-lock.json for refresh: %v\n", err)
		return
	}

	updated := 0
	for _, s := range skills {
		entry, ok := lock.Skills[s.name]
		if !ok {
			continue // 该技能无锁条目（非 project Direct Install），跳过
		}
		hash, err := lockfile.ComputeHash(s.skillDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: rehashing %s for lockfile: %v\n", s.name, err)
			continue
		}
		entry.ComputedHash = hash
		if entry.Ref == "" {
			entry.Ref = s.origin.Ref
		}
		updated++
	}
	if updated > 0 {
		if err := lm.Save(lock); err != nil {
			fmt.Fprintf(os.Stderr, "warning: writing skills-lock.json: %v\n", err)
		} else {
			fmt.Printf("Refreshed %d lockfile entr%s in skills-lock.json\n", updated, pluralEntry(updated))
		}
	}
}

// pluralEntry 返回 "y"/"ies" 以配合刷新计数输出。
func pluralEntry(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// updateInPlaceInstalls 就地刷新项目 projectDir 下的 Copy Install 实体：
//   - Link Install（symlink）跳过（它已跟随 registry，无独立内容可刷）。
//   - Copy Install（普通目录）读其 .sm-origin.json，用 source cache 覆盖自身内容，
//     并刷新 origin 的 commit。source cache 缺失则报错并指向 `sm update`（不触网）。
//
// 该路径不修改 registry。底层复用 replaceSkillDir / writeSkillOrigin / cachedGitSource。
// inPlaceCopyTarget 是一处可就地刷新的 Copy Install 实体：
// entityDir 是 agent 目录里的 copy 实体路径（写回目标）；name 仅用于显示。
type inPlaceCopyTarget struct {
	entityDir string
	name      string
	origin    skillOrigin
}

func updateInPlaceInstalls(projectDir string) error {
	// 1) 扫描项目级 agent 目录，收集带 origin 的 copy 实体；统计跳过的 symlink。
	targets, skippedLinks, missingOrigin := collectInPlaceCopyTargets(projectDir)

	if len(targets) == 0 {
		fmt.Printf("No Copy Install entities to refresh in %s\n", projectDir)
		if skippedLinks > 0 {
			fmt.Printf("  (skipped %d Link Install(s); use `sm update` to refresh the registry)\n", skippedLinks)
		}
		if missingOrigin > 0 {
			fmt.Printf("  (%d install(s) without origin cannot be refreshed in place)\n", missingOrigin)
		}
		return nil
	}

	// 2) 按 source+ref 分组，逐组拉 cache 并回写。
	type groupKey struct{ source, ref string }
	groups := map[groupKey][]inPlaceCopyTarget{}
	order := []groupKey{}
	for _, tgt := range targets {
		k := groupKey{tgt.origin.Source, tgt.origin.Ref}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], tgt)
	}

	updated, failed, missingCache := 0, 0, 0
	for _, k := range order {
		// offline=true：cache 不在就报错，绝不发起网络 clone（D4 "不触网"语义）。
		cacheDir, err := cachedGitSource(k.source, k.ref, true)
		if err != nil {
			missingCache += len(groups[k])
			fmt.Fprintf(os.Stderr, "  ERROR: %s\n", err)
			fmt.Fprintf(os.Stderr, "         run `sm update` (without --in-place) to rebuild the source cache first.\n")
			continue
		}
		commit := gitHeadHash(cacheDir)
		for _, tgt := range groups[k] {
			if refreshInPlaceTarget(tgt, cacheDir, commit) {
				updated++
			} else {
				failed++
			}
		}
	}

	fmt.Printf("\nSummary: %d refreshed, %d failed, %d missing cache", updated, failed, missingCache)
	if skippedLinks > 0 {
		fmt.Printf(", %d link installs skipped", skippedLinks)
	}
	if missingOrigin > 0 {
		fmt.Printf(", %d without origin", missingOrigin)
	}
	fmt.Println()
	return nil
}

// refreshInPlaceTarget 用 source cache 覆盖单个 copy 实体并回写 origin
// （更新 commit），成功打印 ✓ 并返回 true；失败打印 warning 并返回 false。
func refreshInPlaceTarget(tgt inPlaceCopyTarget, cacheDir, commit string) bool {
	src := cacheDir
	if tgt.origin.RelPath != "" && tgt.origin.RelPath != "." {
		src = filepath.Join(cacheDir, tgt.origin.RelPath)
	}
	if _, err := os.Stat(src); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: %s origin path %q missing in cache: %v\n", tgt.name, tgt.origin.RelPath, err)
		return false
	}
	// 覆盖 copy 实体内容（replaceSkillDir 会先清掉带入的旧 origin）。
	if _, err := replaceSkillDir(src, tgt.entityDir, false); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: refresh %s: %v\n", tgt.name, err)
		return false
	}
	// 重新写 origin（更新 commit），使后续 in-place 仍可追踪。
	tgt.origin.Commit = commit
	if err := writeSkillOrigin(tgt.entityDir, tgt.origin); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: origin write %s: %v\n", tgt.name, err)
		return false
	}
	fmt.Printf("  ✓ %s → %s\n", tgt.name, tgt.entityDir)
	return true
}

// collectInPlaceCopyTargets 扫描项目级 agent 目录，收集带 origin 的
// Copy Install 实体，并统计跳过的 Link Install（symlink）与无 origin 实体。
func collectInPlaceCopyTargets(projectDir string) (targets []inPlaceCopyTarget, skippedLinks, missingOrigin int) {
	for _, t := range tool.AllTools() {
		d := tool.GetProjectSkillDir(t, projectDir)
		if d == "" {
			continue
		}
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			link := filepath.Join(d, e.Name())
			info, err := os.Lstat(link)
			if err != nil {
				continue
			}
			if info.Mode()&os.ModeSymlink != 0 {
				// Link Install：就地刷新无意义，跳过。
				skippedLinks++
				continue
			}
			if !info.IsDir() {
				continue
			}
			origin, ok := readSkillOrigin(link)
			if !ok {
				// 无 origin（本地源入库等）→ 无法就地刷新。
				missingOrigin++
				continue
			}
			targets = append(targets, inPlaceCopyTarget{entityDir: link, name: e.Name(), origin: origin})
		}
	}
	return targets, skippedLinks, missingOrigin
}

// gitPullRepo 对单个仓库执行前置检查与 git pull --ff-only：
//   - detached HEAD → skipped=true（钉在固定 commit，不执行 pull）；
//   - 工作区有本地改动 → 报错，要求先 commit/stash；
//   - 否则执行 pull 并返回输出。
func gitPullRepo(repo namedRepo) (output []byte, err error, skipped bool) {
	if detached, detachedErr := gitDetached(repo.path); detachedErr != nil {
		return nil, detachedErr, false
	} else if detached {
		return nil, nil, true
	}
	if dirty, statusErr := gitDirty(repo.path); statusErr != nil {
		return nil, statusErr, false
	} else if dirty {
		return nil, fmt.Errorf("local changes present; commit or stash them first"), false
	}
	pullCmd := exec.Command("git", "-C", repo.path, "pull", "--ff-only")
	output, err = pullCmd.CombinedOutput()
	return output, err, false
}

// rollbackIfLintFails 在 pull 更新了 commit 且引入 lint 错误时，把仓库
// git reset --hard 回滚到 beforeHash。返回（可能改写的）错误与回滚标记；
// 无回滚发生时返回 (nil, false)。
func rollbackIfLintFails(repo namedRepo, beforeHash string, commitChanged, beforeLintErrors bool) (error, bool) {
	if !commitChanged || beforeLintErrors {
		return nil, false
	}
	reg := registry.New(RegistryDir)
	if !reg.LintSkill(repo.skillRel).HasErrors() {
		return nil, false
	}
	resetOutput, resetErr := exec.Command("git", "-C", repo.path, "reset", "--hard", beforeHash).CombinedOutput()
	if resetErr != nil {
		return fmt.Errorf("updated skill failed validation; rollback failed: %v: %s", resetErr, resetOutput), false
	}
	return fmt.Errorf("updated skill failed validation; rolled back to %s", shortHash(beforeHash)), true
}

// printPullOutcome 序列化输出单个仓库的 pull 结果（由 outMu 保护调用）。
func printPullOutcome(outMu *sync.Mutex, repo namedRepo, skipped, ok bool, err error, output []byte, commitChanged bool, beforeHash string, beforeScore, afterScore *registry.SkillScore) {
	outMu.Lock()
	defer outMu.Unlock()
	if skipped {
		fmt.Printf("SKIPPED: pinned at %s\n", shortHash(beforeHash))
	} else if ok {
		fmt.Print("OK")
		if commitChanged && beforeScore != nil && afterScore != nil {
			printScoreDelta(repo.label, beforeScore, afterScore)
		}
		fmt.Println()
	} else {
		fmt.Printf("ERROR: %v\n%s\n", err, string(output))
	}
}

// warnCopyInstallsStale 提示：registry 已更新，但 agent 目录里若是 --copy
// 实体目录则不会自动同步，需重装或改用 symlink。
func warnCopyInstallsStale(skillName string) {
	var paths []string
	for _, t := range tool.AllTools() {
		// 全局
		if t.SkillDir != "" {
			p := filepath.Join(tool.GetGlobalSkillDir(t), skillName)
			if info, err := os.Lstat(p); err == nil && info.Mode()&os.ModeSymlink == 0 {
				paths = append(paths, p)
			}
		}
	}
	// 当前项目
	if wd, err := os.Getwd(); err == nil {
		for _, t := range tool.AllTools() {
			if d := tool.GetProjectSkillDir(t, wd); d != "" {
				p := filepath.Join(d, skillName)
				if info, err := os.Lstat(p); err == nil && info.Mode()&os.ModeSymlink == 0 {
					paths = append(paths, p)
				}
			}
		}
	}
	if len(paths) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "  note: %q was installed with --copy in %d location(s); registry updated but copies were not rewritten:\n", skillName, len(paths))
	for _, p := range paths {
		fmt.Fprintf(os.Stderr, "    - %s\n", p)
	}
	fmt.Fprintf(os.Stderr, "    reinstall without --copy (symlink) or re-run install --copy to refresh agent dirs\n")
}

// collectInstalledUpdateTargets 扫描已安装技能，收集 git 仓库与 origin-backed registry 技能。
func collectInstalledUpdateTargets(projectDir string, names []string, includeProject, includeGlobal bool) installedUpdateTargets {
	seenRepo := map[string]bool{}
	seenOrigin := map[string]bool{} // skillDir
	var targets installedUpdateTargets
	for _, t := range tool.AllTools() {
		if includeProject && projectDir != "" {
			if d := tool.GetProjectSkillDir(t, projectDir); d != "" {
				collectDirUpdateTargets(d, names, seenRepo, seenOrigin, &targets)
			}
		}
		if includeGlobal {
			collectDirUpdateTargets(tool.GetGlobalSkillDir(t), names, seenRepo, seenOrigin, &targets)
		}
	}
	if includeProject && projectDir != "" {
		collectWellKnownUpdateTargets(projectDir, names, &targets)
	}
	return targets
}

func collectWellKnownUpdateTargets(projectDir string, names []string, targets *installedUpdateTargets) {
	lock, err := lockfile.NewManager(projectDir).Load()
	if err != nil {
		return
	}
	selected := make(map[string]bool)
	for _, name := range names {
		selected[strings.ToLower(name)] = true
	}
	wellKnownNames := make(map[string]bool)
	for name, entry := range lock.Skills {
		if entry.SourceType != "well-known" || entry.SourceURL == "" {
			continue
		}
		if len(selected) > 0 && !selected[strings.ToLower(name)] {
			continue
		}
		targets.wellKnownSkills = append(targets.wellKnownSkills, wellKnownSkillTarget{name: name, source: entry.SourceURL})
		wellKnownNames[name] = true
	}
	if len(wellKnownNames) == 0 {
		return
	}
	filtered := targets.orphans[:0]
	for _, name := range targets.orphans {
		if !wellKnownNames[name] {
			filtered = append(filtered, name)
		}
	}
	targets.orphans = filtered
}

// collectInstalledSources 兼容旧测试/调用：只返回 git 仓库路径。
func collectInstalledSources(projectDir string, names []string, includeProject, includeGlobal bool) []string {
	return collectInstalledUpdateTargets(projectDir, names, includeProject, includeGlobal).gitRepos
}

// collectDirUpdateTargets 扫描某个 agent 技能目录，把其中每个已装技能
// 归类为三类可更新目标之一，写回 *targets：
//
//  1. gitRepos   —— 内容根在 git 仓库里（registry 的 skill clone 或 sources
//     缓存）：走 git pull 刷新，按仓库去重（seenRepo）。
//  2. originSkills —— registry 技能目录带 .sm-origin.json（--copy 装或
//     从源克隆后剥离 .git）：按源重新拉取覆盖，按 regPath 去重
//     （seenOrigin）。
//  3. orphans    —— 既无 git 仓库也无 origin（registry 无原件或原件无来源）：
//     无法 update，仅记名提示，按技能名去重。
//
// names 非空时只收集指定技能（--skill 过滤）。决策顺序：symlink 跟到目标 →
// 否则 copy 安装查 registry 同名 → 找 nearestGitRepo → 否则读 origin →
// 否则 orphan。这是 update 命令最绕的一段，每条路径的判定见函数内分段注释。
func collectDirUpdateTargets(dir string, names []string, seenRepo, seenOrigin map[string]bool, targets *installedUpdateTargets) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	nameSet := map[string]bool{}
	for _, n := range names {
		nameSet[n] = true
	}
	for _, e := range entries {
		if len(nameSet) > 0 && !nameSet[e.Name()] {
			continue
		}
		classifyDirUpdateTarget(e.Name(), filepath.Join(dir, e.Name()), seenRepo, seenOrigin, targets)
	}
}

// classifyDirUpdateTarget 判定单个已装条目的更新目标并归类到 targets。
// 决策顺序：symlink 跟到目标 → 否则 copy 安装查 registry 同名 →
// 找 nearestGitRepo → 否则读 origin → 否则 orphan。
func classifyDirUpdateTarget(name, link string, seenRepo, seenOrigin map[string]bool, targets *installedUpdateTargets) {
	reg := registry.New(RegistryDir)

	// 解析“内容根”：symlink 跟到目标；copy 安装则看 registry 同名。
	contentPath, ok := resolveContentRoot(link, name, reg)
	if !ok {
		return
	}

	// 优先：content 位于 git 仓库（registry skill clone 或 source cache）。
	if repo := nearestGitRepo(contentPath, RegistryDir, filepath.Join(DataDir, "sources")); repo != "" {
		if !seenRepo[repo] {
			seenRepo[repo] = true
			targets.gitRepos = append(targets.gitRepos, repo)
		}
		return
	}

	classifyOriginTarget(name, contentPath, reg, seenOrigin, targets)
}

// resolveContentRoot 返回条目对应的“内容根”路径（symlink 目标或 registry
// 同名目录）；条目既非受管 symlink 也无 registry 原件时返回 ok=false。
func resolveContentRoot(link, name string, reg *registry.Registry) (string, bool) {
	if symlink.IsSymlink(link) {
		if !symlink.PointInside(link, RegistryDir) && !symlink.PointInside(link, filepath.Join(DataDir, "sources")) {
			return "", false
		}
		target, err := filepath.EvalSymlinks(link)
		if err != nil {
			return "", false
		}
		return target, true
	}
	// 非 symlink：可能是 --copy 安装；尝试 registry 同名。
	if regPath, _ := reg.FindSkillDir(name); regPath != "" {
		return regPath, true
	}
	return "", false
}

// classifyOriginTarget 按 origin 归属内容根：带 .sm-origin.json 则记为
// origin 目标，否则记 orphan（均按路径去重）。
func classifyOriginTarget(name, contentPath string, reg *registry.Registry, seenOrigin map[string]bool, targets *installedUpdateTargets) {
	// registry 技能目录上的 .sm-origin.json；没有则记 orphan。
	regPath := contentPath
	if !pathInside(regPath, RegistryDir) {
		if p, _ := reg.FindSkillDir(name); p != "" {
			regPath = p
		} else {
			// 已装在 agent 目录，但 registry 无对应原件。
			addOrphanIfNew(name, name, seenOrigin, targets)
			return
		}
	}
	if origin, ok := readSkillOrigin(regPath); ok {
		if seenOrigin[regPath] {
			return
		}
		seenOrigin[regPath] = true
		targets.originSkills = append(targets.originSkills, originSkillTarget{
			skillDir: regPath,
			name:     name,
			origin:   origin,
		})
		return
	}
	// registry 有目录但无 git、无 origin。
	addOrphanIfNew(name, regPath, seenOrigin, targets)
}

// addOrphanIfNew 按 key 去重后追加 orphan 记录（orphan 列表本身记录技能名）。
func addOrphanIfNew(name, key string, seenOrigin map[string]bool, targets *installedUpdateTargets) {
	if seenOrigin[key] {
		return
	}
	seenOrigin[key] = true
	targets.orphans = append(targets.orphans, name)
}

// pathInside 判断 path（解析符号链接后）是否位于 root 目录之内。
// 用 Rel 结果是否以 ".." 开头来判定，避免简单的 HasPrefix 误判
// （如 /a/b 与 /a/bbb）。供 update/cache 多处判断"安装是否指向 registry 或缓存"。
func pathInside(path, root string) bool {
	absPath, err1 := filepath.Abs(path)
	absRoot, err2 := filepath.Abs(root)
	if err1 != nil || err2 != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = resolved
	}
	if resolved, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = resolved
	}
	rel, err := filepath.Rel(absRoot, absPath)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// projectInstalledSources 扫描 projectDir 下所有工具的项目级技能目录，
// 收集指向 RegistryDir 内的符号链接所对应的 git 源目录（去重）。
func projectInstalledSources(projectDir string) []string {
	seen := map[string]bool{}
	var sources []string
	for _, t := range tool.AllTools() {
		dir := tool.GetProjectSkillDir(t, projectDir)
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // 项目未安装该工具的技能
		}
		for _, e := range entries {
			link := filepath.Join(dir, e.Name())
			if !symlink.IsSymlink(link) || (!symlink.PointInside(link, RegistryDir) && !symlink.PointInside(link, filepath.Join(DataDir, "sources"))) {
				continue
			}
			target, err := filepath.EvalSymlinks(link)
			if err != nil {
				continue
			}
			repo := nearestGitRepo(target, RegistryDir, filepath.Join(DataDir, "sources"))
			if repo != "" && !seen[repo] {
				seen[repo] = true
				sources = append(sources, repo)
			}
		}
	}
	return sources
}

// nearestGitRepo 从 path（解析符号链接后）逐级向上查找最近的含 .git 目录，
// 但不得越过任一 root（roots 经符号链接规范化后作为上界）。命中返回该仓库
// 路径，否则返回 ""。用于判定一个已装技能的内容根是否落在可 git pull 的仓库里。
func nearestGitRepo(path string, roots ...string) string {
	for i, root := range roots {
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			roots[i] = resolved
		}
	}
	for p := path; p != "" && p != filepath.Dir(p); p = filepath.Dir(p) {
		inside := false
		for _, root := range roots {
			rel, err := filepath.Rel(root, p)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				inside = true
				break
			}
		}
		if !inside {
			return ""
		}
		if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
			return p
		}
	}
	return ""
}

func updateSpecificSkills(names []string) error {
	var targets installedUpdateTargets
	seenRepo := map[string]bool{}
	notFound := 0
	reg := registry.New(RegistryDir)

	for _, name := range names {
		skillPath, _ := reg.FindSkillDir(name)
		if skillPath == "" {
			fmt.Fprintf(os.Stderr, "warning: skill %q not found in registry\n", name)
			notFound++
			continue
		}
		if repo := nearestGitRepo(skillPath, RegistryDir); repo != "" {
			if !seenRepo[repo] {
				seenRepo[repo] = true
				targets.gitRepos = append(targets.gitRepos, repo)
			}
			continue
		}
		if origin, ok := readSkillOrigin(skillPath); ok {
			targets.originSkills = append(targets.originSkills, originSkillTarget{
				skillDir: skillPath,
				name:     name,
				origin:   origin,
			})
			continue
		}
		// 无 git 无 origin：作为 orphan 进入 updateCollectedTargets 提示
		targets.orphans = append(targets.orphans, name)
	}
	if err := updateCollectedTargets(targets, ""); err != nil {
		return err
	}
	if notFound > 0 {
		fmt.Printf("(plus %d skill(s) not found)\n", notFound)
	}
	return nil
}

// 一次更新的汇总：成功、跳过、失败计数。

type updateSummary struct {
	Updated int
	Skipped int
	Errors  int
}

// add 累加另一份汇总的计数。
func (s *updateSummary) add(o updateSummary) {
	s.addCounts(o.Updated, o.Skipped, o.Errors)
}

// addCounts 累加三项计数。
func (s *updateSummary) addCounts(updated, skipped, errors int) {
	s.Updated += updated
	s.Skipped += skipped
	s.Errors += errors
}

// gitRepoDirs 遍历注册表的 skills/mcp 子树，返回所有含 .git 子目录的
// 绝对路径（即所有 git 管理的注册表条目）。
func managedGitRepoDirs(registryDir, dataDir string) []string {
	repos := gitRepoDirs(registryDir)
	cacheRoot := filepath.Join(dataDir, "sources")
	filepath.WalkDir(cacheRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && d.Name() == ".git" {
			repos = append(repos, filepath.Dir(path))
			return filepath.SkipDir
		}
		return nil
	})
	return repos
}

func gitRepoDirs(registryDir string) []string {
	var repos []string
	for _, root := range []string{
		filepath.Join(registryDir, "skills"),
		filepath.Join(registryDir, "mcp"),
	} {
		filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() && d.Name() == ".git" {
				repos = append(repos, filepath.Dir(path))
				return filepath.SkipDir
			}
			return nil
		})
	}
	return repos
}

// pullResult 是单次 git pull 的结果。
type pullResult struct {
	label         string // human-friendly name (repo dir or skill name)
	ok            bool
	before        *registry.SkillScore // pull 前评分；nil 表示非 skill 或评分失败
	after         *registry.SkillScore // pull 后评分
	commitChanged bool                 // git rev-parse HEAD 前后是否不同
	rolledBack    bool                 // 更新后校验失败且已恢复旧 commit
	skipped       bool                 // 固定在 detached HEAD，不自动更新
}

// pullReposConcurrently 并发执行 `git -C <repo> pull --ff-only`。
// git pull 是网络+I/O 密集型，并发能显著缩短墙上时间；
// worker 数上限 8 以避免压垮主机或触发远端限流。
func pullReposConcurrently(repos []namedRepo) []pullResult {
	if len(repos) == 0 {
		return nil
	}

	var (
		mu      sync.Mutex
		results = make([]pullResult, len(repos))
		outMu   sync.Mutex // 序列化进度输出，避免行交错
	)

	// pull 执行单个仓库的 git pull。
	pull := func(i int) {
		repo := repos[i]

		outMu.Lock()
		fmt.Printf("Updating %s ... ", repo.label)
		outMu.Unlock()

		beforeHash := gitHeadHash(repo.path)
		var beforeScore *registry.SkillScore
		beforeLintErrors := false
		if repo.skillRel != "" {
			reg := registry.New(RegistryDir)
			beforeScore = reg.ScoreSkill(repo.skillRel)
			beforeLintErrors = reg.LintSkill(repo.skillRel).HasErrors()
		}

		output, err, skipped := gitPullRepo(repo)

		ok := err == nil && !skipped
		commitChanged := ok && beforeHash != "" && gitHeadHash(repo.path) != beforeHash
		var afterScore *registry.SkillScore
		rolledBack := false
		if repo.skillRel != "" && ok {
			reg := registry.New(RegistryDir)
			afterScore = reg.ScoreSkill(repo.skillRel)
			if rbErr, rb := rollbackIfLintFails(repo, beforeHash, commitChanged, beforeLintErrors); rbErr != nil {
				err, rolledBack, ok = rbErr, rb, false
			}
		}

		printPullOutcome(&outMu, repo, skipped, ok, err, output, commitChanged, beforeHash, beforeScore, afterScore)

		mu.Lock()
		results[i] = pullResult{
			label:         repo.label,
			ok:            ok,
			before:        beforeScore,
			after:         afterScore,
			commitChanged: commitChanged,
			rolledBack:    rolledBack,
			skipped:       skipped,
		}
		mu.Unlock()
	}

	// 并发上限：git pull 是 I/O+网络密集型，允许真实并行，但封顶 8，
	// 避免压垮主机或触发远端限流（封顶逻辑由 RunIndexed 内部处理）。
	concurrency.RunIndexed(len(repos), 8, pull)
	return results
}

// namedRepo 把仓库路径与展示标签配对，供并发 puller 使用。
type namedRepo struct {
	path     string
	label    string
	skillRel string // 相对 <registry>/skills 的路径（如 "global/my-skill"）；空表示非 skill（如 MCP）
}

// skillRelFromPath 在 repoPath 位于 <registry>/skills 子树且含 SKILL.md 时，
// 返回相对路径（如 "global/my-skill"）；否则返回 ""。
// 用于评分：只有 skill 才有 SKILL.md 可评，MCP 不评。
func skillRelFromPath(registryDir, repoPath string) string {
	skillsRoot := filepath.Join(registryDir, "skills")
	rel, err := filepath.Rel(skillsRoot, repoPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	if _, err := os.Stat(filepath.Join(repoPath, "SKILL.md")); err != nil {
		return ""
	}
	return rel
}

// updateGitRepos 发现并拉取 registryDir 下所有 git 管理的仓库。
// 导出供测试使用；生产路径走 updateAllSkills（先应用范围过滤）。
func updateGitRepos(registryDir string) (updateSummary, error) {
	dirs := gitRepoDirs(registryDir)
	repos := make([]namedRepo, len(dirs))
	for i, d := range dirs {
		repos[i] = namedRepo{path: d, label: d, skillRel: skillRelFromPath(registryDir, d)}
	}
	results := pullReposConcurrently(repos)
	var summary updateSummary
	for _, r := range results {
		if r.skipped {
			summary.Skipped++
		} else if r.ok {
			summary.Updated++
		} else {
			summary.Errors++
		}
	}
	return summary, nil
}

// gitDetached 判断仓库是否处于 detached HEAD（被 update 视为 pinned，跳过）。
// 利用 git symbolic-ref -q HEAD：附着在分支上时退出 0，detached 时退出 1，
// 其它错误才视为真实失败。
func gitDetached(repoPath string) (bool, error) {
	err := exec.Command("git", "-C", repoPath, "symbolic-ref", "-q", "HEAD").Run()
	if err == nil {
		return false, nil
	}
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		return true, nil
	}
	return false, fmt.Errorf("checking pinned state: %w", err)
}

// gitDirty 判断仓库是否有未提交的本地改动（status --porcelain 输出非空）。
// 有改动时 update 拒绝 pull，以免覆盖用户修改。
func gitDirty(repoPath string) (bool, error) {
	out, err := exec.Command("git", "-C", repoPath, "status", "--porcelain").Output()
	if err != nil {
		return false, fmt.Errorf("checking local changes: %w", err)
	}
	return len(out) > 0, nil
}

// shortHash 把完整 hash 截短为 12 字符前缀，用于日志展示。
func shortHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

// gitHeadHash 返回 repoPath 的当前 HEAD commit hash（40 字符十六进制）。
// git 缺失或非 git 仓库时返回 ""。
func gitHeadHash(repoPath string) string {
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// printScoreDelta 把 update 前后的评分变化格式化输出到 stdout。
// 仅在 commit 实际变化时调用。下降时加 ⚠，并列出扣分项 note。
func printScoreDelta(label string, before, after *registry.SkillScore) {
	delta := after.Total - before.Total
	sign := "+"
	if delta < 0 {
		sign = ""
	}
	marker := " "
	if delta < 0 {
		marker = "⚠"
	}
	fmt.Printf("  %s Score: %d → %d (%s%d)", marker, before.Total, after.Total, sign, delta)
	if len(after.Notes) > 0 && delta < 0 {
		fmt.Printf("  [%s]", strings.Join(after.Notes, ", "))
	}
}

func init() {
	updateCmd.Flags().BoolVarP(&updateGlobal, "global", "g", false, "Only update global category entries when using --registry")
	updateCmd.Flags().BoolVarP(&updateProject, "project", "p", false, "Only update non-special category entries when using --registry")
	updateCmd.Flags().BoolVarP(&updateYes, "yes", "y", false, "Skip prompts")
	updateCmd.Flags().StringVar(&updateDir, "dir", "",
		"Project root for installed-skill scan (default: current directory)")
	updateCmd.Flags().BoolVar(&updateRegistry, "registry", false, "Update entire registry instead of only installed sources")
	updateCmd.Flags().BoolVar(&updateInPlace, "in-place", false, "Refresh project Copy Install entities in place from their origin (no registry change)")
	rootCmd.AddCommand(updateCmd)
}

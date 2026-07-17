// cmd/update.go 实现 `sm update`：并发 `git pull` 更新注册表中
// 由 git 管理的条目；支持按技能名或 --global/--project 过滤。
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
)

var (
	updateGlobal   bool
	updateProject  bool
	updateYes      bool
	updateDir      string // --dir: 项目根（默认 cwd）；扫已安装
	updateRegistry bool   // --registry: 更新整个 registry（旧默认）
)

var updateCmd = &cobra.Command{
	Use:   "update [skills...]",
	Short: "Update installed skills to latest versions",
	Long: `Update registry sources that back currently Installed Skills.

Without arguments, updates sources referenced by installs in the current
project (./<agent>/skills) and global agent dirs. With skill names, updates
only those skills (if present in the registry and git-managed).

Examples:
  # Update sources for currently installed skills
  sm update

  # Update a single skill by name
  sm update my-skill

  # Only project installs
  sm update --dir .

  # Update entire registry (legacy / curation)
  sm update --registry

  # Non-interactive
  sm update -y
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if updateRegistry {
			if len(args) > 0 {
				return updateSpecificSkills(args)
			}
			return updateAllSkills()
		}
		// 默认：按已安装（项目优先；可用 --dir 指定项目根）
		dir := updateDir
		if dir == "" {
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			dir = wd
		}
		if len(args) > 0 {
			// 名称过滤：先按项目/全局已安装源，再按名过滤；找不到再回退 registry 名
			return updateInstalledNamed(dir, args)
		}
		return updateInstalledSources(dir)
	},
}

// 更新注册表中所有 git 管理的条目（并发 git pull）。
// --global/--project 可限定范围。

func updateAllSkills() error {
	dirs := managedGitRepoDirs(RegistryDir, DataDir)

	// 当指定 --global 或 --project 时应用范围过滤。
	if updateGlobal || updateProject {
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
		dirs = filtered
	}

	repos := make([]namedRepo, len(dirs))
	for i, d := range dirs {
		repos[i] = namedRepo{path: d, label: d, skillRel: skillRelFromPath(RegistryDir, d)}
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

	fmt.Printf("\nSummary: %d updated, %d pinned, %d errors\n", summary.Updated, summary.Skipped, summary.Errors)
	return nil
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
	if len(targets.gitRepos) == 0 && len(targets.originSkills) == 0 {
		return updateSpecificSkills(names)
	}
	return updateCollectedTargets(targets)
}

// updateInstalledSourcesFiltered 收集已安装源并更新。
func updateInstalledSourcesFiltered(projectDir string, names []string, includeProject, includeGlobal bool) error {
	targets := collectInstalledUpdateTargets(projectDir, names, includeProject, includeGlobal)
	if len(targets.gitRepos) == 0 && len(targets.originSkills) == 0 {
		fmt.Println("No installed skills with updatable sources found; nothing to update")
		fmt.Println("  Tip: sm update --registry updates the entire registry")
		return nil
	}
	return updateCollectedTargets(targets)
}

// installedUpdateTargets 是已安装技能反查到的可更新对象：
//   - gitRepos：registry 内带 .git 的条目，或 agent 直接链到 source cache 的仓库
//   - originSkills：copy 入库但带 .sm-origin.json 的 registry 技能（需拉 cache 再回写）
type installedUpdateTargets struct {
	gitRepos     []string
	originSkills []originSkillTarget
}

type originSkillTarget struct {
	skillDir string
	name     string
	origin   skillOrigin
}

func pullSourceList(sources []string) error {
	return updateCollectedTargets(installedUpdateTargets{gitRepos: sources})
}

func updateCollectedTargets(targets installedUpdateTargets) error {
	var summary updateSummary

	// 1) 纯 git 仓库：并发 pull
	if len(targets.gitRepos) > 0 {
		repos := make([]namedRepo, 0, len(targets.gitRepos))
		for _, src := range targets.gitRepos {
			repos = append(repos, namedRepo{path: src, label: src, skillRel: skillRelFromPath(RegistryDir, src)})
		}
		fmt.Printf("Updating %d git source(s)\n", len(repos))
		results := pullReposConcurrently(repos)
		for _, r := range results {
			if r.skipped {
				summary.Skipped++
			} else if r.ok {
				summary.Updated++
			} else {
				summary.Errors++
			}
		}
	}

	// 2) origin-backed skills：按 Source+Ref 分组 → 刷新 cache → 回写 registry
	if len(targets.originSkills) > 0 {
		groups := groupOriginSkills(targets.originSkills)
		fmt.Printf("Refreshing %d origin-backed skill group(s)\n", len(groups))
		for _, g := range groups {
			u, s, e := refreshOriginGroup(g)
			summary.Updated += u
			summary.Skipped += s
			summary.Errors += e
		}
	}

	fmt.Printf("\nSummary: %d updated, %d pinned, %d errors\n", summary.Updated, summary.Skipped, summary.Errors)
	return nil
}

type originGroup struct {
	source string
	ref    string
	skills []originSkillTarget
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

	// pinned：不自动前进；只保证 cache 在，并回写（内容应已是 pin）
	if g.ref != "" {
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
		// pinned 计为 skipped；rewrite 失败另计 errors
		return 0, len(g.skills) - errN, errN
	}

	cacheDir, err := cachedGitSource(g.source, "")
	if err != nil {
		fmt.Printf("ERROR: cache: %v\n", err)
		return 0, 0, len(g.skills)
	}

	before := gitHeadHash(cacheDir)
	if detached, derr := gitDetached(cacheDir); derr != nil {
		fmt.Printf("ERROR: %v\n", derr)
		return 0, 0, len(g.skills)
	} else if detached {
		okN, errN := rewriteOriginSkills(cacheDir, g.skills)
		fmt.Printf("SKIPPED: pinned at %s (rewrote %d)\n", shortHash(before), okN)
		return 0, len(g.skills) - errN, errN
	}
	if dirty, statusErr := gitDirty(cacheDir); statusErr != nil {
		fmt.Printf("ERROR: %v\n", statusErr)
		return 0, 0, len(g.skills)
	} else if dirty {
		fmt.Printf("ERROR: local changes present in source cache\n")
		return 0, 0, len(g.skills)
	}
	if out, pullErr := exec.Command("git", "-C", cacheDir, "pull", "--ff-only").CombinedOutput(); pullErr != nil {
		fmt.Printf("ERROR: %v\n%s\n", pullErr, out)
		return 0, 0, len(g.skills)
	}
	after := gitHeadHash(cacheDir)
	_, metaPath := sourceCachePaths(g.source, "")
	meta := readSourceCacheMetadata(metaPath)
	meta.Source = g.source
	meta.Commit = after
	_ = writeSourceCacheMetadata(metaPath, meta)

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

// rewriteOriginSkills 从 cacheDir 按 origin.RelPath 覆盖 registry 技能目录，并刷新 origin commit。
func rewriteOriginSkills(cacheDir string, skills []originSkillTarget) (okN, errN int) {
	commit := gitHeadHash(cacheDir)
	for _, s := range skills {
		src := cacheDir
		if s.origin.RelPath != "" && s.origin.RelPath != "." {
			src = filepath.Join(cacheDir, s.origin.RelPath)
		}
		if _, err := os.Stat(src); err != nil {
			fmt.Fprintf(os.Stderr, "warning: skill path %q missing in cache for %s: %v\n", s.origin.RelPath, s.name, err)
			errN++
			continue
		}
		// lint before/after optional: keep simple — copy then write origin
		if err := replaceSkillDir(src, s.skillDir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: rewrite %s: %v\n", s.name, err)
			errN++
			continue
		}
		s.origin.Commit = commit
		if err := writeSkillOrigin(s.skillDir, s.origin); err != nil {
			fmt.Fprintf(os.Stderr, "warning: origin write %s: %v\n", s.name, err)
			errN++
			continue
		}
		okN++
	}
	return okN, errN
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
			collectDirUpdateTargets(filepath.Join(home.Dir(), t.SkillDir), names, seenRepo, seenOrigin, &targets)
		}
	}
	return targets
}

// collectInstalledSources 兼容旧测试/调用：只返回 git 仓库路径。
func collectInstalledSources(projectDir string, names []string, includeProject, includeGlobal bool) []string {
	return collectInstalledUpdateTargets(projectDir, names, includeProject, includeGlobal).gitRepos
}

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
		link := filepath.Join(dir, e.Name())
		reg := registry.New(RegistryDir)

		// 解析“内容根”：symlink 跟到目标；copy 安装则看 registry 同名
		var contentPath string
		if symlink.IsSymlink(link) {
			if !symlink.PointInside(link, RegistryDir) && !symlink.PointInside(link, filepath.Join(DataDir, "sources")) {
				continue
			}
			target, err := filepath.EvalSymlinks(link)
			if err != nil {
				continue
			}
			contentPath = target
		} else {
			// 非 symlink：可能是 --copy 安装；尝试 registry 同名
			if regPath, _ := reg.FindSkillDir(e.Name()); regPath != "" {
				contentPath = regPath
			} else {
				continue
			}
		}

		// 优先：content 位于 git 仓库（registry skill clone 或 source cache）
		if repo := nearestGitRepo(contentPath, RegistryDir, filepath.Join(DataDir, "sources")); repo != "" {
			if !seenRepo[repo] {
				seenRepo[repo] = true
				targets.gitRepos = append(targets.gitRepos, repo)
			}
			continue
		}

		// 否则：registry 技能目录上的 .sm-origin.json
		regPath := contentPath
		if !pathInside(regPath, RegistryDir) {
			if p, _ := reg.FindSkillDir(e.Name()); p != "" {
				regPath = p
			} else {
				continue
			}
		}
		if origin, ok := readSkillOrigin(regPath); ok {
			if seenOrigin[regPath] {
				continue
			}
			seenOrigin[regPath] = true
			targets.originSkills = append(targets.originSkills, originSkillTarget{
				skillDir: regPath,
				name:     e.Name(),
				origin:   origin,
			})
		}
	}
}

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

// nearestGitRepo 从 path 向上查找最近含 .git 的目录；
// 不越过 RegistryDir，找不到返回 ""。
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
		fmt.Fprintf(os.Stderr, "warning: skill %q is not git-managed and has no origin metadata\n", name)
		notFound++
	}

	if len(targets.gitRepos) == 0 && len(targets.originSkills) == 0 {
		fmt.Printf("\nSummary: 0 updated, 0 pinned, %d errors\n", notFound)
		return nil
	}
	// fold notFound into summary after updateCollectedTargets prints its own —
	// call update then print extra notFound if needed.
	if err := updateCollectedTargets(targets); err != nil {
		return err
	}
	if notFound > 0 {
		fmt.Printf("(plus %d skill(s) not found / not updatable)\n", notFound)
	}
	return nil
}

// 一次更新的汇总：成功、跳过、失败计数。

type updateSummary struct {
	Updated int
	Skipped int
	Errors  int
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

	// 并发上限：git pull 是 I/O+网络密集型，允许真实并行，
	// 但封顶 8，避免压垮主机或触发远端限流。
	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8
	}
	if workers > len(repos) {
		workers = len(repos)
	}
	if workers < 1 {
		workers = 1
	}

	var (
		mu      sync.Mutex
		results = make([]pullResult, len(repos))
		outMu   sync.Mutex // 序列化进度输出，避免行交错
		wg      sync.WaitGroup
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

		var output []byte
		var err error
		skipped := false
		if detached, detachedErr := gitDetached(repo.path); detachedErr != nil {
			err = detachedErr
		} else if detached {
			skipped = true
		} else if dirty, statusErr := gitDirty(repo.path); statusErr != nil {
			err = statusErr
		} else if dirty {
			err = fmt.Errorf("local changes present; commit or stash them first")
		} else {
			pullCmd := exec.Command("git", "-C", repo.path, "pull", "--ff-only")
			output, err = pullCmd.CombinedOutput()
		}

		ok := err == nil && !skipped
		var afterScore *registry.SkillScore
		commitChanged := ok && beforeHash != "" && gitHeadHash(repo.path) != beforeHash
		rolledBack := false
		if repo.skillRel != "" && ok {
			reg := registry.New(RegistryDir)
			afterScore = reg.ScoreSkill(repo.skillRel)
			if commitChanged && !beforeLintErrors && reg.LintSkill(repo.skillRel).HasErrors() {
				resetOutput, resetErr := exec.Command("git", "-C", repo.path, "reset", "--hard", beforeHash).CombinedOutput()
				if resetErr != nil {
					err = fmt.Errorf("updated skill failed validation; rollback failed: %v: %s", resetErr, resetOutput)
				} else {
					err = fmt.Errorf("updated skill failed validation; rolled back to %s", shortHash(beforeHash))
					rolledBack = true
				}
				ok = false
			}
		}

		outMu.Lock()
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
		outMu.Unlock()

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

	// 用带缓冲的 channel 分发索引，省去独立的分发 goroutine。
	jobs := make(chan int, workers)
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := range jobs {
				pull(i)
			}
		}()
	}
	for i := range repos {
		jobs <- i
	}
	close(jobs)

	wg.Wait()
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

func gitDirty(repoPath string) (bool, error) {
	out, err := exec.Command("git", "-C", repoPath, "status", "--porcelain").Output()
	if err != nil {
		return false, fmt.Errorf("checking local changes: %w", err)
	}
	return len(out) > 0, nil
}

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
	rootCmd.AddCommand(updateCmd)
}

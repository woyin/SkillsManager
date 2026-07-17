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
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
)

var (
	updateGlobal  bool
	updateProject bool
	updateYes     bool
	updateDir     string // --dir: 只更新该项目安装涉及的 registry 源（扫项目 symlinks 反查）
)

var updateCmd = &cobra.Command{
	Use:   "update [skills...]",
	Short: "Update installed skills to latest versions",
	Long: `Update git-managed registry entries to their latest versions.

Without arguments, updates all git-managed entries in the registry.
With skill names, updates only those specific skills.

Examples:
  # Update all skills (interactive scope prompt)
  sm update

  # Update a single skill by name
  sm update my-skill

  # Update multiple specific skills
  sm update frontend-design web-design-guidelines

  # Update only global skills
  sm update -g

  # Update only project skills
  sm update -p

  # Non-interactive (auto-detects scope)
  sm update -y
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if updateDir != "" {
			return updateProjectSources(args)
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
// 扫描各工具在项目根的 ProjectSkillDir，收集指向 registry 内的符号链接，
// 向上找到含 .git 的源目录，去重后只 pull 这些。
func updateProjectSources(names []string) error {
	sources := projectInstalledSources(updateDir)

	// 构建 name 过滤集（O(1) 查找，优于线性扫描）。
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	repos := make([]namedRepo, 0, len(sources))
	for _, src := range sources {
		if len(nameSet) > 0 && !nameSet[filepath.Base(src)] {
			continue
		}
		repos = append(repos, namedRepo{path: src, label: src, skillRel: skillRelFromPath(RegistryDir, src)})
	}

	if len(repos) == 0 {
		fmt.Println("No installed skills found in project; nothing to update")
		return nil
	}

	fmt.Printf("Updating %d source(s) installed in %s\n", len(repos), updateDir)
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
	var repos []namedRepo
	notFound := 0

	for _, name := range names {
		reg := registry.New(RegistryDir)
		skillPath, _ := reg.FindSkillDir(name)
		if skillPath == "" {
			fmt.Fprintf(os.Stderr, "warning: skill %q not found in registry\n", name)
			notFound++
			continue
		}

		gitDir := filepath.Join(skillPath, ".git")
		if _, err := os.Stat(gitDir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: skill %q is not git-managed\n", name)
			continue
		}
		repos = append(repos, namedRepo{path: skillPath, label: name, skillRel: skillRelFromPath(RegistryDir, skillPath)})
	}

	results := pullReposConcurrently(repos)

	updated, skipped := 0, 0
	errors := notFound
	for _, r := range results {
		if r.skipped {
			skipped++
		} else if r.ok {
			updated++
		} else {
			errors++
		}
	}

	fmt.Printf("\nSummary: %d updated, %d pinned, %d errors\n", updated, skipped, errors)
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
	updateCmd.Flags().BoolVarP(&updateGlobal, "global", "g", false, "Only update global skills")
	updateCmd.Flags().BoolVarP(&updateProject, "project", "p", false, "Only update project skills")
	updateCmd.Flags().BoolVarP(&updateYes, "yes", "y", false, "Skip scope prompt (auto-detect)")
	updateCmd.Flags().StringVar(&updateDir, "dir", "",
		"Only update registry sources installed in this project (scan its ./<agent>/skills symlinks)")
	rootCmd.AddCommand(updateCmd)
}

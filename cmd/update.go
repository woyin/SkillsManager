// cmd/update.go 实现 `sm update`：并发 `git pull` 更新注册表中
// 由 git 管理的条目；支持按技能名或 --global/--project 过滤。
// cmd/update.go
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
)

var (
	updateGlobal  bool
	updateProject bool
	updateYes     bool
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
		if len(args) > 0 {
			return updateSpecificSkills(args)
		}
		return updateAllSkills()
	},
}
// 更新注册表中所有 git 管理的条目（并发 git pull）。
// --global/--project 可限定范围。

func updateAllSkills() error {
	dirs := gitRepoDirs(RegistryDir)

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
		repos[i] = namedRepo{path: d, label: d}
	}

	results := pullReposConcurrently(repos)
	var summary updateSummary
	for _, r := range results {
		if r.ok {
			summary.Updated++
		} else {
			summary.Errors++
		}
	}

	fmt.Printf("\nSummary: %d updated, %d errors\n", summary.Updated, summary.Errors)
	return nil
}
// 按名称更新指定技能；非 git 管理的会跳过。

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
		repos = append(repos, namedRepo{path: skillPath, label: name})
	}

	results := pullReposConcurrently(repos)

	updated := 0
	errors := notFound
	for _, r := range results {
		if r.ok {
			updated++
		} else {
			errors++
		}
	}

	fmt.Printf("\nSummary: %d updated, %d errors\n", updated, errors)
	return nil
}
// 一次更新的汇总：成功、跳过、失败计数。

type updateSummary struct {
	Updated int
	Skipped int
	Errors  int
}

// 遍历注册表的 skills/mcp 子树，返回所有含 .git 的目录。
// gitRepoDirs walks the registry's skills and mcp trees and returns the
// absolute path of every directory that contains a .git subdirectory (i.e.
// every git-managed registry entry).
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

// 单次 git pull 的结果。
// pullResult is the outcome of a single git pull.
type pullResult struct {
	label string // human-friendly name (repo dir or skill name)
	ok    bool
}

// 并发执行 `git -C <repo> pull --ff-only`。
// git pull 是网络+I/O 密集型，并发能显著缩短墙上时间；worker 数上限 8 以避免压垮主机或触发远端限流。
// pullReposConcurrently runs `git -C <repo> pull --ff-only` for every repo in
// parallel using a bounded worker pool. git pull is network+I/O bound, so
// concurrency dramatically reduces wall-clock time when many skills are
// git-managed, while bounding workers avoids fork-bombing the host.
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
		outMu   sync.Mutex // serialize progress output so lines don't interleave
		wg      sync.WaitGroup
		jobs    = make(chan int)
	)

	// Worker：按索引从 repos 取一个仓库执行 pull。
	pull := func(i int) {
		repo := repos[i]

		outMu.Lock()
		fmt.Printf("Updating %s ... ", repo.label)
		outMu.Unlock()

		pullCmd := exec.Command("git", "-C", repo.path, "pull", "--ff-only")
		output, err := pullCmd.CombinedOutput()

		ok := err == nil
		outMu.Lock()
		if ok {
			fmt.Println("OK")
		} else {
			fmt.Printf("ERROR: %v\n%s\n", err, string(output))
		}
		outMu.Unlock()

		mu.Lock()
		results[i] = pullResult{label: repo.label, ok: ok}
		mu.Unlock()
	}

	// 启动 worker。
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				pull(i)
			}
		}()
	}

	// 分发任务索引。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range repos {
			jobs <- i
		}
		close(jobs)
	}()

	wg.Wait()
	return results
}

// 把仓库路径与展示标签配对，供并发 puller 使用。
// namedRepo pairs a repo path with a display label for the parallel puller.
type namedRepo struct {
	path  string
	label string
}



// 发现并拉取 registryDir 下所有 git 管理的仓库。
// 导出供测试使用；生产路径走 updateAllSkills（先应用范围过滤）。
// updateGitRepos discovers and pulls every git-managed repo under registryDir.
// Exported for tests; production code paths go through updateAllSkills which
// applies the --global/--project scope filter first.
func updateGitRepos(registryDir string) (updateSummary, error) {
	dirs := gitRepoDirs(registryDir)
	repos := make([]namedRepo, len(dirs))
	for i, d := range dirs {
		repos[i] = namedRepo{path: d, label: d}
	}
	results := pullReposConcurrently(repos)
	var summary updateSummary
	for _, r := range results {
		if r.ok {
			summary.Updated++
		} else {
			summary.Errors++
		}
	}
	return summary, nil
}

func init() {
	updateCmd.Flags().BoolVarP(&updateGlobal, "global", "g", false, "Only update global skills")
	updateCmd.Flags().BoolVarP(&updateProject, "project", "p", false, "Only update project skills")
	updateCmd.Flags().BoolVarP(&updateYes, "yes", "y", false, "Skip scope prompt (auto-detect)")
	rootCmd.AddCommand(updateCmd)
}

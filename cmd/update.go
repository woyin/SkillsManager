// cmd/update.go
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func updateAllSkills() error {
	summary, err := updateGitRepos(RegistryDir)
	if err != nil {
		return fmt.Errorf("walking registry: %w", err)
	}

	fmt.Printf("\nSummary: %d updated, %d skipped, %d errors\n", summary.Updated, summary.Skipped, summary.Errors)
	return nil
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

type updateSummary struct {
	Updated int
	Skipped int
	Errors  int
}

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

// pullResult is the outcome of a single git pull.
type pullResult struct {
	label string // human-friendly name (repo dir or skill name)
	ok    bool
}

// pullReposConcurrently runs `git -C <repo> pull --ff-only` for every repo in
// parallel using a bounded worker pool. git pull is network+I/O bound, so
// concurrency dramatically reduces wall-clock time when many skills are
// git-managed, while bounding workers avoids fork-bombing the host.
func pullReposConcurrently(repos []namedRepo) []pullResult {
	if len(repos) == 0 {
		return nil
	}

	// Bound concurrency: git pull is I/O+network heavy, so allow real parallelism,
	// but cap at 8 to avoid overwhelming the host or tripping remote rate limits.
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

	// Worker: pull one repo identified by its index into repos.
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

	// Launch workers.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				pull(i)
			}
		}()
	}

	// Dispatch job indices.
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

// namedRepo pairs a repo path with a display label for the parallel puller.
type namedRepo struct {
	path  string
	label string
}

func updateGitRepos(registryDir string) (updateSummary, error) {
	var summary updateSummary

	dirs := gitRepoDirs(registryDir)
	repos := make([]namedRepo, len(dirs))
	for i, d := range dirs {
		repos[i] = namedRepo{path: d, label: d}
	}

	results := pullReposConcurrently(repos)
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

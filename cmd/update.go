// cmd/update.go
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
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
	updated := 0
	errors := 0

	for _, name := range names {
		skillPath := findSkillInRegistry(name)
		if skillPath == "" {
			fmt.Fprintf(os.Stderr, "warning: skill %q not found in registry\n", name)
			errors++
			continue
		}

		gitDir := filepath.Join(skillPath, ".git")
		if _, err := os.Stat(gitDir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: skill %q is not git-managed\n", name)
			continue
		}

		fmt.Printf("Updating %s ... ", name)
		pullCmd := exec.Command("git", "-C", skillPath, "pull", "--ff-only")
		output, err := pullCmd.CombinedOutput()
		if err != nil {
			fmt.Printf("ERROR: %v\n%s\n", err, string(output))
			errors++
		} else {
			fmt.Println("OK")
			updated++
		}
	}

	fmt.Printf("\nSummary: %d updated, %d errors\n", updated, errors)
	return nil
}

func findSkillInRegistry(name string) string {
	skillsDir := filepath.Join(RegistryDir, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return ""
	}
	for _, cat := range entries {
		if !cat.IsDir() {
			continue
		}
		candidate := filepath.Join(skillsDir, cat.Name(), name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

type updateSummary struct {
	Updated int
	Skipped int
	Errors  int
}

func updateGitRepos(registryDir string) (updateSummary, error) {
	var summary updateSummary

	for _, root := range []string{
		filepath.Join(registryDir, "skills"),
		filepath.Join(registryDir, "mcp"),
	} {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() && info.Name() == ".git" {
				repoDir := filepath.Dir(path)
				fmt.Printf("Updating %s ... ", repoDir)

				pullCmd := exec.Command("git", "-C", repoDir, "pull", "--ff-only")
				output, err := pullCmd.CombinedOutput()
				if err != nil {
					fmt.Printf("ERROR: %v\n%s\n", err, string(output))
					summary.Errors++
				} else {
					fmt.Println("OK")
					summary.Updated++
				}
				return filepath.SkipDir
			}
			return nil
		})
		if err != nil {
			return summary, err
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

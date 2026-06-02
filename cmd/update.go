// cmd/update.go
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update all git-managed registry entries to latest",
	Long: `Walk the registry directory, find all entries with .git,
and run git pull --ff-only on each.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		skillsDir := filepath.Join(RegistryDir, "skills")
		updated := 0
		skipped := 0
		errors := 0

		err := filepath.Walk(skillsDir, func(path string, info os.FileInfo, err error) error {
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
					errors++
				} else {
					fmt.Println("OK")
					updated++
				}
				return filepath.SkipDir
			}
			return nil
		})

		if err != nil {
			return fmt.Errorf("walking registry: %w", err)
		}

		fmt.Printf("\nSummary: %d updated, %d skipped, %d errors\n", updated, skipped, errors)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

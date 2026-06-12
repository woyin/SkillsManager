// cmd/init.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/project"
)

var initProfile string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a project with .sm.json",
	Long: `Create a .sm.json configuration file in the current project directory.
Optionally set a profile to use as the base.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}

		pm := project.NewManager(projectDir)
		config, err := pm.Load()
		if err != nil {
			return fmt.Errorf("loading existing config: %w", err)
		}

		if config.Profile != "" || len(config.Skills) > 0 || len(config.MCP) > 0 {
			return fmt.Errorf(".sm.json already exists in %s", projectDir)
		}

		config.Profile = initProfile
		if err := pm.Save(config); err != nil {
			return fmt.Errorf("writing .sm.json: %w", err)
		}

		fmt.Printf("✓ Initialized .sm.json in %s\n", projectDir)
		if initProfile != "" {
			fmt.Printf("  Profile: %s\n", initProfile)
		}
		fmt.Println("  Run 'sm install' to install skills")
		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&initProfile, "profile", "", "Profile name to use as base")
	rootCmd.AddCommand(initCmd)
}

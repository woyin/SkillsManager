// cmd/install.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/db"
	"github.com/woyin/skills-manager/internal/installer"
	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/tool"
)

var (
	installProfile string
	installDir     string
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install skills and MCP into the current project",
	Long: `Install skills and MCP configurations into a project directory.
Reads .sm.json if present, or uses --profile flag.
Creates symlinks in tool-specific skills directories.
Writes .mcp.json for MCP server configurations.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := installDir
		if projectDir == "" {
			var err error
			projectDir, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
		}

		// Load existing project config
		pm := project.NewManager(projectDir)
		config, err := pm.Load()
		if err != nil {
			return fmt.Errorf("loading project config: %w", err)
		}

		profileName := installProfile
		if profileName == "" {
			profileName = config.Profile
		}

		extraSkills := config.Skills
		extraMCP := config.MCP

		if profileName == "" && len(extraSkills) == 0 && len(extraMCP) == 0 {
			return fmt.Errorf("nothing to install: create .sm.json with a profile, or use --profile flag")
		}

		// Detect installed tools
		tools := tool.DetectInstalled(tool.AllTools())
		if len(tools) == 0 {
			// Fallback to default tools if none detected
			tools = tool.DefaultTools()
		}

		inst, err := installer.New(RegistryDir, ProfilesDir, tools)
		if err != nil {
			return fmt.Errorf("creating installer: %w", err)
		}

		result, err := inst.Install(projectDir, profileName, extraSkills, extraMCP)
		if err != nil {
			return fmt.Errorf("install failed: %w", err)
		}

		// Record in database
		dbPath := filepath.Join(DataDir, "sm.db")
		database, err := db.Open(dbPath)
		if err != nil {
			return fmt.Errorf("opening database: %w", err)
		}
		defer database.Close()

		if err := database.RecordInstallation(projectDir, profileName, result.Skills, result.MCP); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to record installation: %v\n", err)
		}

		if err := database.UpsertProject(projectDir, profileName, extraSkills, extraMCP); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to update project record: %v\n", err)
		}

		// Summary
		fmt.Printf("✓ Installed to %s\n", projectDir)
		if profileName != "" {
			fmt.Printf("  Profile: %s\n", profileName)
		}
		if len(result.Skills) > 0 {
			fmt.Printf("  Skills: %d symlinks created\n", len(result.Skills))
			for _, s := range result.Skills {
				fmt.Printf("    → %s\n", s)
			}
		}
		if len(result.MCP) > 0 {
			fmt.Printf("  MCP: %v\n", result.MCP)
		}

		return nil
	},
}

func init() {
	installCmd.Flags().StringVar(&installProfile, "profile", "", "Profile name to install")
	installCmd.Flags().StringVar(&installDir, "dir", "", "Project directory (default: current dir)")

	rootCmd.AddCommand(installCmd)
}

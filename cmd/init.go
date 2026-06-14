// cmd/init.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/project"
)

var (
	initProfile   string
	initSkillName string
)

var initCmd = &cobra.Command{
	Use:   "init [name]",
	Short: "Initialize a project or create a new skill template",
	Long: `Two modes of operation:

1. Without arguments: Initialize a project with a .sm.json configuration file.
   Optionally set a profile to use as the base.

2. With a name argument: Create a new SKILL.md template in a subdirectory.
   This creates a skill scaffold compatible with the Agent Skills specification.

Examples:
  # Initialize project config
  sm init
  sm init --profile cloudflare

  # Create a new skill template
  sm init my-skill
  sm init my-skill --description "My custom skill"
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return initSkillTemplate(args[0])
		}
		return initProject()
	},
}

func initProject() error {
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
}

func initSkillTemplate(name string) error {
	// Sanitize name: lowercase, hyphens
	sanitized := strings.ToLower(name)
	sanitized = strings.ReplaceAll(sanitized, " ", "-")

	// Determine directory
	dir := name
	if initSkillName != "" {
		dir = initSkillName
	}

	skillDir := filepath.Join(".", dir)
	skillMD := filepath.Join(skillDir, "SKILL.md")

	// Check if already exists
	if _, err := os.Stat(skillMD); err == nil {
		return fmt.Errorf("SKILL.md already exists in %s", skillDir)
	}

	// Create directory
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	// Generate SKILL.md content
	description := fmt.Sprintf("Instructions for %s", name)
	content := fmt.Sprintf(`---
name: %s
description: %s
---

# %s

Instructions for the agent to follow when this skill is activated.

## When to Use

Describe the scenarios where this skill should be used.

## Steps

1. First, do this
2. Then, do that
3. Finally, verify the result
`, sanitized, description, name)

	if err := os.WriteFile(skillMD, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing SKILL.md: %w", err)
	}

	fmt.Printf("✓ Created skill template: %s\n", skillMD)
	fmt.Println("  Edit the SKILL.md file to add your skill instructions.")
	fmt.Println("  Share it by pushing to a git repository.")
	return nil
}

func init() {
	initCmd.Flags().StringVar(&initProfile, "profile", "", "Profile name to use as base (project mode)")
	initCmd.Flags().StringVar(&initSkillName, "dir", "", "Directory name for skill template (default: skill name)")
	rootCmd.AddCommand(initCmd)
}

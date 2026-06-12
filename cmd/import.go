// cmd/import.go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/db"
	"github.com/woyin/skills-manager/internal/profile"
	"github.com/woyin/skills-manager/internal/prompt"
	"github.com/woyin/skills-manager/internal/registry"
)

var (
	importDryRun  bool
	importMerge   bool
	importReplace bool
)

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import configuration from JSON",
	Long: `Import SkillsManager configuration from a JSON file.
Supports merge (default) or replace mode.
Use --replace to clear existing data before importing, or --merge (default) to add alongside existing entries.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]

		if importReplace && cmd.Flags().Changed("merge") {
			return fmt.Errorf("--replace and --merge are mutually exclusive")
		}
		if importReplace {
			importMerge = false
		}

		// Read file
		var reader io.Reader
		if filePath == "-" {
			reader = os.Stdin
		} else {
			file, err := os.Open(filePath)
			if err != nil {
				return fmt.Errorf("opening file: %w", err)
			}
			defer file.Close()
			reader = file
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}

		var exportData ExportData
		if err := json.Unmarshal(data, &exportData); err != nil {
			return fmt.Errorf("parsing JSON: %w", err)
		}

		if importDryRun {
			return printImportPreview(&exportData)
		}

		return performImport(&exportData)
	},
}

func printImportPreview(data *ExportData) error {
	fmt.Println("Import Preview")
	fmt.Println("==============")

	if len(data.Skills) > 0 {
		fmt.Printf("\nSkills:\n")
		for category, skills := range data.Skills {
			for _, s := range skills {
				fmt.Printf("  - %s/%s\n", category, s.Name)
			}
		}
	}

	if len(data.MCP) > 0 {
		fmt.Printf("\nMCP Servers:\n")
		for _, m := range data.MCP {
			fmt.Printf("  - %s\n", m.Name)
		}
	}

	if len(data.Profiles) > 0 {
		fmt.Printf("\nProfiles:\n")
		for name := range data.Profiles {
			fmt.Printf("  - %s\n", name)
		}
	}

	if len(data.Prompts) > 0 {
		fmt.Printf("\nPrompt Sets:\n")
		for name := range data.Prompts {
			fmt.Printf("  - %s\n", name)
		}
	}

	if len(data.Projects) > 0 {
		fmt.Printf("\nProjects:\n")
		for _, p := range data.Projects {
			fmt.Printf("  - %s (profile: %s)\n", p.Path, p.Profile)
		}
	}

	fmt.Println("\nRun without --dry-run to apply changes.")
	return nil
}

func performImport(data *ExportData) error {
	replace := importReplace

	// Import skills
	if len(data.Skills) > 0 {
		reg := registry.New(RegistryDir)

		if replace {
			// Remove existing skills before importing
			existing, _ := reg.ListSkillDetails()
			for category, skills := range existing {
				for _, s := range skills {
					if err := reg.RemoveSkill(s.Name, category, ""); err != nil {
						fmt.Fprintf(os.Stderr, "warning: removing skill %s/%s: %v\n", category, s.Name, err)
					}
				}
			}
		}

		for category, skills := range data.Skills {
			for _, s := range skills {
				if s.SourceURL == "" {
					continue // Skip skills without source URL
				}
				if err := reg.AddSkill(s.SourceURL, category, ""); err != nil {
					if !importMerge {
						return fmt.Errorf("adding skill %s/%s: %w", category, s.Name, err)
					}
					fmt.Fprintf(os.Stderr, "warning: skipping skill %s/%s: %v\n", category, s.Name, err)
				}
			}
		}
	}

	// Import MCP
	if len(data.MCP) > 0 {
		reg := registry.New(RegistryDir)

		if replace {
			existing, _ := reg.ListMCPDetails()
			for _, m := range existing {
				if err := reg.RemoveMCP(m.Name); err != nil {
					fmt.Fprintf(os.Stderr, "warning: removing MCP %s: %v\n", m.Name, err)
				}
			}
		}

		for _, m := range data.MCP {
			if m.SourceURL == "" {
				continue // Skip MCP without source URL
			}
			if err := reg.AddMCP(m.SourceURL); err != nil {
				if !importMerge {
					return fmt.Errorf("adding MCP %s: %w", m.Name, err)
				}
				fmt.Fprintf(os.Stderr, "warning: skipping MCP %s: %v\n", m.Name, err)
			}
		}
	}

	// Import profiles
	if len(data.Profiles) > 0 {
		loader := profile.NewLoader(ProfilesDir)

		if replace {
			existing, _ := loader.List()
			for _, name := range existing {
				if err := loader.Delete(name); err != nil {
					fmt.Fprintf(os.Stderr, "warning: removing profile %s: %v\n", name, err)
				}
			}
		}

		for name, config := range data.Profiles {
			if err := loader.Save(name, config); err != nil {
				if !importMerge {
					return fmt.Errorf("saving profile %s: %w", name, err)
				}
				fmt.Fprintf(os.Stderr, "warning: skipping profile %s: %v\n", name, err)
			}
		}
	}

	// Import prompt sets
	if len(data.Prompts) > 0 {
		manager := prompt.NewManager(filepath.Join(RegistryDir, "prompts"))

		if replace {
			existing, _ := manager.List()
			for _, name := range existing {
				if err := manager.Delete(name); err != nil {
					fmt.Fprintf(os.Stderr, "warning: removing prompt set %s: %v\n", name, err)
				}
			}
		}

		for name, ps := range data.Prompts {
			if ps == nil {
				continue
			}
			if ps.Name == "" {
				ps.Name = name
			}
			if err := manager.Save(ps); err != nil {
				if !importMerge {
					return fmt.Errorf("saving prompt set %s: %w", name, err)
				}
				fmt.Fprintf(os.Stderr, "warning: skipping prompt set %s: %v\n", name, err)
			}
		}
	}

	// Import projects
	if len(data.Projects) > 0 {
		dbPath := filepath.Join(DataDir, "sm.db")
		database, err := db.Open(dbPath)
		if err != nil {
			return fmt.Errorf("opening database: %w", err)
		}
		defer database.Close()

		if replace {
			existing, _ := database.GetAllProjects()
			for _, p := range existing {
				if err := database.RemoveProject(p.Path); err != nil {
					fmt.Fprintf(os.Stderr, "warning: removing project %s: %v\n", p.Path, err)
				}
			}
		}

		for _, p := range data.Projects {
			if err := database.UpsertProject(p.Path, p.Profile, p.ExtraSkills, p.ExtraMCP); err != nil {
				if !importMerge {
					return fmt.Errorf("importing project %s: %w", p.Path, err)
				}
				fmt.Fprintf(os.Stderr, "warning: skipping project %s: %v\n", p.Path, err)
			}
		}
	}

	fmt.Println("✓ Import completed successfully")
	return nil
}

func init() {
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "Show what would be imported without making changes")
	importCmd.Flags().BoolVar(&importMerge, "merge", true, "Merge with existing data (default)")
	importCmd.Flags().BoolVar(&importReplace, "replace", false, "Replace existing data (clear everything first)")

	rootCmd.AddCommand(importCmd)
}

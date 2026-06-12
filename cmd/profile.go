// cmd/profile.go
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/profile"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage skill profiles",
	Long:  `List, show, create, and delete skill profiles. Profiles bundle skills and MCP configs for scenarios.`,
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := profile.NewLoader(ProfilesDir)
		names, err := loader.List()
		if err != nil {
			return fmt.Errorf("listing profiles: %w", err)
		}
		if len(names) == 0 {
			fmt.Println("No profiles found")
			return nil
		}
		fmt.Println("Available profiles:")
		for _, name := range names {
			fmt.Printf("  - %s\n", name)
		}
		return nil
	},
}

var profileShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show profile contents",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := profile.NewLoader(ProfilesDir)
		p, err := loader.Load(args[0])
		if err != nil {
			return err
		}
		fmt.Printf("Profile: %s\n", args[0])
		fmt.Printf("  Skills: %s\n", formatList(p.Skills))
		fmt.Printf("  MCP:    %s\n", formatList(p.MCP))
		return nil
	},
}

var profileCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new profile",
	Long:  `Create a new profile with the given skills and MCP servers.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := profile.NewLoader(ProfilesDir)
		p := &profile.Profile{}
		if profileCreateSkills != "" {
			p.Skills = splitAndTrim(profileCreateSkills)
		}
		if profileCreateMCP != "" {
			p.MCP = splitAndTrim(profileCreateMCP)
		}
		if err := loader.Save(args[0], p); err != nil {
			return fmt.Errorf("creating profile: %w", err)
		}
		fmt.Printf("✓ Created profile %q\n", args[0])
		return nil
	},
}

var profileDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := profile.NewLoader(ProfilesDir)
		if err := loader.Delete(args[0]); err != nil {
			return fmt.Errorf("deleting profile: %w", err)
		}
		fmt.Printf("✓ Deleted profile %q\n", args[0])
		return nil
	},
}

var (
	profileCreateSkills string
	profileCreateMCP    string
)

func init() {
	profileCreateCmd.Flags().StringVar(&profileCreateSkills, "skills", "", "Comma-separated list of skills")
	profileCreateCmd.Flags().StringVar(&profileCreateMCP, "mcp", "", "Comma-separated list of MCP servers")

	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileShowCmd)
	profileCmd.AddCommand(profileCreateCmd)
	profileCmd.AddCommand(profileDeleteCmd)

	rootCmd.AddCommand(profileCmd)
}

func formatList(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	return strings.Join(items, ", ")
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// cmd/profile.go 实现 `sm profile` 子命令：list/show/create/delete
// 管理 skills + MCP 的命名预设。
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
	Long:  `List, show, create, update, and delete skill profiles. Profiles bundle skills and MCP configs for scenarios.`,
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

var profileUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Update an existing profile's skills or MCP servers",
	Long: `Update an existing profile. Only the fields whose flags are passed
are overwritten; omitted flags keep their existing values (so
--mcp x does not clear skills, and --skills a,b does not clear mcp).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := profile.NewLoader(ProfilesDir)
		p, err := loader.Load(args[0])
		if err != nil {
			return fmt.Errorf("loading profile: %w", err)
		}
		// 只覆盖显式传入的字段；未传的保留原值，避免清空。
		if cmd.Flags().Changed("skills") {
			p.Skills = splitAndTrim(profileUpdateSkills)
		}
		if cmd.Flags().Changed("mcp") {
			p.MCP = splitAndTrim(profileUpdateMCP)
		}
		if err := loader.Save(args[0], p); err != nil {
			return fmt.Errorf("updating profile: %w", err)
		}
		fmt.Printf("✓ Updated profile %q\n", args[0])
		fmt.Printf("  Skills: %s\n", formatList(p.Skills))
		fmt.Printf("  MCP:    %s\n", formatList(p.MCP))
		return nil
	},
}

var (
	profileCreateSkills string
	profileCreateMCP    string
	profileUpdateSkills string
	profileUpdateMCP    string
)

func init() {
	profileCreateCmd.Flags().StringVar(&profileCreateSkills, "skills", "", "Comma-separated list of skills")
	profileCreateCmd.Flags().StringVar(&profileCreateMCP, "mcp", "", "Comma-separated list of MCP servers")
	profileUpdateCmd.Flags().StringVar(&profileUpdateSkills, "skills", "", "Comma-separated list of skills (overwrites)")
	profileUpdateCmd.Flags().StringVar(&profileUpdateMCP, "mcp", "", "Comma-separated list of MCP servers (overwrites)")

	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileShowCmd)
	profileCmd.AddCommand(profileCreateCmd)
	profileCmd.AddCommand(profileUpdateCmd)
	profileCmd.AddCommand(profileDeleteCmd)

	rootCmd.AddCommand(profileCmd)
}

// 把字符串切片格式化为逗号分隔的单一字符串；空切片返回 "(none)"。

func formatList(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	return strings.Join(items, ", ")
}

// 按逗号切分并去除空白，丢弃空段。

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

// cmd/profile.go 实现 `sm profile` 子命令：list/show/create/update/delete
// 管理 skills + MCP 的命名预设。create/update 保存前用 ValidateMembers 校验
// 所有引用存在且唯一（ADR 0012），失败不改写旧 Profile。
//
// Input: fmt, strings, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/profile
// Output: var profileCmd, var profileListCmd, var profileShowCmd, var profileCreateCmd, var profileDeleteCmd, var profileUpdateCmd, func formatList, func splitAndTrim
// Pos: 控制层-profile命令实现
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/profile"
	"github.com/woyin/skills-manager/internal/registry"
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
		// ADR 0012: 保存前验证所有引用存在且唯一；失败不改旧文件。
		if err := p.ValidateMembers(registrySkillExists, registryMCPExists); err != nil {
			return fmt.Errorf("validating profile: %w", err)
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
		// ADR 0012: 保存前验证所有引用存在且唯一；失败不改旧文件。
		if err := p.ValidateMembers(registrySkillExists, registryMCPExists); err != nil {
			return fmt.Errorf("validating profile: %w", err)
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

// registrySkillExists 是 SkillExistenceChecker：用 ResolveUniqueSkill 验证
// skill 名在 Registry 中存在且唯一（ADR 0010/0012）。
func registrySkillExists(name string) error {
	reg := registry.New(RegistryDir)
	if _, err := reg.ResolveUniqueSkill(name); err != nil {
		return err
	}
	return nil
}

// registryMCPExists 是 MCPExistenceChecker：验证 MCP 名在 Registry 中存在。
func registryMCPExists(name string) error {
	reg := registry.New(RegistryDir)
	if _, err := os.Stat(reg.GetMCPPath(name)); err != nil {
		return fmt.Errorf("MCP %q not found in registry", name)
	}
	return nil
}

// cmd/prompt.go 实现 `sm prompt` 子命令：list/show/apply/create/delete
// 管理面向不同 AI 助手的提示词集合。
// cmd/prompt.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/prompt"
)

var promptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Manage prompt sets",
	Long:  `Manage prompt sets for different AI coding assistants.`,
}

var promptListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available prompt sets",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := filepath.Join(RegistryDir, "prompts")
		m := prompt.NewManager(dir)

		names, err := m.List()
		if err != nil {
			return err
		}

		if len(names) == 0 {
			fmt.Println("No prompt sets found")
			return nil
		}

		fmt.Println("Available prompt sets:")
		for _, name := range names {
			fmt.Printf("  - %s\n", name)
		}
		return nil
	},
}

var promptShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show contents of a prompt set",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := filepath.Join(RegistryDir, "prompts")
		m := prompt.NewManager(dir)

		ps, err := m.Load(args[0])
		if err != nil {
			return err
		}

		fmt.Printf("Prompt set: %s\n", ps.Name)
		fmt.Println(strings.Repeat("-", 40))
		for filename, content := range ps.Prompts {
			fmt.Printf("\n=== %s ===\n", filename)
			fmt.Println(content)
		}
		return nil
	},
}

var promptApplyCmd = &cobra.Command{
	Use:   "apply <name>",
	Short: "Apply a prompt set to a project",
	Long: `Apply a prompt set to a project directory.
Writes prompt files (CLAUDE.md, AGENTS.md, etc.) to the project.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := filepath.Join(RegistryDir, "prompts")
		m := prompt.NewManager(dir)

		projectDir := promptApplyDir
		if projectDir == "" {
			var err error
			projectDir, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
		}

		var tools []string
		if promptApplyTools != "" {
			tools = strings.Split(promptApplyTools, ",")
		}

		if err := m.Apply(projectDir, args[0], tools); err != nil {
			return err
		}

		fmt.Printf("✓ Applied prompt set %q to %s\n", args[0], projectDir)
		return nil
	},
}

var promptCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new prompt set from current project",
	Long: `Create a new prompt set from existing prompt files in a project.
Reads CLAUDE.md, AGENTS.md, GEMINI.md, etc. from the project directory.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := filepath.Join(RegistryDir, "prompts")
		m := prompt.NewManager(dir)

		projectDir := promptCreateDir
		if projectDir == "" {
			var err error
			projectDir, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
		}

		var filenames []string
		if promptCreateFiles != "" {
			filenames = strings.Split(promptCreateFiles, ",")
		}

		if err := m.CreateFromProject(projectDir, args[0], filenames); err != nil {
			return err
		}

		fmt.Printf("✓ Created prompt set %q from %s\n", args[0], projectDir)
		return nil
	},
}

var promptDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a prompt set",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := filepath.Join(RegistryDir, "prompts")
		m := prompt.NewManager(dir)

		if err := m.Delete(args[0]); err != nil {
			return err
		}

		fmt.Printf("✓ Deleted prompt set %q\n", args[0])
		return nil
	},
}

var (
	promptApplyDir    string
	promptApplyTools  string
	promptCreateDir   string
	promptCreateFiles string
)

func init() {
	promptApplyCmd.Flags().StringVar(&promptApplyDir, "dir", "", "Project directory (default: current dir)")
	promptApplyCmd.Flags().StringVar(&promptApplyTools, "tools", "", "Comma-separated list of prompt files to apply (default: all)")

	promptCreateCmd.Flags().StringVar(&promptCreateDir, "dir", "", "Project directory (default: current dir)")
	promptCreateCmd.Flags().StringVar(&promptCreateFiles, "files", "", "Comma-separated list of prompt files to include (default: CLAUDE.md,AGENTS.md,GEMINI.md)")

	promptCmd.AddCommand(promptListCmd)
	promptCmd.AddCommand(promptShowCmd)
	promptCmd.AddCommand(promptApplyCmd)
	promptCmd.AddCommand(promptCreateCmd)
	promptCmd.AddCommand(promptDeleteCmd)

	rootCmd.AddCommand(promptCmd)
}

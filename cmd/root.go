package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	RegistryDir string
	DataDir     string
	ProfilesDir string
	Version     = "dev"
)

var rootCmd = &cobra.Command{
	Use:   "sm",
	Short: "SkillsManager — manage AI agent skills and MCP configurations",
	Long:  "A CLI tool for managing skills (Codex, Claude) and MCP server configurations across projects using symlinks and profiles.",
}

func init() {
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, "Documents", "DevelopmentRepository", "SkillsManager")

	rootCmd.PersistentFlags().StringVar(&RegistryDir, "registry", filepath.Join(base, "registry"), "Registry directory path")
	rootCmd.PersistentFlags().StringVar(&DataDir, "data", filepath.Join(base, "data"), "Data directory path")
	rootCmd.PersistentFlags().StringVar(&ProfilesDir, "profiles", filepath.Join(base, "profiles"), "Profiles directory path")

	rootCmd.Version = Version
}

func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}

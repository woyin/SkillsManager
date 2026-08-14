// cmd/root.go 定义 sm 的根命令与全局持久化标志（--registry、--data、--profiles）。
// 这些目录默认位于 ~/.sm/ 下，可被用户覆盖。
//
// Input: fmt, os, path/filepath, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/home
// Output: var RegistryDir, var DataDir, var ProfilesDir, var Version, func Execute
// Pos: 控制层-根命令与全局标志
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/home"
)

var (
	RegistryDir string
	DataDir     string
	ProfilesDir string
	Version     = "0.3.1-dev"
)

var rootCmd = &cobra.Command{
	Use:   "sm",
	Short: "SkillsManager — manage AI agent skills and MCP configurations",
	Long:  "A CLI tool for managing AI agent skills (Codex, Claude, Gemini, OpenCode, Hermes, OpenClaw) and MCP server configurations across projects, built around a cross-project personal Skill Registry (sm add → sm install <name> → sm update).",
}

func init() {
	base := filepath.Join(home.Dir(), ".sm")

	rootCmd.PersistentFlags().StringVar(&RegistryDir, "registry", filepath.Join(base, "registry"), "Registry directory path")
	rootCmd.PersistentFlags().StringVar(&DataDir, "data", filepath.Join(base, "data"), "Data directory path")
	rootCmd.PersistentFlags().StringVar(&ProfilesDir, "profiles", filepath.Join(base, "profiles"), "Profiles directory path")

	rootCmd.Version = Version
}

// Execute runs the root cobra command and is the entry point for sm. It prints
// any error to stderr and returns it so main can set the exit code.
func Execute() error {
	if err := home.Init(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}

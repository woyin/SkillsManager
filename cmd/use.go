// cmd/use.go 实现 `sm use`：临时使用一个技能（不加入注册表）。
// 解析来源、读取 SKILL.md，打印 prompt 或直接启动指定代理。
package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	useSkill string
	useAgent string
)

var useCmd = &cobra.Command{
	Use:   "use <source>",
	Short: "Use a skill without installing it",
	Long: `Use a skill temporarily without adding it to the registry.

Resolves the source (same formats as 'sm add'), writes the selected skill
files to a temporary directory, and either prints a generated prompt to stdout
or starts a supported agent interactively with the skill loaded.

Examples:
  # Print skill prompt to stdout (pipe to an agent)
  sm use owner/repo --skill my-skill | claude

  # Start an agent interactively with the skill
  sm use owner/repo --skill my-skill --agent claude-code

  # Use a local skill directory
  sm use ./my-skill
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		source := args[0]
		return runUse(source)
	},
}

func runUse(source string) error {
	var (
		skillDir string
		tmpDir   string
	)
	defer registry.RemoveCloneTemp(tmpDir)

	if registry.IsGitURL(source) {
		// 把来源克隆到临时目录。
		cloneDest, td, err := registry.CloneToTemp(source, "sm-use-*")
		if err != nil {
			return err
		}
		tmpDir = td

		_, _, subPath, _ := registry.ParseTreeURL(source)
		if subPath != "" {
			skillDir = filepath.Join(cloneDest, subPath)
		} else if useSkill != "" {
			// 查找指定技能
			discovered, err := registry.DiscoverSkills(cloneDest)
			if err != nil {
				return fmt.Errorf("discovering skills: %w", err)
			}
			for _, s := range discovered {
				if s.Name == useSkill {
					skillDir = s.Path
					break
				}
			}
			if skillDir == "" {
				return fmt.Errorf("skill %q not found in source", useSkill)
			}
		} else {
			// 使用首个发现的技能，否则用仓库根
			discovered, err := registry.DiscoverSkills(cloneDest)
			if err != nil {
				return fmt.Errorf("discovering skills: %w", err)
			}
			if len(discovered) == 1 {
				skillDir = discovered[0].Path
			} else if len(discovered) > 1 {
				fmt.Fprintln(os.Stderr, "Multiple skills found. Use --skill to select one:")
				for _, s := range discovered {
					fmt.Fprintf(os.Stderr, "  - %s", s.Name)
					if s.Description != "" {
						fmt.Fprintf(os.Stderr, ": %s", s.Description)
					}
					fmt.Fprintln(os.Stderr)
				}
				return fmt.Errorf("multiple skills found, use --skill to select")
			} else {
				skillDir = cloneDest
			}
		}
	} else {
		// 本地路径
		skillDir = source
		if useSkill != "" {
			discovered, err := registry.DiscoverSkills(source)
			if err != nil {
				return fmt.Errorf("discovering skills: %w", err)
			}
			found := false
			for _, s := range discovered {
				if s.Name == useSkill {
					skillDir = s.Path
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("skill %q not found in %s", useSkill, source)
			}
		}
	}

	// 读取 SKILL.md 内容
	skillMD := filepath.Join(skillDir, "SKILL.md")
	content, err := os.ReadFile(skillMD)
	if err != nil {
		// 兜底：读取任意一个 .md 文件
		entries, _ := os.ReadDir(skillDir)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".md") {
				skillMD = filepath.Join(skillDir, e.Name())
				content, err = os.ReadFile(skillMD)
				break
			}
		}
		if err != nil {
			return fmt.Errorf("no SKILL.md found in %s", skillDir)
		}
	}

	prompt := string(content)

	if useAgent != "" {
		// 用 prompt 启动代理
		return startAgent(useAgent, prompt, tmpDir)
	}

	// 把 prompt 打印到 stdout
	fmt.Print(prompt)
	return nil
}

// 用 prompt 启动指定代理：写入临时 prompt 文件并以 --prompt 参数调用代理二进制。

func startAgent(agentName, prompt, tmpDir string) error {
	t := tool.ToolByAgentName(agentName)
	if t == nil {
		t = tool.ToolByName(agentName)
	}
	if t == nil {
		return fmt.Errorf("unknown agent: %s", agentName)
	}

	if t.Binary == "" {
		return fmt.Errorf("agent %q has no CLI binary configured", agentName)
	}

	// 当来源是本地路径时 tmpDir 为空；此处分配一个临时目录，
	// 避免把 prompt 文件落到用户当前工作目录。
	if tmpDir == "" {
		td, err := os.MkdirTemp("", "sm-use-*")
		if err != nil {
			return fmt.Errorf("creating temp dir: %w", err)
		}
		defer os.RemoveAll(td)
		tmpDir = td
	}

	// 把 prompt 写入临时文件
	promptFile := filepath.Join(tmpDir, "prompt.md")
	if err := os.WriteFile(promptFile, []byte(prompt), 0644); err != nil {
		return fmt.Errorf("writing prompt file: %w", err)
	}

	// 以临时 prompt 文件为输入启动代理
	cmd := exec.Command(t.Binary, "--prompt", promptFile)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func init() {
	useCmd.Flags().StringVarP(&useSkill, "skill", "s", "", "Specific skill to use")
	useCmd.Flags().StringVarP(&useAgent, "agent", "a", "", "Start an agent interactively")
	rootCmd.AddCommand(useCmd)
}

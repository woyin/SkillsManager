// cmd/use.go 实现 `sm use`：临时使用一个技能（不加入注册表）。
// 解析来源、读取 SKILL.md，打印 prompt 或直接启动指定代理。
//
// Input: fmt, os, os/exec, path/filepath, strings, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/internal/tool
// Output: var useCmd, func runUse, func startAgent
// Pos: 控制层-use命令实现
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/lockfile"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

var (
	useSkill               string
	useAgent               string
	useAcceptOpenClawRisks bool
	useFullDepth           bool
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
		parsed := registry.ParseSource(lockfile.ResolveAlias(args[0]))
		source := parsed.Source()
		if parsed.SkillFilter != "" && useSkill == "" {
			useSkill = parsed.SkillFilter
		}
		if err := checkOpenClawRisk(source); err != nil {
			return err
		}
		return runUse(source)
	},
}

// checkOpenClawRisk gates skills from the "openclaw" GitHub owner behind an
// explicit --dangerously-accept-openclaw-risks flag, matching npx skills.
func checkOpenClawRisk(source string) error {
	lower := strings.ToLower(source)
	isOpenClaw := strings.HasPrefix(lower, "openclaw/") ||
		strings.Contains(lower, "github.com/openclaw/")
	if isOpenClaw && !useAcceptOpenClawRisks {
		msg := "OpenClaw skills are unverified community submissions.\n" +
			"Skills run with full agent permissions and could be malicious.\n" +
			"If you understand the risks, re-run with: sm use " + source + " --dangerously-accept-openclaw-risks"
		return fmt.Errorf("%s", msg)
	}
	return nil
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
		subPath = registry.SanitizeSubpath(subPath)
		if subPath != "" {
			skillDir = filepath.Join(cloneDest, subPath)
		} else if useSkill != "" {
			// 查找指定技能
			s, err := findSkillByName(cloneDest, useSkill)
			if err != nil {
				return err
			}
			skillDir = s
		} else {
			// 使用首个发现的技能，否则用仓库根
			s, err := selectSingleSkill(cloneDest)
			if err != nil {
				return err
			}
			skillDir = s
		}
	} else {
		// 本地路径
		skillDir = source
		if useSkill != "" {
			s, err := findSkillByName(source, useSkill)
			if err != nil {
				return err
			}
			skillDir = s
		}
	}

	// 读取 SKILL.md 内容
	skillMD := filepath.Join(skillDir, "SKILL.md")
	// 物化技能目录（含支持文件）到临时目录，并读取 SKILL.md 内容。
	skillMD, supportDir, hasSupport, err := materializeSkill(skillDir, &tmpDir, source)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(skillMD)
	if err != nil {
		return fmt.Errorf("no SKILL.md found in %s", skillDir)
	}

	prompt := buildUsePrompt(string(content), supportDir, hasSupport)

	if useAgent != "" {
		// 用 prompt 启动代理
		return startAgent(useAgent, prompt, tmpDir)
	}

	// 把 prompt 打印到 stdout
	fmt.Print(prompt)
	return nil
}

// findSkillByName 在 root 下按名称查找技能目录（遵循 --full-depth）。
func findSkillByName(root, name string) (string, error) {
	discovered, err := registry.DiscoverSkillsWithOptions(root, discoverOpts())
	if err != nil {
		return "", fmt.Errorf("discovering skills: %w", err)
	}
	for _, s := range discovered {
		if s.Name == name {
			return s.Path, nil
		}
	}
	return "", fmt.Errorf("skill %q not found in source", name)
}

// selectSingleSkill 在 root 下选择唯一技能；多于一个时提示用 --skill。
func selectSingleSkill(root string) (string, error) {
	discovered, err := registry.DiscoverSkillsWithOptions(root, discoverOpts())
	if err != nil {
		return "", fmt.Errorf("discovering skills: %w", err)
	}
	if len(discovered) == 1 {
		return discovered[0].Path, nil
	}
	if len(discovered) > 1 {
		fmt.Fprintln(os.Stderr, "Multiple skills found. Use --skill to select one:")
		for _, s := range discovered {
			fmt.Fprintf(os.Stderr, "  - %s", s.Name)
			if s.Description != "" {
				fmt.Fprintf(os.Stderr, ": %s", s.Description)
			}
			fmt.Fprintln(os.Stderr)
		}
		return "", fmt.Errorf("multiple skills found, use --skill to select")
	}
	return root, nil
}

// discoverOpts 返回当前 use 选项对应的发现参数（--full-depth）。
func discoverOpts() registry.DiscoverOptions {
	return registry.DiscoverOptions{FullDepth: useFullDepth, AutoFullDepth: true}
}

// materializeSkill 确保技能（含支持文件）可被代理访问。
//   - git 克隆源：技能已在 tmpDir 内，直接返回原目录（克隆树本身即临时目录）。
//   - 本地路径：复制技能目录到新建临时目录，避免污染用户工作目录。
//
// 返回 SKILL.md 路径、供代理读取支持文件的目录、以及是否存在支持文件。
func materializeSkill(skillDir string, tmpDir *string, source string) (skillMD, supportDir string, hasSupport bool, err error) {
	skillMD = resolveSkillMD(skillDir)

	isClone := *tmpDir != ""
	if isClone {
		supportDir = skillDir
	} else {
		td, mkErr := os.MkdirTemp("", "sm-use-*")
		if mkErr != nil {
			return "", "", false, fmt.Errorf("creating temp dir: %w", mkErr)
		}
		*tmpDir = td
		dest := filepath.Join(td, filepath.Base(skillDir))
		if copyErr := copySkillDir(skillDir, dest); copyErr != nil {
			return "", "", false, fmt.Errorf("copying skill to temp: %w", copyErr)
		}
		skillMD = resolveSkillMD(dest)
		supportDir = dest
	}

	hasSupport = dirHasSupportingFiles(supportDir)
	return skillMD, supportDir, hasSupport, nil
}

// resolveSkillMD 返回 dir 下 SKILL.md 路径；不存在时回退到首个 .md 文件。
func resolveSkillMD(dir string) string {
	skillMD := filepath.Join(dir, "SKILL.md")
	if _, err := os.Stat(skillMD); err == nil {
		return skillMD
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			return filepath.Join(dir, e.Name())
		}
	}
	return skillMD
}

// dirHasSupportingFiles 报告 dir 内除 SKILL.md（忽略大小写）外是否还有其它文件。
func dirHasSupportingFiles(dir string) bool {
	var found bool
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || found {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Base(path), "SKILL.md") {
			return nil
		}
		found = true
		return nil
	})
	return found
}

// buildUsePrompt 构造对齐 npx skills 的结构化 prompt：
// 用 <SKILL.md> 标签包裹内容，并在存在支持文件时告知代理其所在目录。
func buildUsePrompt(skillMD, supportDir string, hasSupport bool) string {
	sections := []string{
		"You are being given a Skill to execute for the user's next request.",
		"Use the following SKILL.md as your instructions:",
		"<SKILL.md>\n" + skillMD + "\n</SKILL.md>",
	}
	if hasSupport && supportDir != "" {
		sections = append(sections, "Supporting files for this skill were downloaded to:\n"+supportDir+
			"\n\nWhen the SKILL.md references relative paths, read them from that directory.")
	}
	return strings.Join(sections, "\n\n") + "\n"
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
	useCmd.Flags().BoolVar(&useAcceptOpenClawRisks, "dangerously-accept-openclaw-risks", false, "Allow unverified OpenClaw community skills")
	useCmd.Flags().BoolVar(&useFullDepth, "full-depth", false, "Also discover SKILL.md outside standard skill dirs (e.g. examples/, tests/)")
	rootCmd.AddCommand(useCmd)
}

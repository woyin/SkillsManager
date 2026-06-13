// cmd/use.go
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

var (
	useSkill  string
	useAgent  string
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
	tmpDir, err := os.MkdirTemp("", "sm-use-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	var skillDir string

	if registry.IsGitURL(source) {
		// Clone the source
		repoURL, branch, subPath, _ := registry.ParseTreeURL(source)
		if repoURL == "" {
			repoURL = registry.NormalizeGitURL(source)
		}

		cloneDest := filepath.Join(tmpDir, "repo")
		if branch != "" {
			err = cloneRepoWithBranch(repoURL, branch, cloneDest)
		} else {
			err = cloneRepoSimple(repoURL, cloneDest)
		}
		if err != nil {
			return fmt.Errorf("cloning: %w", err)
		}

		if subPath != "" {
			skillDir = filepath.Join(cloneDest, subPath)
		} else if useSkill != "" {
			// Find specific skill
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
			// Use first discovered skill or the repo root
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
		// Local path
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

	// Read the SKILL.md content
	skillMD := filepath.Join(skillDir, "SKILL.md")
	content, err := os.ReadFile(skillMD)
	if err != nil {
		// Try reading any markdown file
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
		// Start the agent with the prompt
		return startAgent(useAgent, prompt, tmpDir)
	}

	// Print prompt to stdout
	fmt.Print(prompt)
	return nil
}

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

	// Write the prompt to a temp file
	promptFile := filepath.Join(tmpDir, "prompt.md")
	if err := os.WriteFile(promptFile, []byte(prompt), 0644); err != nil {
		return fmt.Errorf("writing prompt file: %w", err)
	}

	// Start the agent with the prompt file as input
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

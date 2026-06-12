// cmd/add.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"os/exec"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

var (
	addFlags    specialFlags
	addIsMCP    bool
	addList     bool
	addSkills   []string
	addAgents   []string
	addCopy     bool
	addYes      bool
	addAll      bool
)

var addCmd = &cobra.Command{
	Use:   "add <source> [category]",
	Short: "Add a skill or MCP to the registry",
	Long: `Add a skill or MCP server definition to the registry.

Source formats:
  owner/repo                          GitHub shorthand
  https://github.com/owner/repo       Full GitHub URL
  https://github.com/owner/repo/tree/main/skills/name  Direct skill path
  https://gitlab.com/org/repo         GitLab URL
  git@github.com:owner/repo.git       SSH git URL
  ./my-local-skills                   Local path

Category is the directory name under registry/skills/ or registry/mcp/.
Use --agent to target specific AI agents, --skill to pick specific skills.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		source := args[0]
		reg := registry.New(RegistryDir)

		if addIsMCP {
			if err := reg.AddMCP(source); err != nil {
				return fmt.Errorf("adding MCP: %w", err)
			}
			fmt.Printf("✓ Added MCP from %s\n", source)
			return nil
		}

		// --list mode: discover and list skills from source
		if addList {
			return listSkillsFromSource(source)
		}

		// --all mode: install all skills to all agents
		if addAll {
			addSkills = []string{"*"}
			addAgents = []string{"*"}
			addYes = true
		}

		// If --agent is specified, install to specific agents
		if len(addAgents) > 0 {
			return addWithAgents(reg, source)
		}

		// Standard add flow (backward compatible)
		special := addFlags.Resolve()
		category := ""
		if len(args) > 1 {
			category = args[1]
		}

		if err := reg.AddSkillWithOptions(source, category, special, addSkills, addCopy); err != nil {
			return fmt.Errorf("adding skill: %w", err)
		}

		dest := special
		if dest == "" {
			dest = category
		}
		name := registry.SkillNameFromPath(source)
		if len(addSkills) > 0 {
			fmt.Printf("✓ Added %d skill(s) to %s\n", len(addSkills), dest)
		} else {
			fmt.Printf("✓ Added skill %q to %s\n", name, dest)
		}
		return nil
	},
}

// listSkillsFromSource clones the source and discovers skills
func listSkillsFromSource(source string) error {
	if !registry.IsGitURL(source) {
		// Local path: discover directly
		skills, err := registry.DiscoverSkills(source)
		if err != nil {
			return fmt.Errorf("discovering skills: %w", err)
		}
		printDiscoveredSkills(skills)
		return nil
	}

	// Clone to temp dir
	repoURL, branch, _, _ := registry.ParseTreeURL(source)
	if repoURL == "" {
		repoURL = normalizeSourceURL(source)
	}

	tmpDir, err := os.MkdirTemp("", "sm-list-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cloneDest := filepath.Join(tmpDir, "repo")
	reg := registry.New(RegistryDir)

	// Use internal clone method - we need to call through the registry
	if branch != "" {
		err = cloneRepoWithBranch(repoURL, branch, cloneDest)
	} else {
		err = cloneRepoSimple(repoURL, cloneDest)
	}
	if err != nil {
		return fmt.Errorf("cloning %s: %w", repoURL, err)
	}

	_ = reg // suppress unused warning
	skills, err := registry.DiscoverSkills(cloneDest)
	if err != nil {
		return fmt.Errorf("discovering skills: %w", err)
	}
	printDiscoveredSkills(skills)
	return nil
}

func printDiscoveredSkills(skills []registry.DiscoveredSkill) {
	if len(skills) == 0 {
		fmt.Println("No skills found in source.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION")
	fmt.Fprintln(w, "----\t-----------")
	for _, s := range skills {
		desc := s.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Fprintf(w, "%s\t%s\n", s.Name, desc)
	}
	w.Flush()
	fmt.Printf("\n%d skill(s) found\n", len(skills))
}

// addWithAgents installs skills to specific agent directories
func addWithAgents(reg *registry.Registry, source string) error {
	targetTools := tool.ToolsByNames(addAgents)
	if len(targetTools) == 0 {
		return fmt.Errorf("no matching agents found for: %v", addAgents)
	}

	// First, get the skills into a temp location
	var skillsToInstall []registry.DiscoveredSkill

	if registry.IsGitURL(source) {
		repoURL, branch, subPath, _ := registry.ParseTreeURL(source)
		if repoURL == "" {
			repoURL = normalizeSourceURL(source)
		}

		tmpDir, err := os.MkdirTemp("", "sm-add-*")
		if err != nil {
			return fmt.Errorf("creating temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		cloneDest := filepath.Join(tmpDir, "repo")
		if branch != "" {
			err = cloneRepoWithBranch(repoURL, branch, cloneDest)
		} else {
			err = cloneRepoSimple(repoURL, cloneDest)
		}
		if err != nil {
			return fmt.Errorf("cloning %s: %w", repoURL, err)
		}

		// If subPath specified, treat it as a single skill
		if subPath != "" {
			skillDir := filepath.Join(cloneDest, subPath)
			skillMD := filepath.Join(skillDir, "SKILL.md")
			name := filepath.Base(subPath)
			desc := ""
			if _, err := os.Stat(skillMD); err == nil {
				desc = parseSkillDesc(skillMD)
			}
			skillsToInstall = append(skillsToInstall, registry.DiscoveredSkill{
				Name: name, Description: desc, Path: skillDir, SkillMDPath: skillMD,
			})
		} else {
			discovered, err := registry.DiscoverSkills(cloneDest)
			if err != nil {
				return fmt.Errorf("discovering skills: %w", err)
			}
			skillsToInstall = filterSkills(discovered, addSkills)
		}
	} else {
		// Local path
		discovered, err := registry.DiscoverSkills(source)
		if err != nil {
			return fmt.Errorf("discovering skills: %w", err)
		}
		skillsToInstall = filterSkills(discovered, addSkills)
	}

	if len(skillsToInstall) == 0 {
		return fmt.Errorf("no matching skills found in source")
	}

	// Install to each agent's global skill directory
	home, _ := os.UserHomeDir()
	installed := 0
	for _, t := range targetTools {
		agentSkillDir := filepath.Join(home, t.SkillDir)
		for _, skill := range skillsToInstall {
			dest := filepath.Join(agentSkillDir, skill.Name)
			if addCopy {
				if err := copySkillDir(skill.Path, dest); err != nil {
					fmt.Fprintf(os.Stderr, "warning: skipping %s for %s: %v\n", skill.Name, t.Name, err)
					continue
				}
			} else {
				if err := os.MkdirAll(agentSkillDir, 0755); err != nil {
					fmt.Fprintf(os.Stderr, "warning: creating dir for %s: %v\n", t.Name, err)
					continue
				}
				absSrc, _ := filepath.Abs(skill.Path)
				// Remove existing
				os.Remove(dest)
				if err := os.Symlink(absSrc, dest); err != nil {
					fmt.Fprintf(os.Stderr, "warning: symlink %s for %s: %v\n", skill.Name, t.Name, err)
					continue
				}
			}
			installed++
			fmt.Printf("  ✓ %s → %s\n", skill.Name, dest)
		}
	}

	fmt.Printf("\n✓ Installed %d skill(s) to %d agent(s)\n", installed, len(targetTools))
	return nil
}

func filterSkills(discovered []registry.DiscoveredSkill, names []string) []registry.DiscoveredSkill {
	if len(names) == 0 {
		return discovered
	}
	for _, n := range names {
		if n == "*" {
			return discovered
		}
	}
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	var filtered []registry.DiscoveredSkill
	for _, s := range discovered {
		if nameSet[s.Name] {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func copySkillDir(src, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		os.RemoveAll(dest)
	}
	return copyDirAll(src, dest)
}

func copyDirAll(src, dest string) error {
	srcInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, srcInfo.Mode()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())
		if entry.IsDir() {
			if err := copyDirAll(srcPath, destPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			info, _ := entry.Info()
			if err := os.WriteFile(destPath, data, info.Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseSkillDesc(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	content := string(data)
	if len(content) < 6 || content[:3] != "---" {
		return ""
	}
	rest := content[3:]
	endIdx := -1
	for i := 0; i < len(rest)-2; i++ {
		if rest[i:i+3] == "---" {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		return ""
	}
	fm := rest[:endIdx]
	for _, line := range splitLines(fm) {
		if len(line) > 12 && line[:12] == "description:" {
			desc := line[12:]
			desc = trimSpace(desc)
			if len(desc) >= 2 && (desc[0] == '"' || desc[0] == '\'') && desc[len(desc)-1] == desc[0] {
				desc = desc[1 : len(desc)-1]
			}
			return desc
		}
	}
	return ""
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func normalizeSourceURL(source string) string {
	if registry.IsGitURL(source) {
		// Try to normalize GitHub shorthand
		if len(source) > 0 && source[0] != '/' && source[0] != '.' {
			parts := splitBySlash(source)
			if len(parts) >= 2 && !hasScheme(source) {
				return "https://github.com/" + parts[0] + "/" + parts[1]
			}
		}
	}
	return source
}

func splitBySlash(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func hasScheme(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return i > 0 && i < len(s)-1 && s[i+1] == '/'
		}
		if s[i] == '/' || s[i] == '@' {
			return false
		}
	}
	return false
}

// Simple git clone helpers (standalone, not through registry)
func cloneRepoSimple(url, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("destination already exists: %s", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	cmd := exec.Command("git", "clone", url, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func cloneRepoWithBranch(url, branch, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("destination already exists: %s", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	cmd := exec.Command("git", "clone", "--branch", branch, "--depth", "1", url, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func init() {
	addCmd.Flags().BoolVar(&addFlags.Global, "global", false, "Add to global directory (all tools)")
	addCmd.Flags().BoolVar(&addFlags.Codex, "codex", false, "Add to codex-only directory")
	addCmd.Flags().BoolVar(&addFlags.Claude, "claude", false, "Add to claude-only directory")
	addCmd.Flags().BoolVar(&addFlags.Gemini, "gemini", false, "Add to gemini-only directory")
	addCmd.Flags().BoolVar(&addFlags.OpenCode, "opencode", false, "Add to opencode-only directory")
	addCmd.Flags().BoolVar(&addFlags.Hermes, "hermes", false, "Add to hermes-only directory")
	addCmd.Flags().BoolVar(&addFlags.OpenClaw, "openclaw", false, "Add to openclaw-only directory")
	addCmd.Flags().BoolVar(&addIsMCP, "mcp", false, "Add as MCP server definition")

	// New flags from vercel-labs/skills
	addCmd.Flags().BoolVarP(&addList, "list", "l", false, "List available skills without installing")
	addCmd.Flags().StringArrayVarP(&addSkills, "skill", "s", nil, "Install specific skills by name (use '*' for all)")
	addCmd.Flags().StringArrayVarP(&addAgents, "agent", "a", nil, "Target specific agents (use '*' for all)")
	addCmd.Flags().BoolVar(&addCopy, "copy", false, "Copy files instead of symlinking")
	addCmd.Flags().BoolVarP(&addYes, "yes", "y", false, "Skip all confirmation prompts")
	addCmd.Flags().BoolVar(&addAll, "all", false, "Install all skills to all agents without prompts")

	rootCmd.AddCommand(addCmd)
}

// cmd/install.go 实现 `sm install`：
//   - 无 source：把 profile 与额外 skills/MCP 安装到当前项目（创建符号链接 + 合并 .mcp.json），并写入数据库。
//   - 带 source：从来源（GitHub/URL/本地路径）发现技能，安装到指定代理的全局技能目录（--agent/--skill/--all/--copy/--yes）。
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/fsutil"
	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/installer"
	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

var (
	installProfile string
	installDir     string

	// source-based install flags
	installList    bool
	installSkills  []string
	installAgents  []string
	installCopy    bool
	installYes     bool
	installAll     bool
	installProject bool // --project: 装到项目级目录 ./<agent>/skills 而非全局 ~/<agent>/skills
	installRef     string
	installOffline bool
)

var installCmd = &cobra.Command{
	Use:   "install [source]",
	Short: "Install skills and MCP into the current project or agent directories",
	Long: `Install skills and MCP configurations.

Without a source argument:
  Reads .sm.json if present, or uses --profile flag.
  Creates symlinks in tool-specific skills directories.
  Writes .mcp.json for MCP server configurations.

With a source argument:
  Discovers skills in the source (GitHub shorthand, full URL, SSH URL, or local path)
  and installs them into the specified agents' global skill directories.
  Use --agent to target agents, --skill to pick specific skills.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// source-based mode
		if len(args) == 1 {
			return installFromSource(args[0])
		}

		// profile/project mode (original behavior)
		projectDir := installDir
		if projectDir == "" {
			var err error
			projectDir, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
		}

		pm := project.NewManager(projectDir)
		config, err := pm.Load()
		if err != nil {
			return fmt.Errorf("loading project config: %w", err)
		}

		profileName := installProfile
		if profileName == "" {
			profileName = config.Profile
		}

		extraSkills := config.Skills
		extraMCP := config.MCP

		if profileName == "" && len(extraSkills) == 0 && len(extraMCP) == 0 {
			return fmt.Errorf("nothing to install: create .sm.json with a profile, or use --profile flag")
		}

		tools := tool.DetectInstalled(tool.AllTools())
		if len(tools) == 0 {
			tools = tool.DefaultTools()
		}

		inst, err := installer.New(RegistryDir, ProfilesDir, tools)
		if err != nil {
			return fmt.Errorf("creating installer: %w", err)
		}

		result, err := inst.Install(projectDir, profileName, extraSkills, extraMCP)
		if err != nil {
			return fmt.Errorf("install failed: %w", err)
		}

		database, err := openDB()
		if err != nil {
			return fmt.Errorf("opening database: %w", err)
		}
		defer database.Close()

		if err := database.RecordInstallation(projectDir, profileName, result.Skills, result.MCP); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to record installation: %v\n", err)
		}

		if err := database.UpsertProject(projectDir, profileName, extraSkills, extraMCP); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to update project record: %v\n", err)
		}

		fmt.Printf("✓ Installed to %s\n", projectDir)
		if profileName != "" {
			fmt.Printf("  Profile: %s\n", profileName)
		}
		if len(result.Skills) > 0 {
			fmt.Printf("  Skills: %d symlinks created\n", len(result.Skills))
			for _, s := range result.Skills {
				fmt.Printf("    → %s\n", s)
			}
		}
		if len(result.MCP) > 0 {
			fmt.Printf("  MCP: %v\n", result.MCP)
		}

		return nil
	},
}

// installFromSource handles `sm install <source>`: discover skills in the source
// and install them into the targeted agents' skill directories.
func installFromSource(source string) error {
	if installRef != "" && !registry.IsGitURL(source) {
		return fmt.Errorf("--ref requires a remote Git source")
	}
	// --list: discover only, no install
	if installList {
		return listSkillsFromSource(source)
	}

	if installAll {
		installSkills = []string{"*"}
		installAgents = []string{"*"}
		installYes = true
	}

	projectDir := ""
	if installProject {
		projectDir = installDir
		if projectDir == "" {
			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			projectDir = wd
		}
	}

	return installSkillsToAgents(source, installAgents, installSkills, installCopy, installProject, projectDir)
}

// listSkillsFromSource clones (if needed) and lists discoverable skills.
func listSkillsFromSource(source string) error {
	if !registry.IsGitURL(source) {
		skills, err := registry.DiscoverSkills(source)
		if err != nil {
			return fmt.Errorf("discovering skills: %w", err)
		}
		printDiscoveredSkills(skills)
		return nil
	}

	cloneDest, err := cachedGitSource(source, installRef, installOffline)
	if err != nil {
		return err
	}

	_, _, subPath, _ := registry.ParseTreeURL(source)
	if subPath != "" {
		cloneDest = filepath.Join(cloneDest, subPath)
	}
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

// installSkillsToAgents installs discovered skills into each target agent's skill dir.
func installSkillsToAgents(source string, agentNames, skillNames []string, copyMode bool, project bool, projectDir string) error {
	targetTools := tool.ToolsByNames(agentNames)
	if len(targetTools) == 0 {
		return fmt.Errorf("no matching agents found for: %v", agentNames)
	}

	var skillsToInstall []registry.DiscoveredSkill

	if registry.IsGitURL(source) {
		cloneDest, err := cachedGitSource(source, installRef, installOffline)
		if err != nil {
			return err
		}

		_, _, subPath, _ := registry.ParseTreeURL(source)
		if subPath != "" {
			skillDir := filepath.Join(cloneDest, subPath)
			skillMD := filepath.Join(skillDir, "SKILL.md")
			name := filepath.Base(subPath)
			desc := ""
			if _, err := os.Stat(skillMD); err == nil {
				desc = registry.ParseFrontmatterDescription(skillMD)
			}
			skillsToInstall = append(skillsToInstall, registry.DiscoveredSkill{
				Name: name, Description: desc, Path: skillDir, SkillMDPath: skillMD,
			})
		} else {
			discovered, err := registry.DiscoverSkills(cloneDest)
			if err != nil {
				return fmt.Errorf("discovering skills: %w", err)
			}
			skillsToInstall = filterSkills(discovered, skillNames)
		}
	} else {
		discovered, err := registry.DiscoverSkills(source)
		if err != nil {
			return fmt.Errorf("discovering skills: %w", err)
		}
		skillsToInstall = filterSkills(discovered, skillNames)
	}

	if len(skillsToInstall) == 0 {
		return fmt.Errorf("no matching skills found in source")
	}

	jobs := make([]installJob, 0, len(targetTools)*len(skillsToInstall))
	for _, t := range targetTools {
		var agentSkillDir string
		if project {
			agentSkillDir = tool.GetProjectSkillDir(t, projectDir)
			if agentSkillDir == "" {
				continue // 该工具无项目级目录，跳过
			}
		} else {
			agentSkillDir = filepath.Join(home.Dir(), t.SkillDir)
		}
		for _, skill := range skillsToInstall {
			jobs = append(jobs, installJob{
				tool:     t,
				skill:    skill,
				dest:     filepath.Join(agentSkillDir, skill.Name),
				agentDir: agentSkillDir,
			})
		}
	}

	results := installSkillsConcurrently(jobs, copyMode)

	installed := 0
	for _, ok := range results {
		if ok {
			installed++
		}
	}

	fmt.Printf("\n✓ Installed %d skill(s) to %d agent(s)", installed, len(targetTools))
	if project {
		fmt.Printf(" [project: %s]", projectDir)
	} else {
		fmt.Print(" [global]")
	}
	fmt.Println()
	return nil
}

// installJob is one (agent, skill) install target.
type installJob struct {
	tool     tool.Tool
	skill    registry.DiscoveredSkill
	dest     string
	agentDir string
}

// installSkillsConcurrently 通过有限 worker 池并发安装技能。
// 每个 job 写入不同目标路径，彼此独立；MkdirAll 是幂等的，
// 对同一 agent 目录的并发调用是安全的。
func installSkillsConcurrently(jobs []installJob, copyMode bool) []bool {
	results := make([]bool, len(jobs))
	if len(jobs) == 0 {
		return results
	}

	// 并发上限：I/O 密集型允许真实并行，封顶 8 避免压垮主机。
	workers := clamp(runtime.NumCPU(), 1, min(8, len(jobs)))

	var (
		wg    sync.WaitGroup
		outMu sync.Mutex
	)

	doInstall := func(i int) {
		j := jobs[i]
		var err error
		if copyMode {
			err = copySkillDir(j.skill.Path, j.dest)
		} else {
			if mkErr := os.MkdirAll(j.agentDir, 0755); mkErr != nil {
				outMu.Lock()
				fmt.Fprintf(os.Stderr, "warning: creating dir for %s: %v\n", j.tool.Name, mkErr)
				outMu.Unlock()
				return
			}
			absSrc, _ := filepath.Abs(j.skill.Path)
			os.Remove(j.dest)
			err = os.Symlink(absSrc, j.dest)
		}
		if err != nil {
			outMu.Lock()
			fmt.Fprintf(os.Stderr, "warning: install %s for %s: %v\n", j.skill.Name, j.tool.Name, err)
			outMu.Unlock()
			return
		}
		outMu.Lock()
		fmt.Printf("  ✓ %s → %s\n", j.skill.Name, j.dest)
		outMu.Unlock()
		results[i] = true
	}

	// 用带缓冲的 channel 分发索引，省去独立的分发 goroutine。
	jobCh := make(chan int, workers)
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := range jobCh {
				doInstall(i)
			}
		}()
	}
	for i := range jobs {
		jobCh <- i
	}
	close(jobCh)
	wg.Wait()
	return results
}

// clamp 限制 v 到 [lo, hi]。
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// filterSkills keeps skills whose names match; "*" returns all.
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

// copySkillDir copies a skill dir, overwriting an existing destination.
func copySkillDir(src, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		os.RemoveAll(dest)
	}
	return fsutil.CopyDir(src, dest)
}

func init() {
	installCmd.Flags().StringVar(&installProfile, "profile", "", "Profile name to install")
	installCmd.Flags().StringVar(&installDir, "dir", "", "Project directory (default: current dir)")

	// source-based install flags
	installCmd.Flags().BoolVarP(&installList, "list", "l", false, "List available skills in source without installing")
	installCmd.Flags().StringArrayVarP(&installSkills, "skill", "s", nil, "Install specific skills by name (use '*' for all)")
	installCmd.Flags().StringArrayVarP(&installAgents, "agent", "a", nil, "Target specific agents (use '*' for all)")
	installCmd.Flags().BoolVar(&installCopy, "copy", false, "Copy files instead of symlinking")
	installCmd.Flags().BoolVarP(&installYes, "yes", "y", false, "Skip all confirmation prompts")
	installCmd.Flags().BoolVar(&installAll, "all", false, "Install all skills to all agents without prompts")
	installCmd.Flags().BoolVarP(&installProject, "project", "p", false,
		"Install into project-level skill dirs (./<agent>/skills) instead of global (~/<agent>/skills)")
	installCmd.Flags().StringVar(&installRef, "ref", "", "Snapshot remote source at a Git branch, tag, or commit (use commit for reproducibility)")
	installCmd.Flags().BoolVar(&installOffline, "offline", false, "Use exact cached source/ref without network access")

	rootCmd.AddCommand(installCmd)
}

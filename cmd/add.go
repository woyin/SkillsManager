// cmd/add.go 实现 `sm add`：把技能/MCP 加入注册表。
// 支持 GitHub 简写、完整 URL、SSH URL、本地路径；
// --agent 可直接安装到指定代理目录（并发执行）。
// cmd/add.go
package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/fsutil"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"text/tabwriter"
)

var (
	addFlags  specialFlags
	addIsMCP  bool
	addList   bool
	addSkills []string
	addAgents []string
	addCopy   bool
	addYes    bool
	addAll    bool
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

		// --list 模式：发现并列出来源中的技能
		if addList {
			return listSkillsFromSource(source)
		}

		// --all 模式：把全部技能安装到全部代理
		if addAll {
			addSkills = []string{"*"}
			addAgents = []string{"*"}
			addYes = true
		}

		// 指定 --agent 时安装到指定代理目录
		if len(addAgents) > 0 {
			return addWithAgents(reg, source)
		}

		// 标准 add 流程（向后兼容）
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

// 克隆（或本地读取）来源并列出其中可发现的技能。
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

	cloneDest, tmpDir, err := registry.CloneToTemp(source, "sm-list-*")
	if err != nil {
		return err
	}
	defer registry.RemoveCloneTemp(tmpDir)

	skills, err := registry.DiscoverSkills(cloneDest)
	if err != nil {
		return fmt.Errorf("discovering skills: %w", err)
	}
	printDiscoveredSkills(skills)
	return nil
}
// 以表格形式打印发现的技能清单。

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

// 把技能安装到指定的代理目录（每个 (代理, 技能) 任务相互独立，可并发）。
// addWithAgents installs skills to specific agent directories
func addWithAgents(reg *registry.Registry, source string) error {
	targetTools := tool.ToolsByNames(addAgents)
	if len(targetTools) == 0 {
		return fmt.Errorf("no matching agents found for: %v", addAgents)
	}

	// 先把技能放到一个临时位置
	var skillsToInstall []registry.DiscoveredSkill

	if registry.IsGitURL(source) {
		cloneDest, tmpDir, err := registry.CloneToTemp(source, "sm-add-*")
		if err != nil {
			return err
		}
		defer registry.RemoveCloneTemp(tmpDir)

		_, _, subPath, _ := registry.ParseTreeURL(source)
		// 指定 subPath 时，把它当作单个技能
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

	// 安装到每个代理的全局技能目录。
	// 每个 (代理, 技能) 对的目标路径互不相同，故彼此独立，
	// 可并发执行——当 --all 涉及数十个代理时收益尤其明显。
	// （续上）
	home, _ := os.UserHomeDir()

	jobs := make([]installJob, 0, len(targetTools)*len(skillsToInstall))
	for _, t := range targetTools {
		agentSkillDir := filepath.Join(home, t.SkillDir)
		for _, skill := range skillsToInstall {
			jobs = append(jobs, installJob{
				tool:     t,
				skill:    skill,
				dest:     filepath.Join(agentSkillDir, skill.Name),
				agentDir: agentSkillDir,
			})
		}
	}

	results := installSkillsConcurrently(jobs, addCopy)

	installed := 0
	for _, ok := range results {
		if ok {
			installed++
		}
	}

	fmt.Printf("\n✓ Installed %d skill(s) to %d agent(s)\n", installed, len(targetTools))
	return nil
}

// installSkillsConcurrently 的单个安装任务：某个代理的某个技能目标。
// installJob is one (agent, skill) install target for installSkillsConcurrently.
type installJob struct {
	tool     tool.Tool
	skill    registry.DiscoveredSkill
	dest     string // destination path under the agent's skills dir
	agentDir string // the agent's skills dir (created if missing)
}

// 通过有界 worker 池并发执行所有安装任务。
// 每个任务写入唯一目标路径，故彼此独立；MkdirAll 对同一代理目录幂等且并发安全。
// 返回与输入同序的 ok 标记切片。
// installSkillsConcurrently runs every install job through a bounded worker
// pool. Each job writes to a unique destination, so the operations are
// independent; MkdirAll is idempotent and safe under concurrent calls to the
// same agent directory. Returns one ok flag per job (input order preserved).
func installSkillsConcurrently(jobs []installJob, copyMode bool) []bool {
	results := make([]bool, len(jobs))
	if len(jobs) == 0 {
		return results
	}

	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8
	}
	if workers > len(jobs) {
		workers = len(jobs)
	}
	if workers < 1 {
		workers = 1
	}

	var (
		wg    sync.WaitGroup
		outMu sync.Mutex // serialize stdout/stderr so lines stay whole
		jobCh = make(chan int)
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
			os.Remove(j.dest) // clear a prior install at this path
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

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobCh {
				doInstall(i)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range jobs {
			jobCh <- i
		}
		close(jobCh)
	}()
	wg.Wait()
	return results
}
// 按名称过滤已发现的技能；names 含 "*" 时返回全部。

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
// 以拷贝方式安装一个技能目录（覆盖已存在的目标）。

func copySkillDir(src, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		os.RemoveAll(dest)
	}
	return fsutil.CopyDir(src, dest)
}

func init() {
	addFlags.Bind(addCmd, "Add to")
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

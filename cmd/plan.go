// cmd/plan.go 实现 `sm plan`：Curation Core 的命令入口（ADR 0022）。
//
// 默认只读，生成并预览项目的 Curation Plan；支持：
//   - 无参：打印人类可读的拟改清单（add / remove / leave）与证据；
//   - --json：输出机器可读表示（供 CI / 编辑器 / dashboard 集成）；
//   - --check：供 CI 判定是否满足（不满足返回非零）；
//   - --apply：显式应用（必须先 preflight、原子应用、只移除 owned Link Install）；
//   - --profile / --skill：为 bootstrap 计划提供显式目标（ADR 0028）。
//
// sm status 保持为简洁的健康一页纸；策划动作统一收敛到本命令。
//
// Input: encoding/json, fmt, os, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/curation, github.com/woyin/skills-manager/internal/project, github.com/woyin/skills-manager/internal/tool
// Output: var planCmd, func runPlanCommand, func renderPlanHuman, func renderPlanJSON, func resolvePlanTarget, func applyPlan
// Pos: 控制层-plan命令实现（策划计划生成/预览/应用）
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/curation"
	"github.com/woyin/skills-manager/internal/installer"
	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/tool"
)

// planOut 是 plan 命令输出的目标，默认 os.Stdout；测试替换为内存缓冲。
var planOut io.Writer = os.Stdout

var (
	planDir     string
	planApply   bool
	planJSON    bool
	planCheck   bool
	planProfile string
	planSkills  []string
	planYes     bool
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Preview or apply a Curation Plan for the project",
	Long: `Inspect, preview, and apply the Curation Plan for this project.

Read-only by default: sm plan shows the proposed adds, removes, and leaves for
the project's Installed Skills, each with a reason and supporting evidence.

  sm plan                 # preview
  sm plan --json          # machine-readable preview
  sm plan --check         # exit nonzero unless the plan is satisfied
  sm plan --apply         # explicitly apply (preflighted, atomic)

Without an explicit target (.sm.json has no profile/skills), sm plan produces a
Bootstrap Curation Plan that recommends only. Applying requires choosing a
target first: --profile <name> or --skill <name>.

Application never removes manual installations, Copy Installs, or entries whose
ownership is unknown: it only removes recorded project-scope Link Installs owned
by the baseline (sm status stays your concise health report).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPlanCommand()
	},
}

func runPlanCommand() error {
	projectDir, err := project.ResolveProjectDir(planDir)
	if err != nil {
		return err
	}

	// 目标代理：Detected Agents，无则回退默认工具集（与 install/status 一致）。
	agents := tool.DetectInstalled(tool.AllTools())
	if len(agents) == 0 {
		agents = tool.DefaultTools()
	}

	planner := curation.NewPlanner(RegistryDir, ProfilesDir, agents, projectDir)

	plan, err := planner.PlanForProject()
	if err != nil {
		return fmt.Errorf("generating curation plan: %w", err)
	}

	switch {
	case planJSON:
		return renderPlanJSON(plan)
	case planCheck:
		return evaluatePlanCheck(plan, projectDir)
	case planApply:
		return applyPlan(plan, projectDir)
	default:
		return renderPlanHuman(plan)
	}
}

func renderPlanJSON(plan *curation.Plan) error {
	data, err := json.MarshalIndent(plan.Evaluate(), "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(planOut, string(data))
	return nil
}

func evaluatePlanCheck(plan *curation.Plan, projectDir string) error {
	renderPlanHuman(plan)
	if !plan.CheckOK {
		return fmt.Errorf("curation plan not satisfied for %s", projectDir)
	}
	return nil
}

func renderPlanHuman(plan *curation.Plan) error {
	out := planOut
	fmt.Fprintf(out, "Project: %s\n", plan.Project)
	if plan.Bootstrap {
		fmt.Fprintln(out, "Status: bootstrap (no explicit curation target yet)")
		fmt.Fprintln(out, "  This project has no .sm.json profile/skills. Choose a target with:")
		fmt.Fprintln(out, "    sm plan --apply --profile <name>")
		fmt.Fprintln(out, "    sm plan --apply --skill <name> [--skill <name> ...]")
		fmt.Fprintln(out, "No changes will be made until you choose an explicit target.")
		if len(plan.RecommendedProfiles) > 0 {
			fmt.Fprintln(out, "  Available profiles:")
			for _, p := range plan.RecommendedProfiles {
				fmt.Fprintf(out, "    - %s\n", p)
			}
		}
		if len(plan.RecommendedSkills) > 0 {
			fmt.Fprintln(out, "  Available registry skills:")
			for _, s := range plan.RecommendedSkills {
				fmt.Fprintf(out, "    - %s\n", s)
			}
		}
		if len(plan.Warnings) > 0 {
			for _, w := range plan.Warnings {
				fmt.Fprintf(out, "  warning: %s\n", w)
			}
		}
		return nil
	}

	fmt.Fprintln(out, "Baseline:")
	if plan.Baseline != nil {
		if plan.Baseline.HasProfile {
			fmt.Fprintf(out, "  profile: %s\n", plan.Baseline.Profile)
		}
		if len(plan.Baseline.Skills) > 0 {
			fmt.Fprintf(out, "  skills:  %v\n", plan.Baseline.Skills)
		}
	}

	if len(plan.Proposals) == 0 {
		fmt.Fprintln(out, "No changes proposed (project is curate-clean).")
		return nil
	}

	sort.Slice(plan.Proposals, func(i, j int) bool {
		if plan.Proposals[i].Action != plan.Proposals[j].Action {
			return plan.Proposals[i].Action < plan.Proposals[j].Action
		}
		return plan.Proposals[i].Agent < plan.Proposals[j].Agent
	})

	groups := map[curation.Action][]curation.Proposal{}
	for _, p := range plan.Proposals {
		groups[p.Action] = append(groups[p.Action], p)
	}
	order := []curation.Action{curation.ActionAdd, curation.ActionRemove, curation.ActionLeave}
	for _, act := range order {
		list := groups[act]
		if len(list) == 0 {
			continue
		}
		label := map[curation.Action]string{
			curation.ActionAdd:    "ADD",
			curation.ActionRemove: "REMOVE",
			curation.ActionLeave:  "LEAVE",
		}[act]
		fmt.Fprintf(out, "%s:\n", label)
		for _, p := range list {
			owned := ""
			if p.Action == curation.ActionRemove && p.Owned {
				owned = " [owned]"
			}
			if p.Action == curation.ActionRemove && !p.Owned {
				owned = " [cleanup candidate, not auto-removable]"
			}
			fmt.Fprintf(out, "  %s/%s%s — %s\n", p.Agent, p.Skill, owned, p.Reason)
		}
	}

	if len(plan.Warnings) > 0 {
		fmt.Fprintln(out, "Warnings:")
		for _, w := range plan.Warnings {
			fmt.Fprintf(out, "  %s\n", w)
		}
	}

	if plan.NeedsAction() {
		fmt.Fprintln(out, "\nRun `sm plan --apply` to apply (preflighted, atomic).")
	} else {
		fmt.Fprintln(out, "\nPlan satisfied; no action required.")
	}
	return nil
}

// applyPlan 应用计划。bootstrap 需要显式目标（--profile/--skill）。
//
// 两条路径：
//  1. curated 项目：用 plan.Apply（先落地缺失 baseline 成员、再只移除 owned
//     Link Install）并把新增项登记为 managed（ADR 0023）；
//  2. bootstrap 项目：先经共享 installer.Install 原子预检并落地用户选定的
//     显式目标组成，再把 `.sm.json` 写为目标（ADR 0028）。
func applyPlan(plan *curation.Plan, projectDir string) error {
	if plan.Bootstrap {
		return applyBootstrapTarget(plan, projectDir)
	}

	opts := curation.ApplyOptions{ApplyRemovals: true}
	opts.InstallAdditions = func(skills []string) error {
		inst, err := installer.New(RegistryDir, ProfilesDir, agentsForPlan())
		if err != nil {
			return err
		}
		inst.SetScope(projectDir, false)
		_, err = inst.InstallFromRegistry(skills, "")
		return err
	}

	if !planJSON {
		if err := renderPlanHuman(plan); err != nil {
			return err
		}
	}

	result, err := plan.Apply(opts)
	if err != nil {
		return fmt.Errorf("applying curation plan: %w", err)
	}

	// 登记新增项为 managed（ADR 0023）。
	if len(result.Added) > 0 {
		if err := recordAddedOwnership(projectDir, result.Added); err != nil {
			return err
		}
	}

	return reportApplyResult(plan, result, projectDir)
}

// applyBootstrapTarget 为 bootstrap 项目应用用户选定的显式目标。
// 原子：installer.Install 全量预检；写 .sm.json 仅在此成功后进行（ADR 0028）。
func applyBootstrapTarget(plan *curation.Plan, projectDir string) error {
	if planProfile == "" && len(planSkills) == 0 {
		return fmt.Errorf("bootstrap plan requires an explicit curation target (use --profile <name> or --skill <name>)")
	}

	if !planJSON {
		if err := renderPlanHuman(plan); err != nil {
			return err
		}
	}

	inst, err := installer.New(RegistryDir, ProfilesDir, agentsForPlan())
	if err != nil {
		return err
	}
	inst.SetScope(projectDir, false)
	res, err := inst.Install(projectDir, planProfile, planSkills, nil)
	if err != nil {
		return fmt.Errorf("installing explicit curation target: %w", err)
	}
	_ = res

	// 登记新增项为 managed。
	added := append([]string(nil), planSkills...)
	if err := recordAddedOwnership(projectDir, added); err != nil {
		return err
	}

	reported := &curation.ApplyResult{}
	for _, s := range planSkills {
		reported.Added = append(reported.Added, s)
	}
	return reportApplyResult(plan, reported, projectDir)
}

func reportApplyResult(plan *curation.Plan, result *curation.ApplyResult, projectDir string) error {
	out := planOut
	fmt.Fprintf(out, "✓ Applied curation plan to %s\n", projectDir)
	if len(result.Removed) > 0 {
		fmt.Fprintln(out, "  Removed:")
		for _, r := range result.Removed {
			fmt.Fprintf(out, "    - %s\n", r)
		}
	}
	if len(result.Added) > 0 {
		fmt.Fprintln(out, "  Added (planned):")
		for _, a := range result.Added {
			fmt.Fprintf(out, "    + %s\n", a)
		}
	}
	if len(result.Removed) == 0 && len(result.Added) == 0 {
		fmt.Fprintln(out, "  No owned removals or additions to apply.")
	}
	return nil
}

func init() {
	planCmd.Flags().StringVar(&planDir, "dir", "", "Project directory (default: current dir)")
	planCmd.Flags().BoolVar(&planApply, "apply", false, "Explicitly apply the Curation Plan (preflighted, atomic)")
	planCmd.Flags().BoolVar(&planJSON, "json", false, "Output machine-readable plan")
	planCmd.Flags().BoolVar(&planCheck, "check", false, "Exit nonzero if the plan is not satisfied (CI)")
	planCmd.Flags().StringVar(&planProfile, "profile", "", "Explicit curation target profile (for bootstrap projects)")
	planCmd.Flags().StringArrayVarP(&planSkills, "skill", "s", nil, "Explicit curation target skill(s) (for bootstrap projects)")
	planCmd.Flags().BoolVarP(&planYes, "yes", "y", false, "Skip confirmation (unused in current apply; kept for CLI parity)")
	rootCmd.AddCommand(planCmd)
}

// agentsForPlan 返回策划应用的目标代理（与 runPlanCommand 一致）。
func agentsForPlan() []tool.Tool {
	agents := tool.DetectInstalled(tool.AllTools())
	if len(agents) == 0 {
		return tool.DefaultTools()
	}
	return agents
}

// recordAddedOwnership 把本次新增安装登记进 .sm.json#curation.managed。
// additions 是技能名列表；扫描各代理项目目录，为每个真实落地该技能的代理登记。

func recordAddedOwnership(projectDir string, additions []string) error {
	pm := project.NewManager(projectDir)
	config, err := pm.Load()
	if err != nil {
		return err
	}
	if config.Curation == nil {
		config.Curation = &project.Curation{}
	}
	// 若无 baseline 快照，则用当前显式目标（顶层 profile+skills）补录，
	// 供后续计划复现（ADR 0021/0028）。
	if config.Curation.Baseline == nil && len(additions) > 0 {
		config.Curation.Baseline = &project.Baseline{
			Profile: config.Profile,
			Skills:  append([]string(nil), config.Skills...),
			MCP:     append([]string(nil), config.MCP...),
		}
	}
	for _, skill := range additions {
		if skill == "" {
			continue
		}
		hosts, err := agentsWithSkill(projectDir, skill)
		if err != nil {
			return err
		}
		for _, a := range hosts {
			config.Curation.AddOwned(a, skill)
		}
	}
	return pm.Save(config)
}

// agentsWithSkill 返回项目目录中真实拥有 skill 的代理名单。
func agentsWithSkill(projectDir, skill string) ([]string, error) {
	var hosts []string
	for _, t := range agentsForPlan() {
		dir := tool.GetProjectSkillDir(t, projectDir)
		if dir == "" {
			continue
		}
		link := filepath.Join(dir, skill)
		if info, err := os.Lstat(link); err == nil && info.Mode()&os.ModeSymlink != 0 {
			hosts = append(hosts, t.Name)
		}
	}
	return hosts, nil
}

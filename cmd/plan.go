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
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/curation"
	"github.com/woyin/skills-manager/internal/installer"
	"github.com/woyin/skills-manager/internal/profile"
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
		renderBootstrapPlan(out, plan)
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

	renderProposals(out, plan)

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

// renderBootstrapPlan 渲染 bootstrap 状态的计划：项目尚无显式 curation
// 目标，列出可选 profiles/skills 与警告，等待用户显式选择。
func renderBootstrapPlan(out io.Writer, plan *curation.Plan) {
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
}

// renderProposals 按 ADD → REMOVE → LEAVE 分组渲染变更提案；
// 同组内按 agent 名排序。REMOVE 项标注 owned / cleanup candidate。
func renderProposals(out io.Writer, plan *curation.Plan) {
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
		fmt.Fprintf(out, "%s:\n", actionLabel(act))
		for _, p := range list {
			fmt.Fprintf(out, "  %s/%s%s — %s\n", p.Agent, p.Skill, removalSuffix(p), p.Reason)
		}
	}
}

// actionLabel 返回提案分组的显示标题。
func actionLabel(act curation.Action) string {
	return map[curation.Action]string{
		curation.ActionAdd:    "ADD",
		curation.ActionRemove: "REMOVE",
		curation.ActionLeave:  "LEAVE",
	}[act]
}

// removalSuffix 返回 REMOVE 提案的所有权标注；其余提案为空。
func removalSuffix(p curation.Proposal) string {
	if p.Action != curation.ActionRemove {
		return ""
	}
	if p.Owned {
		return " [owned]"
	}
	return " [cleanup candidate, not auto-removable]"
}

// applyPlan 应用计划。新增与 owned 移除纳入同一安装事务，.sm.json 由事务单点写盘。
//
// 两条路径共用 curation.Apply 的事务编排：
//  1. curated 项目：BeginRegistryInstall 落地缺失 baseline 成员；
//  2. bootstrap 项目：BeginInstall 落地用户选定的显式目标（ADR 0028）。
func applyPlan(plan *curation.Plan, projectDir string) error {
	if plan.Bootstrap {
		return applyBootstrapTarget(plan, projectDir)
	}

	opts := curation.ApplyOptions{
		ApplyRemovals: true,
		BeginAdditions: func(skills []string) (curation.AdditionTx, error) {
			return beginPlanTransaction(projectDir, skills)
		},
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
	return reportApplyResult(plan, result, projectDir)
}

// applyBootstrapTarget 为 bootstrap 项目应用用户选定的显式目标。
// 安装、移除、baseline 与 ownership 都在同一事务里完成（ADR 0028）。
func applyBootstrapTarget(plan *curation.Plan, projectDir string) error {
	if planProfile == "" && len(planSkills) == 0 {
		return fmt.Errorf("bootstrap plan requires an explicit curation target (use --profile <name> or --skill <name)")
	}

	effectiveSkills, err := bootstrapEffectiveSkills(planProfile, planSkills)
	if err != nil {
		return err
	}

	if !planJSON {
		if err := renderPlanHuman(plan); err != nil {
			return err
		}
	}

	opts := curation.ApplyOptions{
		ApplyRemovals: true,
		ExplicitTarget: &curation.ExplicitTarget{
			Profile:        planProfile,
			Skills:         effectiveSkills,
			TopLevelSkills: append([]string(nil), planSkills...),
		},
		BeginAdditions: func([]string) (curation.AdditionTx, error) {
			inst, err := installer.New(RegistryDir, ProfilesDir, agentsForPlan())
			if err != nil {
				return nil, err
			}
			inst.SetScope(projectDir, false)
			tx, err := inst.BeginInstall(projectDir, planProfile, planSkills, nil)
			if err != nil {
				return nil, err
			}
			return &planTx{tx: tx}, nil
		},
	}

	result, err := plan.Apply(opts)
	if err != nil {
		return fmt.Errorf("installing explicit curation target: %w", err)
	}
	return reportApplyResult(plan, result, projectDir)
}

// beginPlanTransaction 打开一个 registry 安装事务（curated 路径）。
func beginPlanTransaction(projectDir string, skills []string) (curation.AdditionTx, error) {
	inst, err := installer.New(RegistryDir, ProfilesDir, agentsForPlan())
	if err != nil {
		return nil, err
	}
	inst.SetScope(projectDir, false)
	tx, err := inst.BeginRegistryInstall(skills, "")
	if err != nil {
		return nil, err
	}
	return &planTx{tx: tx}, nil
}

// bootstrapEffectiveSkills 返回 bootstrap 显式目标组成的有效技能名
// （profile 引用的技能展开 + `--skill` 附加项，去重）。
func bootstrapEffectiveSkills(profileName string, skills []string) ([]string, error) {
	loader := profile.NewLoader(ProfilesDir)
	var out []string
	if profileName != "" {
		p, err := loader.Load(profileName)
		if err != nil {
			return nil, fmt.Errorf("loading curation target profile %q: %w", profileName, err)
		}
		out = append(out, p.Skills...)
	}
	out = append(out, skills...)
	// 去重
	seen := map[string]bool{}
	var uniq []string
	for _, s := range out {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		uniq = append(uniq, s)
	}
	return uniq, nil
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

// planTx 把 installer 的安装事务适配为 curation.AdditionTx。
// 只有真正的符号链接实体可被登记为 owned（ADR 0023）。
type planTx struct {
	tx        *installer.InstallTransaction
	committed bool
}

func (p *planTx) Added() []curation.AddedEntity {
	placements := p.tx.ChangedPlacements()
	out := make([]curation.AddedEntity, 0, len(placements))
	for _, pl := range placements {
		out = append(out, curation.AddedEntity{
			Agent:   pl.Agent,
			Skill:   pl.Skill,
			Path:    pl.Destination,
			Symlink: pl.ActualMode == installer.SymlinkMode,
		})
	}
	return out
}

func (p *planTx) StageProjectConfig(cfg *project.Config) error { return p.tx.StageProjectConfig(cfg) }

// Committed 报告内容是否已提交。final 协议不再提供 Committed()，
// 由适配器按 Commit 结果推断：nil 或 *CommitCleanupError 表示内容已提交。
func (p *planTx) Committed() bool { return p.committed }

func (p *planTx) Commit() error {
	err := p.tx.Commit()
	if err == nil {
		p.committed = true
		return nil
	}
	var cleanup *installer.CommitCleanupError
	if errors.As(err, &cleanup) {
		p.committed = true
	}
	return err
}

func (p *planTx) Rollback() error { return p.tx.Rollback() }

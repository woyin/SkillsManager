package curation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/woyin/skills-manager/internal/project"
)

// ApplyResult 汇总一次计划应用的结果。
type ApplyResult struct {
	Removed []string // 已移除的 (agent/skill)
	Added   []string // 本次实际安装成功的技能名
}

// ApplyOptions 控制应用行为。
type ApplyOptions struct {
	// ApplyRemovals 为 true 时才执行移除；默认 false（仅报告）。
	ApplyRemovals bool
	// ExplicitTarget 为 bootstrap 计划提供显式目标（profile 或技能名）。
	// 非空表示用户已选定目标（ADR 0028）。
	ExplicitTarget *ExplicitTarget
	// BeginAdditions 开启新增安装事务：返回延迟提交的句柄，
	// curation 在 owned 移除与 ownership 写盘都成功后才 Commit。
	// 为 nil 时不安装新增项。
	BeginAdditions func(skills []string) (AdditionTx, error)
}

// ExplicitTarget 是 bootstrap 计划的显式目标。
//   - Skills 为 profile 展开后的有效技能名（用于安装实际落地）；
//   - TopLevelSkills 为 `--skill` 的原样值，写入 .sm.json 顶层 skills
//     （与既有命令行为一致：profile 展开不落到顶层）。
type ExplicitTarget struct {
	Profile        string
	Skills         []string
	TopLevelSkills []string
}

// Apply 应用计划。
// 事务范围：新增安装（由句柄表达）→ owned 移除 → 一次写 .sm.json（baseline+managed）
// → Commit。任一前置步骤失败时逆序补偿本次新增与本次移除；Commit 返回
// “内容已提交”类错误时不得撤销移除。只移除 owned Link Install（ADR 0023）。
func (pl *Plan) Apply(opts ApplyOptions) (*ApplyResult, error) {
	result := &ApplyResult{}

	// 若为 bootstrap 且未提供显式目标，拒绝变更（ADR 0028）。
	if pl.Bootstrap && opts.ExplicitTarget == nil {
		return result, fmt.Errorf("bootstrap plan requires an explicit target before applying (use --profile or --skill)")
	}

	removals, additions := partitionProposals(pl.Proposals)
	if pl.Bootstrap {
		additions = explicitTargetProposals(opts.ExplicitTarget)
	}

	// preflight：只允许真实存在的符号链接被移除，同路径只处理一次。
	if opts.ApplyRemovals {
		preflighted, err := preflightRemovals(removals)
		if err != nil {
			return result, err
		}
		removals = preflighted
	}

	// 先读取配置：旧 .sm.json 损坏时在任何写入前失败（零副作用）。
	cfg, err := pl.loadConfig()
	if err != nil {
		return result, err
	}

	// 落地新增（延迟提交）。bootstrap 的显式目标可能只含 MCP（无技能），
	// 此时也必须开启事务，否则 profile 的 MCP 会漏装。
	var tx AdditionTx
	if opts.BeginAdditions != nil && (len(additions) > 0 || pl.Bootstrap) {
		started, err := opts.BeginAdditions(additionNames(additions))
		if err != nil {
			// 契约上 Begin* 失败应已自行补偿并返回 nil 事务；若仍返回句柄，
			// 这里必须补偿，避免遗留本次已落地的实体。
			if started != nil {
				if rerr := started.Rollback(); rerr != nil {
					return result, errors.Join(fmt.Errorf("installing baseline additions: %w", err), fmt.Errorf("rolling back additions: %w", rerr))
				}
			}
			return result, fmt.Errorf("installing baseline additions: %w", err)
		}
		tx = started
	}

	rollbackAdditions := func(errs ...error) error {
		if tx != nil {
			if rerr := tx.Rollback(); rerr != nil {
				errs = append(errs, fmt.Errorf("rolling back additions: %w", rerr))
			}
		}
		return errors.Join(errs...)
	}

	// 执行移除。
	removed, removedResult, err := removeOwnedLinks(removals, opts.ApplyRemovals)
	if err != nil {
		return result, rollbackAdditions(err)
	}

	restoreRemovals := func(errs ...error) error {
		if rerr := rollbackRemovedLinks(removed); rerr != nil {
			errs = append(errs, fmt.Errorf("restoring removed links: %w", rerr))
		}
		return errors.Join(errs...)
	}

	// 配置与 ownership：只对实际删除的实体清理 managed，再一次合并、单点写盘。
	appliedRemovals := make([]Proposal, 0, len(removed))
	for _, pr := range removals {
		for _, r := range removed {
			if filepath.Clean(r.path) == filepath.Clean(pr.Path) {
				appliedRemovals = append(appliedRemovals, pr)
				break
			}
		}
	}
	// 无新增事务且没有任何实际移除时不写盘：纯报告与已满足的 --apply
	// 必须保持 .sm.json 字节不变（避免凭空写入空 curation 块）。
	if tx == nil && len(appliedRemovals) == 0 {
		return result, nil
	}

	added, err := pl.mergeOwnership(cfg, appliedRemovals, tx, opts)
	if err != nil {
		return result, restoreRemovals(rollbackAdditions(err))
	}
	if err := pl.commitConfig(tx, cfg); err != nil {
		if tx != nil && tx.Committed() {
			// 内容已提交，仅清理未完成：不得撤销移除或新增，否则状态不一致；
			// 因此这里同样报告已移除与已新增。
			result.Removed = removedResult
			result.Added = added
			return result, fmt.Errorf("curation content committed but cleanup incomplete: %w", err)
		}
		return result, restoreRemovals(rollbackAdditions(err))
	}

	// 只有真正完成（含 cleanup 已提交）才报告已移除与已新增。
	result.Removed = removedResult
	result.Added = added
	return result, nil
}

// explicitTargetProposals 把 bootstrap 的显式目标展开为新增提案。
func explicitTargetProposals(target *ExplicitTarget) []Proposal {
	if target == nil {
		return nil
	}
	out := make([]Proposal, 0, len(target.Skills))
	for _, s := range target.Skills {
		if s == "" {
			continue
		}
		out = append(out, Proposal{Action: ActionAdd, Skill: s, Reason: "explicit curation target"})
	}
	return out
}

// loadConfig 读取当前 .sm.json，作为本次合并 baseline/managed 的基底。
// 协议 final 不再提供 DesiredConfig：配置内容由 curation 全权构造。
func (pl *Plan) loadConfig() (*project.Config, error) {
	return project.NewManager(pl.Project).Load()
}

// commitConfig 单点写盘：有句柄时 Stage 后 Commit（由 installer 原子写），
// 无句柄时直接保存（仅移除的场景）。
func (pl *Plan) commitConfig(tx AdditionTx, cfg *project.Config) error {
	if tx != nil {
		if err := tx.StageProjectConfig(cfg); err != nil {
			return fmt.Errorf("staging project config: %w", err)
		}
		return tx.Commit()
	}
	return project.NewManager(pl.Project).Save(cfg)
}

// partitionProposals 把提案按处置动作分为 owned 移除与添加两组。
func partitionProposals(proposals []Proposal) (removals, additions []Proposal) {
	for _, pr := range proposals {
		switch pr.Action {
		case ActionRemove:
			if pr.Owned {
				removals = append(removals, pr)
			}
		case ActionAdd:
			additions = append(additions, pr)
		}
	}
	return removals, additions
}

// additionNames 提取添加提案的技能名列表。
func additionNames(additions []Proposal) []string {
	names := make([]string, 0, len(additions))
	for _, pr := range additions {
		names = append(names, pr.Skill)
	}
	return names
}

// removedLink 记录一个已移除的 owned Link Install，供回滚恢复。
type removedLink struct {
	path   string
	target string
}

// removeOwnedLinks 逐个移除 owned Link Install，先读原目标再删除。
// apply 为 false 时不做任何事。任一步骤失败时回滚已移除的全部链接，
// 并把恢复失败一并返回（不再静默丢弃）。
func removeOwnedLinks(removals []Proposal, apply bool) ([]removedLink, []string, error) {
	if !apply {
		return nil, nil, nil
	}
	var removed []removedLink
	var result []string
	for _, pr := range removals {
		target, readErr := os.Readlink(pr.Path)
		if readErr != nil {
			return removed, result, errors.Join(
				fmt.Errorf("reading %s before removal: %w", pr.Path, readErr),
				rollbackRemovedLinks(removed),
			)
		}
		if err := os.Remove(pr.Path); err != nil {
			return removed, result, errors.Join(
				fmt.Errorf("removing %s: %w", pr.Path, err),
				rollbackRemovedLinks(removed),
			)
		}
		removed = append(removed, removedLink{path: pr.Path, target: target})
		result = append(result, pr.Agent+"/"+pr.Skill)
	}
	return removed, result, nil
}

// rollbackRemovedLinks 逆序恢复已移除的链接；失败不再静默，返回聚合错误。
func rollbackRemovedLinks(removed []removedLink) error {
	var errs []error
	for i := len(removed) - 1; i >= 0; i-- {
		path := filepath.Clean(removed[i].path)
		if _, err := os.Lstat(path); err == nil {
			continue // 已恢复
		}
		if err := os.Symlink(removed[i].target, path); err != nil {
			errs = append(errs, fmt.Errorf("restoring %s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}

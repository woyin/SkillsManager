package curation

import (
	"fmt"
	"os"

	"github.com/woyin/skills-manager/internal/project"
)

// ApplyResult 汇总一次计划应用的结果。
type ApplyResult struct {
	Removed []string // 已移除的 (agent/skill)
	Added   []string // 已添加/新增的技能名（宿主代理由调用方扫描确定）
}

// ApplyOptions 控制应用行为。
type ApplyOptions struct {
	// ApplyRemovals 为 true 时才执行移除；默认 false（仅报告）。
	ApplyRemovals bool
	// ExplicitTarget 为 bootstrap 计划提供显式目标（profile 或技能名）。
	// 非空表示用户已选定目标（ADR 0028）。
	ExplicitTarget *ExplicitTarget
	// InstallAdditions 为 nil 时，添加项仅被记录而不实际安装
	// （调用方可在生成后自行决定是否交由 install 流程落地）。
	InstallAdditions func(skills []string) error
}

// ExplicitTarget 是 bootstrap 计划的显式目标。
type ExplicitTarget struct {
	Profile string
	Skills  []string
}

// Apply 应用计划。返回是否发生了任何变更。
// 原子性：先 preflight 全部，再执行；执行阶段失败即回滚本次已做变更。
// 只移除 owned Link Install（ADR 0023）；未拥有/Copy/手动实体永不移除。
func (pl *Plan) Apply(opts ApplyOptions) (*ApplyResult, error) {
	result := &ApplyResult{}

	// 若为 bootstrap 且未提供显式目标，拒绝变更（ADR 0028）。
	if pl.Bootstrap && opts.ExplicitTarget == nil {
		return result, fmt.Errorf("bootstrap plan requires an explicit target before applying (use --profile or --skill)")
	}

	// preflight：收集要移除与要添加的实体。
	removals, additions := partitionProposals(pl.Proposals)

	// 先落地缺失的 baseline 成员（添加），再做移除：若添加失败，
	// 命令在此中止且尚未移除任何东西，保持接近原子的顺序（ADR 0020）。
	// 新增项以技能名记录（宿主代理由调用方扫描确定）。
	result.Added = additionNames(additions)
	if err := installAdditions(opts.InstallAdditions, additions); err != nil {
		return result, err
	}

	// 执行移除（仅在允许时）。移除是事务性的：每移除一个 owned Link Install 前
	// 先记录其原目标，若中途失败或后续 .sm.json 写盘失败，则把已移除的全部
	// 链接恢复原状（原子性，ADR 0020）。
	removed, removedResult, err := removeOwnedLinks(removals, opts.ApplyRemovals)
	if err != nil {
		return result, err
	}
	result.Removed = removedResult

	// 更新 .sm.json 的 managed：移除的 owned 项从 managed 删除。
	// 该写盘失败时回滚全部已移除的链接，保证应用为原子操作。
	if len(removed) > 0 {
		if err := pl.syncManagedAfterRemoval(removals); err != nil {
			rollbackRemovedLinks(removed)
			return result, fmt.Errorf("updating curation ownership: %w (all changes rolled back)", err)
		}
	}

	return result, nil
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

// installAdditions 通过 InstallAdditions 回调落地添加项；回调为 nil
// 或没有添加项时不做任何事。
func installAdditions(install func([]string) error, additions []Proposal) error {
	if install == nil || len(additions) == 0 {
		return nil
	}
	if err := install(additionNames(additions)); err != nil {
		return fmt.Errorf("installing baseline additions: %w", err)
	}
	return nil
}

// removedLink 记录一个已移除的 owned Link Install，供回滚恢复。
type removedLink struct {
	path   string
	target string
}

// removeOwnedLinks 逐个移除 owned Link Install，先读原目标再删除。
// apply 为 false 时不做任何事。任一步骤失败时回滚已移除的全部链接。
// 返回已移除的链接记录、移除结果列表，以及错误。
func removeOwnedLinks(removals []Proposal, apply bool) ([]removedLink, []string, error) {
	if !apply {
		return nil, nil, nil
	}
	var removed []removedLink
	var result []string
	for _, pr := range removals {
		target, readErr := os.Readlink(pr.Path)
		if readErr != nil {
			rollbackRemovedLinks(removed)
			return removed, result, fmt.Errorf("reading %s before removal: %w (all changes rolled back)", pr.Path, readErr)
		}
		if err := os.Remove(pr.Path); err != nil {
			rollbackRemovedLinks(removed)
			return removed, result, fmt.Errorf("removing %s: %w (all changes rolled back)", pr.Path, err)
		}
		removed = append(removed, removedLink{path: pr.Path, target: target})
		result = append(result, pr.Agent+"/"+pr.Skill)
	}
	return removed, result, nil
}

// rollbackRemovedLinks 逆序恢复已移除的链接（每项只恢复一次）。
func rollbackRemovedLinks(removed []removedLink) {
	for i := len(removed) - 1; i >= 0; i-- {
		_ = os.Symlink(removed[i].target, removed[i].path)
	}
}

// syncManagedAfterRemoval 从 .sm.json#curation.managed 中移除已删除的 owned 项。
func (pl *Plan) syncManagedAfterRemoval(removals []Proposal) error {
	pm := project.NewManager(pl.Project)
	config, err := pm.Load()
	if err != nil {
		return err
	}
	if config.Curation == nil {
		return nil
	}
	for agent := range config.Curation.Managed {
		keep := config.Curation.Managed[agent][:0]
		for _, name := range config.Curation.Managed[agent] {
			removed := false
			for _, pr := range removals {
				if pr.Agent == agent && pr.Skill == name {
					removed = true
					break
				}
			}
			if !removed {
				keep = append(keep, name)
			}
		}
		if len(keep) == 0 {
			delete(config.Curation.Managed, agent)
		} else {
			config.Curation.Managed[agent] = keep
		}
	}
	return pm.Save(config)
}

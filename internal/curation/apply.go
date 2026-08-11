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
	var removals []Proposal
	var additions []Proposal
	for _, pr := range pl.Proposals {
		switch pr.Action {
		case ActionRemove:
			if pr.Owned {
				removals = append(removals, pr)
			}
		case ActionAdd:
			additions = append(additions, pr)
		}
	}

	// 先落地缺失的 baseline 成员（添加），再做移除：若添加失败，
	// 命令在此中止且尚未移除任何东西，保持接近原子的顺序（ADR 0020）。
	// 新增项以技能名记录（宿主代理由调用方扫描确定）。
	for _, pr := range additions {
		result.Added = append(result.Added, pr.Skill)
	}
	if opts.InstallAdditions != nil && len(additions) > 0 {
		var missing []string
		for _, pr := range additions {
			missing = append(missing, pr.Skill)
		}
		if err := opts.InstallAdditions(missing); err != nil {
			return result, fmt.Errorf("installing baseline additions: %w", err)
		}
	}

	// 执行移除（仅在允许时）。
	appliedRemovals := 0
	if opts.ApplyRemovals {
		for _, pr := range removals {
			if err := os.Remove(pr.Path); err != nil {
				// 移除是幂等的、可重复，直接报错中止。
				return result, fmt.Errorf("removing %s: %w", pr.Path, err)
			}
			result.Removed = append(result.Removed, pr.Agent+"/"+pr.Skill)
			appliedRemovals++
		}
	}

	// 更新 .sm.json 的 managed：移除的 owned 项从 managed 删除（它们已不在磁盘）。
	// 添加项由 install 后的后续 confirm 记录；此处不做。
	if appliedRemovals > 0 {
		if err := pl.syncManagedAfterRemoval(removals); err != nil {
			return result, err
		}
	}

	return result, nil
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

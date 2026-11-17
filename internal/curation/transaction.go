package curation

// 本文件定义 Curation 消费的安装事务窄接口（A1 句柄由 cmd 层适配），
// 以及 ownership 的事务性合并与校验（ADR 0023）。

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/tool"
)

// AddedEntity 是本次安装事务实际改变的一个实体（幂等既有实体不会出现）。
type AddedEntity struct {
	Agent   string
	Skill   string
	Path    string
	Symlink bool
}

// AdditionTx 是延迟提交的安装事务（A1 提供，cmd 适配）。
// 契约：
//   - Commit 内容步失败时 Committed() 为 false，且内部已补偿本次新增；
//   - Commit 返回错误但 Committed() 为 true 表示内容已提交、仅清理未完成；
//   - Commit 之前调用 Rollback 撤销本次新增。
type AdditionTx interface {
	Added() []AddedEntity
	StageProjectConfig(*project.Config) error
	Committed() bool
	Commit() error
	Rollback() error
}

// validateAdded 校验一条新增实体是否允许登记为计划拥有（ADR 0023）。
func (pl *Plan) validateAdded(e AddedEntity) error {
	if e.Agent == "" || e.Skill == "" || e.Path == "" {
		return fmt.Errorf("ownership: incomplete added entity (agent=%q skill=%q path=%q)", e.Agent, e.Skill, e.Path)
	}
	if !e.Symlink {
		return fmt.Errorf("ownership: refusing to record %s/%s: not a Link Install", e.Agent, e.Skill)
	}
	t := tool.ToolByName(e.Agent)
	if t == nil {
		return fmt.Errorf("ownership: unknown agent %q", e.Agent)
	}
	dir := tool.GetProjectSkillDir(*t, pl.Project)
	if dir == "" {
		return fmt.Errorf("ownership: agent %q has no project skill directory", e.Agent)
	}
	want := filepath.Join(dir, e.Skill)
	if filepath.Clean(e.Path) != filepath.Clean(want) {
		return fmt.Errorf("ownership: %s is not the project-scope link for %s/%s (want %s)", e.Path, e.Agent, e.Skill, want)
	}
	if !symlinkPointsInside(e.Path, pl.registryDir) {
		return fmt.Errorf("ownership: %s does not point inside the registry", e.Path)
	}
	return nil
}

// removeManaged 从 curation.managed 删除一条 (agent, skill) 记录。
func removeManaged(c *project.Curation, agent, skill string) {
	if c == nil || c.Managed == nil {
		return
	}
	names := c.Managed[agent]
	keep := names[:0]
	for _, n := range names {
		if n != skill {
			keep = append(keep, n)
		}
	}
	if len(keep) == 0 {
		delete(c.Managed, agent)
		return
	}
	c.Managed[agent] = keep
}

// mergeOwnership 在 config 上一次性完成 managed 增删与 baseline 快照，
// 只登记本次实际 changed 且校验通过的 Link Install。
func (pl *Plan) mergeOwnership(cfg *project.Config, removals []Proposal, tx AdditionTx, opts ApplyOptions) ([]string, error) {
	if cfg.Curation == nil {
		cfg.Curation = &project.Curation{}
	}
	// 只对本次实际删除的实体清理 managed：链接仍在磁盘上时必须保留所有权。
	for _, pr := range removals {
		removeManaged(cfg.Curation, pr.Agent, pr.Skill)
		for _, agent := range pr.agents {
			removeManaged(cfg.Curation, agent, pr.Skill)
		}
	}

	var added []string
	if tx != nil {
		seen := map[string]bool{}
		for _, e := range tx.Added() {
			if err := pl.validateAdded(e); err != nil {
				return nil, err
			}
			cfg.Curation.AddOwned(e.Agent, e.Skill)
			key := e.Agent + "/" + e.Skill
			if !seen[key] {
				seen[key] = true
				added = append(added, e.Skill)
			}
		}
	}

	if pl.Bootstrap && opts.ExplicitTarget != nil {
		// baseline.Skills 只记录 --skill 额外项；profile 展开的成员由 Profile
		// 字段表达，避免把成员固化进 Baseline 后 profile 变更不再移除。
		cfg.Curation.Baseline = &project.Baseline{
			Profile: opts.ExplicitTarget.Profile,
			Skills:  append([]string(nil), opts.ExplicitTarget.TopLevelSkills...),
		}
		cfg.Profile = opts.ExplicitTarget.Profile
		cfg.Skills = append([]string(nil), opts.ExplicitTarget.TopLevelSkills...)
	} else if cfg.Curation.Baseline == nil && pl.Baseline != nil {
		cfg.Curation.Baseline = &project.Baseline{
			Profile: pl.Baseline.Profile,
			Skills:  append([]string(nil), pl.Baseline.Skills...),
			MCP:     append([]string(nil), pl.Baseline.MCP...),
		}
	}
	return dedupe(added), nil
}

// preflightRemovals 在执行任何写操作前校验移除对象：只能是真实存在的符号链接，
// 同一路径只处理一次（共享项目目录）。
func preflightRemovals(removals []Proposal) ([]Proposal, error) {
	seen := map[string]bool{}
	out := make([]Proposal, 0, len(removals))
	for _, pr := range removals {
		path := filepath.Clean(pr.Path)
		if path == "" {
			return nil, fmt.Errorf("removal preflight: empty path for %s/%s", pr.Agent, pr.Skill)
		}
		if seen[path] {
			continue
		}
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("removal preflight %s: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return nil, fmt.Errorf("removal preflight: %s is not a Link Install", path)
		}
		seen[path] = true
		out = append(out, pr)
	}
	return out, nil
}

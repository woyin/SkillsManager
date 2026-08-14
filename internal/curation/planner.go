package curation

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/woyin/skills-manager/internal/profile"
	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

// InstalledSkill 描述项目某代理目录下一个已安装的技能实体。
type InstalledSkill struct {
	Agent string
	Name  string
	Path  string
	Kind  string // symlink | copy | other
	// LinkInstall 为该实体是 Link Install（符号链接且目标在注册表内）。
	LinkInstall bool
}

// Planner 为项目生成 Curation Plan。操作注册表、profiles、项目工具集。
type Planner struct {
	registry   *registry.Registry
	profiles   *profile.Loader
	tools      []tool.Tool
	projectDir string
}

// NewPlanner 返回一个 Planner。
func NewPlanner(registryDir, profilesDir string, tools []tool.Tool, projectDir string) *Planner {
	return &Planner{
		registry:   registry.New(registryDir),
		profiles:   profile.NewLoader(profilesDir),
		tools:      tools,
		projectDir: projectDir,
	}
}

// Plan 是策划结果：baseline、proposals、check 状态、bootstrap 标记。
type Plan struct {
	Project           string     `json:"project"`
	Baseline          *Baseline  `json:"baseline"`
	Bootstrap         bool       `json:"bootstrap"`
	Proposals         []Proposal `json:"proposals"`
	Warnings          []string   `json:"warnings,omitempty"`
	CheckOK           bool       `json:"check"`
	ExplicitTargetSet bool       `json:"has_explicit_target"`
	// 下面供 bootstrap 的本地证据建议（ADR 0027/0028）。
	RecommendedProfiles []string `json:"recommended_profiles,omitempty"`
	RecommendedSkills   []string `json:"recommended_skills,omitempty"`
}

// PlanForProject 生成计划。resolveProfile 展开 profile 里引用的技能名
// （profile 名 → 其 Skills），可为 nil 表示不解锁/禁用。当 .sm.json 已有
// 显式 profile 时的展开由调用方提供，否则留空。
func (p *Planner) PlanForProject() (*Plan, error) {
	pm := project.NewManager(p.projectDir)
	config, err := pm.Load()
	if err != nil {
		return nil, err
	}

	bl := &Baseline{
		Profile:    config.Profile,
		Skills:     config.Skills,
		MCP:        config.MCP,
		HasProfile: config.Profile != "",
	}
	// 一个已设置 Curation 块（managed/baseline）的项目曾有过显式目标
	//（ADR 0028）：不再视为 bootstrap，即便顶层 profile/skills 为空。
	curatedProject := config.Curation != nil
	targetSkills := []string{}
	if curatedProject && len(config.Skills) == 0 && config.Profile == "" && config.Curation.Baseline != nil {
		bl = &Baseline{
			Profile:    config.Curation.Baseline.Profile,
			Skills:     config.Curation.Baseline.Skills,
			MCP:        config.Curation.Baseline.MCP,
			HasProfile: config.Curation.Baseline.Profile != "",
		}
	}
	isBootstrap := bl.IsBootstrap() && !curatedProject
	plan := &Plan{
		Project:   p.projectDir,
		Baseline:  bl,
		Bootstrap: isBootstrap,
	}
	plan.ExplicitTargetSet = !isBootstrap

	installed := p.scanProjectInstalls()

	// 展开 profile 引用的技能。
	profileSkills, pierr := p.expandProfile(bl.Profile)
	if pierr != nil && bl.HasProfile {
		plan.Warnings = append(plan.Warnings, "profile "+bl.Profile+": "+pierr.Error())
	}
	targetSkills = bl.ExpandSkillNames(profileSkills)

	// bootstrap（无显式目标）：只给建议，不产生 add/remove。
	if isBootstrap {
		plan.CheckOK = true
		if profiles, err := p.profiles.List(); err == nil {
			plan.RecommendedProfiles = profiles
		}
		if skills, err := p.registry.ListSkills(); err == nil {
			plan.RecommendedSkills = skills[registry.Global]
		}
		return plan, nil
	}

	// 判定每条已装实体的归属（ADR 0023）。
	for _, inst := range installed {
		inBaseline := contains(targetSkills, inst.Name)
		owned := config.Curation != nil && config.Curation.IsOwned(inst.Agent, inst.Name)
		pp := Proposal{
			Skill: inst.Name,
			Agent: inst.Agent,
			Path:  inst.Path,
		}
		switch {
		case inBaseline:
			pp.Action = ActionLeave
			pp.Reason = "in baseline"
		case !inst.LinkInstall:
			pp.Action = ActionLeave
			pp.Reason = "not in baseline; not an owned Link Install (left in place)"
		case owned:
			pp.Action = ActionRemove
			pp.Reason = "not in baseline"
			pp.Owned = true
		default:
			pp.Action = ActionRemove
			pp.Reason = "not in baseline; unowned Link Install (cleanup candidate, not auto-removable)"
			pp.Owned = false
		}
		plan.Proposals = append(plan.Proposals, pp)
	}

	// 提议添加缺失的 baseline 成员。
	for _, name := range targetSkills {
		if !installedForName(installed, name) {
			plan.Proposals = append(plan.Proposals, Proposal{
				Action: ActionAdd,
				Skill:  name,
				Reason: "in baseline but not installed",
			})
		}
	}

	plan.CheckOK = !plan.HasUnsatisfiedRequired()
	return plan, nil
}

// expandProfile 返回 profile 引用的技能名；profile 名称空或加载失败返回 (nil, err)。
func (p *Planner) expandProfile(name string) ([]string, error) {
	if name == "" {
		return nil, nil
	}
	prof, err := p.profiles.Load(name)
	if err != nil {
		return nil, err
	}
	return prof.Skills, nil
}

func installedForName(installed []InstalledSkill, name string) bool {
	for _, i := range installed {
		if i.Name == name {
			return true
		}
	}
	return false
}

// HasUnsatisfiedRequired 报告是否存在必须满足而未满足的必要项。
// `sm plan --check` 唯一使 CI 失败的真值源：任一处于 baseline 的成员尚
//未安装（即有 ADD 拟改）即视为未满足。这与帮助文本"unless the plan is
// satisfied"及 ADR 0025 的 --check 语义一致。
func (pl *Plan) HasUnsatisfiedRequired() bool {
	for _, pr := range pl.Proposals {
		if pr.Action == ActionAdd {
			return true
		}
	}
	return false
}

// NeedsAction 报告计划是否包含需要用户动作的项（add 或 owned remove）。
func (pl *Plan) NeedsAction() bool {
	for _, pr := range pl.Proposals {
		if pr.Action != ActionLeave {
			return true
		}
	}
	return false
}

// scanProjectInstalls 扫描各工具项目级目录下的已装实体。
func (p *Planner) scanProjectInstalls() []InstalledSkill {
	regDir := p.registry.Dir()
	var out []InstalledSkill
	for _, t := range p.tools {
		dir := tool.GetProjectSkillDir(t, p.projectDir)
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Name() == ".gitkeep" {
				continue
			}
			linkPath := filepath.Join(dir, e.Name())
			info, err := os.Lstat(linkPath)
			if err != nil {
				continue
			}
			inst := InstalledSkill{Agent: t.Name, Name: e.Name(), Path: linkPath}
			if info.Mode()&os.ModeSymlink != 0 {
				inst.Kind = "symlink"
				inst.LinkInstall = symlinkPointsInside(linkPath, regDir)
			} else {
				inst.Kind = "other"
			}
			out = append(out, inst)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Agent != out[j].Agent {
			return out[i].Agent < out[j].Agent
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// symlinkPointsInside 判断 linkPath 为符号链接且目标落在 root 之内。
func symlinkPointsInside(linkPath, root string) bool {
	target, err := os.Readlink(linkPath)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(linkPath), target)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return false
	}
	return rel != ".." && rel != "." && !isParentRel(rel)
}

func isParentRel(rel string) bool {
	return len(rel) >= 3 && rel[:3] == ".."+string(os.PathSeparator)
}

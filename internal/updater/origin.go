// origin.go 承接 Registry Origin 的分组、source cache 刷新与原件回写。
// 一条内聚业务路径：同一 Source+Ref 的技能作为一组，先刷新/对齐 cache，
// 再用 updater.Apply 原子替换 Registry 原件并写入 registry schema 的 origin。
// 输出与范围选择留给 cmd。
//
// Input: fmt, os, os/exec, path/filepath, strings, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/internal/sourcecache
// Output: type OriginTarget, type OriginGroup, func (OriginGroup) IsPinned, func GroupOriginTargets, type TargetFailure, type GroupResult, type Options, func RefreshOriginGroup, func RewriteOriginTargets
// Pos: 业务层-Registry 来源刷新（分组/缓存/回写）
package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/sourcecache"
)

// OriginTarget 是一个待刷新的 Registry 原件及其 provenance。
type OriginTarget struct {
	Name     string
	SkillDir string
	Origin   registry.SkillOrigin
}

// OriginGroup 是同 Source+Ref 的一组技能；RefKind 决定是否 pinned。
type OriginGroup struct {
	Source  string
	Ref     string
	RefKind registry.RefKind
	Targets []OriginTarget
}

// IsPinned 报告该组是否 pinned（不自动前进）。
// tag/commit → pinned；default-branch/branch → tracking；
// 旧 metadata（refKind 未知）→ 非空 ref 保守视为 pinned（保留旧行为）。
func (g OriginGroup) IsPinned() bool {
	switch g.RefKind {
	case registry.RefTag, registry.RefCommit:
		return true
	case registry.RefBranch, registry.RefDefaultBranch:
		return false
	default:
		return g.Ref != ""
	}
}

// GroupOriginTargets 按 Source+Ref 分组，保持首次出现顺序。
func GroupOriginTargets(targets []OriginTarget) []OriginGroup {
	index := make(map[string]int, len(targets))
	var groups []OriginGroup
	for _, t := range targets {
		key := t.Origin.Source + "\x00" + t.Origin.Ref
		if i, ok := index[key]; ok {
			groups[i].Targets = append(groups[i].Targets, t)
			continue
		}
		index[key] = len(groups)
		groups = append(groups, OriginGroup{
			Source:  t.Origin.Source,
			Ref:     t.Origin.Ref,
			RefKind: t.Origin.RefKind,
			Targets: []OriginTarget{t},
		})
	}
	return groups
}

// TargetFailure 记录一个技能级失败。
type TargetFailure struct {
	Name     string
	SkillDir string
	Err      error
}

// GroupResult 汇总一组刷新的结果。
type GroupResult struct {
	Source       string
	Ref          string
	Pinned       bool
	CommitBefore string
	CommitAfter  string
	Written      int
	Failures     []TargetFailure
}

// Options 是模块的显式输入；调用方（cmd）提供 Registry 与数据目录。
type Options struct {
	RegistryDir string
	DataDir     string
}

// RefreshOriginGroup 刷新一组同 Source+Ref 的技能：
//   - pinned：不前进 cache，只确保 cache 存在并回写当前 pin 内容；
//   - tracking：拉取 cache（detached 视为 pinned，dirty/拉取失败为组级错误），
//     回写新内容。
//
// 返回的 error 只表示组级失败（cache 获取、pull、状态检查）；技能级失败进
// GroupResult.Failures，调用方据此保持 Source 隔离与计数。
func RefreshOriginGroup(g OriginGroup, opts Options) (GroupResult, error) {
	res := GroupResult{Source: g.Source, Ref: g.Ref}
	store := sourcecache.New(opts.DataDir)

	if g.IsPinned() {
		cache, err := store.Acquire(g.Source, g.Ref, false)
		if err != nil {
			return res, err
		}
		res.Pinned = true
		res.CommitBefore = HeadHash(cache.Path)
		res.CommitAfter = res.CommitBefore
		res.Written, res.Failures = RewriteOriginTargets(cache.Path, g.Targets, opts)
		return res, nil
	}

	cacheRef := ""
	if g.RefKind == registry.RefBranch {
		cacheRef = g.Ref
	}
	cache, err := store.Acquire(g.Source, cacheRef, false)
	if err != nil {
		return res, err
	}
	res.CommitBefore = HeadHash(cache.Path)

	if detached, err := IsDetached(cache.Path); err != nil {
		return res, err
	} else if detached {
		res.Pinned = true
		res.CommitAfter = res.CommitBefore
		res.Written, res.Failures = RewriteOriginTargets(cache.Path, g.Targets, opts)
		return res, nil
	}

	if dirty, err := IsDirty(cache.Path); err != nil {
		return res, err
	} else if dirty {
		return res, fmt.Errorf("local changes present in source cache")
	}
	if out, pullErr := exec.Command("git", "-C", cache.Path, "pull", "--ff-only").CombinedOutput(); pullErr != nil {
		return res, fmt.Errorf("%v\n%s", pullErr, out)
	}
	res.CommitAfter = HeadHash(cache.Path)

	// metadata 回写失败不影响内容刷新（保留既有行为）。
	_, metaPath := sourcecache.Paths(opts.DataDir, g.Source, "")
	meta, _ := sourcecache.ReadMetadata(metaPath)
	meta.Source = g.Source
	meta.Commit = res.CommitAfter
	_ = sourcecache.WriteMetadata(metaPath, meta)

	res.Written, res.Failures = RewriteOriginTargets(cache.Path, g.Targets, opts)
	return res, nil
}

// rewriteInfo 记录单个目标回写所需的 preflight 上下文。
type rewriteInfo struct {
	name         string
	origin       registry.SkillOrigin
	beforeHadErr bool
}

// RewriteOriginTargets 从 cacheDir 按 origin 子路径覆盖 Registry 原件。
// 同一 Source 的所有技能先统一 stage + lint，再一次性提交；任一技能失败
// （含 cache 内源路径缺失）时整组保持旧内容。返回成功写入数与该组失败项。
func RewriteOriginTargets(cacheDir string, targets []OriginTarget, opts Options) (int, []TargetFailure) {
	if len(targets) == 0 {
		return 0, nil
	}
	commit := HeadHash(cacheDir)
	reg := registry.New(opts.RegistryDir)

	plan := make([]Target, 0, len(targets))
	infos := make(map[string]rewriteInfo, len(targets))
	failures := make([]TargetFailure, 0)

	for _, t := range targets {
		src := cacheDir
		if rel := originSubPath(t.Origin); rel != "" && rel != "." {
			src = filepath.Join(cacheDir, rel)
		}
		if _, err := os.Stat(src); err != nil {
			failures = append(failures, TargetFailure{
				Name: t.Name, SkillDir: t.SkillDir,
				Err: fmt.Errorf("%s: skill path %q missing in cache: %w", t.Name, originSubPath(t.Origin), err),
			})
			continue
		}
		beforeHadErr := false
		if rel := registrySkillRel(opts.RegistryDir, t.SkillDir); rel != "" {
			beforeHadErr = reg.LintSkill(rel).HasErrors()
		}
		plan = append(plan, Target{Name: t.Name, SourceDir: src, Destination: t.SkillDir})
		infos[t.SkillDir] = rewriteInfo{name: t.Name, origin: t.Origin, beforeHadErr: beforeHadErr}
	}

	// 任一源缺失：整组不写，返回全部目标的失败（保持旧的“整组中止”语义）。
	if len(failures) > 0 {
		for _, t := range targets {
			already := false
			for _, f := range failures {
				if f.SkillDir == t.SkillDir {
					already = true
					break
				}
			}
			if !already {
				failures = append(failures, TargetFailure{Name: t.Name, SkillDir: t.SkillDir, Err: fmt.Errorf("source update aborted: sibling source path missing")})
			}
		}
		return 0, failures
	}

	_, err := Apply(plan, Hooks{
		Prepare: func(target Target, staged string) error {
			info := infos[target.Destination]
			// Source cache 不携带 Registry provenance：先删掉带入的 origin，
			// 再按 registry schema 写回。
			_ = os.Remove(filepath.Join(staged, registry.OriginFile))
			origin := info.origin
			origin.Commit = commit
			if err := reg.WriteOrigin(staged, origin); err != nil {
				return fmt.Errorf("origin write %s: %w", info.name, err)
			}
			return nil
		},
		Validate: func(target Target, staged string) error {
			info := infos[target.Destination]
			rel := registrySkillRel(opts.RegistryDir, staged)
			if rel == "" {
				return fmt.Errorf("staging path %q is outside registry", staged)
			}
			if info.beforeHadErr {
				return nil
			}
			if reg.LintSkill(rel).HasErrors() {
				return fmt.Errorf("updated skill %s failed validation", info.name)
			}
			return nil
		},
	})
	if err != nil {
		out := make([]TargetFailure, 0, len(plan))
		for _, t := range plan {
			out = append(out, TargetFailure{Name: t.Name, SkillDir: t.Destination, Err: err})
		}
		return 0, out
	}
	return len(plan), nil
}

// originSubPath 返回技能在 source cache 中的相对路径。
// 新 schema 用 SubPath；旧 schema 用 RelPath。
func originSubPath(o registry.SkillOrigin) string {
	if o.SubPath != "" {
		return o.SubPath
	}
	return o.RelPath
}

// registrySkillRel 返回 registry 技能绝对路径相对 <registryDir>/skills 的路径；
// 不在该子树内时返回空串。语义与 cmd 的 skillRelForLint 一致，但显式接收
// registryDir，避免模块依赖 cmd 全局变量。
func registrySkillRel(registryDir, skillAbs string) string {
	skillsRoot := filepath.Join(registryDir, "skills")
	try := func(skill, root string) string {
		rel, err := filepath.Rel(root, skill)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return ""
		}
		return rel
	}
	if rel := try(skillAbs, skillsRoot); rel != "" {
		return rel
	}
	absSkill, err1 := filepath.EvalSymlinks(skillAbs)
	absRoot, err2 := filepath.EvalSymlinks(skillsRoot)
	if err1 != nil || err2 != nil {
		return ""
	}
	return try(absSkill, absRoot)
}

// discovery.go 承接“已安装技能 → 可更新目标”的发现与分类。
// 一条内聚业务路径：扫描项目/全局 agent 技能目录，把每个已装条目归类为
// git 仓库、origin 承载的 registry 原件、Well-Known Source 技能或孤儿。
// 模块只接收显式目录与 scope 参数（不读 cmd 全局变量），也提供
// PathInside/NearestGitRepo 的单一实现；文案、退出码与交互留在 cmd。
//
// Input: os, path/filepath, strings, github.com/woyin/skills-manager/internal/lockfile, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/internal/symlink, github.com/woyin/skills-manager/internal/tool
// Output: type DiscoverOptions, type Diagnostic, type DiscoveredOrigin, type DiscoveredWellKnownSkill, type DiscoveredTargets, func (DiscoveredTargets) Empty, func DiscoverInstalledTargets, func PathInside, func NearestGitRepo
// Pos: 业务层-已安装技能更新目标发现
package updater

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/woyin/skills-manager/internal/lockfile"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
)

// DiscoverOptions 是发现阶段的显式输入。
// Options 提供 RegistryDir/DataDir；scope 与按名过滤由调用方决定。
type DiscoverOptions struct {
	Options
	ProjectDir     string
	Names          []string
	IncludeProject bool
	IncludeGlobal  bool
}

// Diagnostic 记录发现阶段被跳过的路径与原因。
// 与既有行为一致：扫描失败的目录不阻断整体发现，诊断只供调用方决定是否输出。
type Diagnostic struct {
	Op   string
	Path string
	Err  error
}

// DiscoveredOrigin 是带 legacy .sm-origin.json 的 registry 技能。
// Metadata 保留 UpdatedAt；registry schema 的视图仍由调用方按需读取。
type DiscoveredOrigin struct {
	Name     string
	SkillDir string
	Metadata OriginMetadata
}

// DiscoveredWellKnownSkill 是锁文件指向 Well-Known Source 的已装技能。
type DiscoveredWellKnownSkill struct {
	Name   string
	Source string
}

// DiscoveredTargets 是发现结果的四类目标，顺序与去重规则由发现流程决定。
type DiscoveredTargets struct {
	GitRepos        []string
	Origins         []DiscoveredOrigin
	WellKnownSkills []DiscoveredWellKnownSkill
	Orphans         []string
}

// Empty 报告是否没有任何已发现的目标（含孤儿）。
func (t DiscoveredTargets) Empty() bool {
	return len(t.GitRepos) == 0 && len(t.Origins) == 0 &&
		len(t.WellKnownSkills) == 0 && len(t.Orphans) == 0
}

// discoverer 持有一次发现过程的累积状态。
type discoverer struct {
	opts        DiscoverOptions
	targets     DiscoveredTargets
	diagnostics []Diagnostic
	seenRepo    map[string]bool
	seenOrigin  map[string]bool // skillDir / orphan key
}

// DiscoverInstalledTargets 扫描已安装技能，收集 git 仓库、origin-backed
// registry 技能、Well-Known Source 技能与孤儿。scope 由 IncludeProject/
// IncludeGlobal 决定；ProjectDir 为空时项目 scope 不生效（保留旧行为）。
func DiscoverInstalledTargets(opts DiscoverOptions) (DiscoveredTargets, []Diagnostic) {
	d := &discoverer{
		opts:       opts,
		seenRepo:   map[string]bool{},
		seenOrigin: map[string]bool{},
	}
	for _, t := range tool.AllTools() {
		if opts.IncludeProject && opts.ProjectDir != "" {
			if dir := tool.GetProjectSkillDir(t, opts.ProjectDir); dir != "" {
				d.collectDir(dir)
			}
		}
		if opts.IncludeGlobal {
			d.collectDir(tool.GetGlobalSkillDir(t))
		}
	}
	if opts.IncludeProject && opts.ProjectDir != "" {
		d.collectWellKnown()
	}
	return d.targets, d.diagnostics
}

// diagnose 记录一次跳过；不改写目标集合。
func (d *discoverer) diagnose(op, path string, err error) {
	d.diagnostics = append(d.diagnostics, Diagnostic{Op: op, Path: path, Err: err})
}

// collectDir 扫描某个 agent 技能目录，把其中每个已装技能归类写回结果。
// names 非空时只收集指定技能名。目录不存在是常态（该工具未安装技能），
// 只有真正不可读的目录才记诊断；两种情形都不中断发现。
func (d *discoverer) collectDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			d.diagnose("read-dir", dir, err)
		}
		return
	}
	nameSet := map[string]bool{}
	for _, n := range d.opts.Names {
		nameSet[n] = true
	}
	for _, e := range entries {
		if len(nameSet) > 0 && !nameSet[e.Name()] {
			continue
		}
		d.classifyDirEntry(e.Name(), filepath.Join(dir, e.Name()))
	}
}

// classifyDirEntry 判定单个已装条目的更新目标并归类。
// 决策顺序：symlink 跟到目标 → 否则 copy 安装查 registry 同名 →
// 找最近 git 仓库 → 否则读 legacy origin → 否则 orphan。
func (d *discoverer) classifyDirEntry(name, link string) {
	reg := registry.New(d.opts.RegistryDir)

	contentPath, ok := d.resolveContentRoot(link, name, reg)
	if !ok {
		return
	}

	if repo := NearestGitRepo(contentPath, d.opts.RegistryDir, filepath.Join(d.opts.DataDir, "sources")); repo != "" {
		if !d.seenRepo[repo] {
			d.seenRepo[repo] = true
			d.targets.GitRepos = append(d.targets.GitRepos, repo)
		}
		return
	}

	d.classifyOrigin(name, contentPath, reg)
}

// resolveContentRoot 返回条目对应的“内容根”路径（symlink 目标或 registry
// 同名目录）；条目既非受管 symlink 也无 registry 原件时返回 ok=false。
func (d *discoverer) resolveContentRoot(link, name string, reg *registry.Registry) (string, bool) {
	if symlink.IsSymlink(link) {
		if !symlink.PointInside(link, d.opts.RegistryDir) && !symlink.PointInside(link, filepath.Join(d.opts.DataDir, "sources")) {
			return "", false
		}
		target, err := filepath.EvalSymlinks(link)
		if err != nil {
			d.diagnose("eval-symlink", link, err)
			return "", false
		}
		return target, true
	}
	// 非 symlink：可能是 --copy 安装；尝试 registry 同名。
	if regPath, _ := reg.FindSkillDir(name); regPath != "" {
		return regPath, true
	}
	return "", false
}

// classifyOrigin 按 legacy origin 归属内容根：带 .sm-origin.json 则记为
// origin 目标，否则记 orphan（均按路径去重）。
func (d *discoverer) classifyOrigin(name, contentPath string, reg *registry.Registry) {
	regPath := contentPath
	if !PathInside(regPath, d.opts.RegistryDir) {
		p, _ := reg.FindSkillDir(name)
		if p == "" {
			// 已装在 agent 目录，但 registry 无对应原件。
			d.addOrphan(name, name)
			return
		}
		regPath = p
	}
	if meta, ok := ReadOriginMetadata(regPath); ok {
		if d.seenOrigin[regPath] {
			return
		}
		d.seenOrigin[regPath] = true
		d.targets.Origins = append(d.targets.Origins, DiscoveredOrigin{Name: name, SkillDir: regPath, Metadata: meta})
		return
	}
	// registry 有目录但无 git、无 origin。
	d.addOrphan(name, regPath)
}

// addOrphan 按 key 去重后追加 orphan 记录（orphan 列表本身记录技能名）。
func (d *discoverer) addOrphan(name, key string) {
	if d.seenOrigin[key] {
		return
	}
	d.seenOrigin[key] = true
	d.targets.Orphans = append(d.targets.Orphans, name)
}

// collectWellKnown 从项目锁文件读取 Well-Known Source 技能，并把同名孤儿
// 从孤儿列表移除（它们能通过重拉 endpoint 更新）。
func (d *discoverer) collectWellKnown() {
	lock, err := lockfile.NewManager(d.opts.ProjectDir).Load()
	if err != nil {
		d.diagnose("read-lockfile", d.opts.ProjectDir, err)
		return
	}
	selected := make(map[string]bool)
	for _, name := range d.opts.Names {
		selected[strings.ToLower(name)] = true
	}
	wellKnownNames := make(map[string]bool)
	for name, entry := range lock.Skills {
		if entry.SourceType != "well-known" || entry.SourceURL == "" {
			continue
		}
		if len(selected) > 0 && !selected[strings.ToLower(name)] {
			continue
		}
		d.targets.WellKnownSkills = append(d.targets.WellKnownSkills, DiscoveredWellKnownSkill{Name: name, Source: entry.SourceURL})
		wellKnownNames[name] = true
	}
	if len(wellKnownNames) == 0 {
		return
	}
	filtered := d.targets.Orphans[:0]
	for _, name := range d.targets.Orphans {
		if !wellKnownNames[name] {
			filtered = append(filtered, name)
		}
	}
	d.targets.Orphans = filtered
}

// PathInside 判断 path（解析符号链接后）是否位于 root 目录之内。
// 用 Rel 结果是否以 ".." 开头来判定，避免简单的 HasPrefix 误判
// （如 /a/b 与 /a/bbb）。供 update/cache/status 判断“安装是否指向 registry 或缓存”。
func PathInside(path, root string) bool {
	absPath, err1 := filepath.Abs(path)
	absRoot, err2 := filepath.Abs(root)
	if err1 != nil || err2 != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = resolved
	}
	if resolved, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = resolved
	}
	rel, err := filepath.Rel(absRoot, absPath)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// NearestGitRepo 从 path（解析符号链接后）逐级向上查找最近的含 .git 目录，
// 但不得越过任一 root（roots 经符号链接规范化后作为上界）。命中返回该仓库
// 路径，否则返回 ""。用于判定内容根是否落在可 git pull 的仓库里。
func NearestGitRepo(path string, roots ...string) string {
	for i, root := range roots {
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			roots[i] = resolved
		}
	}
	for p := path; p != "" && p != filepath.Dir(p); p = filepath.Dir(p) {
		inside := false
		for _, root := range roots {
			rel, err := filepath.Rel(root, p)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				inside = true
				break
			}
		}
		if !inside {
			return ""
		}
		if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
			return p
		}
	}
	return ""
}

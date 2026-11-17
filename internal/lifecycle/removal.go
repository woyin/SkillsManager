// removal.go 承接删除生命周期的两件事：引用扫描与清理。函数只接收显式
// Agent 目录/项目/已知项目，不读 cmd 全局；Registry 原件删除与“有引用拒绝/
// --force”等 ADR 0017 决策留在 cmd。
//
// Input: fmt, os, path/filepath, github.com/woyin/skills-manager/internal/lockfile, github.com/woyin/skills-manager/internal/symlink, github.com/woyin/skills-manager/internal/tool
// Output: type ScanOptions, func ScanDirs, type CollectOptions, type ReferenceSet, func CollectReferences, type CleanupOptions, type CleanupResult, func CleanupInstalls, func RemoveLockEntry
// Pos: 业务层-删除引用扫描与清理
package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/woyin/skills-manager/internal/lockfile"
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
)

// ScanOptions 是 rm 扫描目录的显式范围。
type ScanOptions struct {
	ProjectDir    string
	IncludeGlobal bool
}

// ScanDirs 返回要扫描的技能目录：全局（可选）+ 项目级。
func ScanDirs(t tool.Tool, opts ScanOptions) []string {
	var dirs []string
	if opts.IncludeGlobal {
		dirs = append(dirs, tool.GetGlobalSkillDir(t))
	}
	if pd := tool.GetProjectSkillDir(t, opts.ProjectDir); pd != "" {
		dirs = append(dirs, pd)
	}
	return dirs
}

// CollectOptions 是引用扫描的显式输入。
type CollectOptions struct {
	ProjectDir    string
	Tools         []tool.Tool
	KnownProjects []string
}

// ReferenceSet 是一次引用扫描的结果。扫描失败不静默：错误单独返回，
// Inaccessible 记录无法检查的路径（调用方据此不得声称“无引用”）。
type ReferenceSet struct {
	Paths        []string
	Inaccessible []string
}

// CollectReferences 收集全局与项目中对 skillName/skillPath 的引用。
// 全局目录同时检查同名条目与指向 skillPath 的链接；项目目录只检查同名条目
// （保持既有扫描范围），并覆盖当前项目与 DB 已知项目。
func CollectReferences(skillName, skillPath string, opts CollectOptions) (ReferenceSet, []error) {
	set := ReferenceSet{}
	seen := make(map[string]bool)
	var errs []error
	add := func(path string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		set.Paths = append(set.Paths, path)
	}
	inaccessible := func(path string, err error) {
		set.Inaccessible = append(set.Inaccessible, path)
		errs = append(errs, err)
	}

	for _, t := range opts.Tools {
		globalDir := tool.GetGlobalSkillDir(t)
		if link, err := existingLink(globalDir, skillName); err != nil {
			inaccessible(globalDir, err)
		} else {
			add(link)
		}
		if err := addPointingRefs(globalDir, skillPath, add); err != nil {
			inaccessible(globalDir, err)
		}
	}

	for _, project := range collectProjects(opts.ProjectDir, opts.KnownProjects) {
		for _, t := range opts.Tools {
			dir := tool.GetProjectSkillDir(t, project)
			if dir == "" {
				continue
			}
			if link, err := existingLink(dir, skillName); err != nil {
				inaccessible(dir, err)
				continue
			} else {
				add(link)
			}
		}
	}
	return set, errs
}

// collectProjects 返回当前项目 + 已知项目（去重保序，跳过空路径）。
func collectProjects(projectDir string, known []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, p := range append([]string{projectDir}, known...) {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// existingLink 返回 dir 下与 skillName 同名条目的路径；不存在返回空串。
// 其他 IO 错误（不可访问等）返回 error，不得当成“无引用”。
func existingLink(dir, skillName string) (string, error) {
	if dir == "" {
		return "", nil
	}
	link := filepath.Join(dir, skillName)
	if _, err := os.Lstat(link); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("inspecting %s: %w", link, err)
	}
	return link, nil
}

// addPointingRefs 收集 dir 中指向 skillPath 的链接；skillPath 为空时跳过。
func addPointingRefs(dir, skillPath string, add func(string)) error {
	if dir == "" || skillPath == "" {
		return nil
	}
	links, err := symlink.FindPointingTo(dir, skillPath)
	if err != nil {
		return fmt.Errorf("scanning %s: %w", dir, err)
	}
	for _, l := range links {
		add(l)
	}
	return nil
}

// CleanupOptions 是清理的显式输入。ExtraDirs 用于 Eve 子代理目录等额外位置。
type CleanupOptions struct {
	ProjectDir    string
	IncludeGlobal bool
	Tools         []tool.Tool
	KnownProjects []string
	ExtraDirs     []string
}

// CleanupResult 汇总一次清理：Removed 是实际删除的条目数；Failures 是删除或
// lock 清理错误；Inaccessible 是无法检查/删除的路径。
type CleanupResult struct {
	Removed      int
	Failures     []error
	Inaccessible []string
}

// CleanupInstalls 删除所有可访问项目/全局中 skillName 的安装条目，并清理
// 对应 lock entries。当前项目同时清理全局目录（与既有语义一致）。
func CleanupInstalls(skillName string, opts CleanupOptions) CleanupResult {
	res := CleanupResult{}
	for _, project := range collectProjects(opts.ProjectDir, opts.KnownProjects) {
		includeGlobal := opts.IncludeGlobal && project == opts.ProjectDir
		for _, t := range opts.Tools {
			if includeGlobal {
				res.remove(filepath.Join(tool.GetGlobalSkillDir(t), skillName))
			}
			if dir := tool.GetProjectSkillDir(t, project); dir != "" {
				res.remove(filepath.Join(dir, skillName))
			}
		}
		if err := RemoveLockEntry(project, skillName); err != nil {
			res.Failures = append(res.Failures, err)
		}
	}
	// 无项目路径（如 rm --dir）时，既有语义仍清理全局目录。
	if opts.ProjectDir == "" && opts.IncludeGlobal {
		for _, t := range opts.Tools {
			res.remove(filepath.Join(tool.GetGlobalSkillDir(t), skillName))
		}
	}
	for _, dir := range opts.ExtraDirs {
		if dir == "" {
			continue
		}
		res.remove(filepath.Join(dir, skillName))
	}
	return res
}

// remove 删除单个条目并记录结果；不存在视为无操作。
func (r *CleanupResult) remove(path string) {
	if _, err := os.Lstat(path); err != nil {
		if !os.IsNotExist(err) {
			r.Inaccessible = append(r.Inaccessible, path)
		}
		return
	}
	if err := os.RemoveAll(path); err != nil {
		r.Failures = append(r.Failures, fmt.Errorf("removing %s: %w", path, err))
		return
	}
	r.Removed++
}

// RemoveLockEntry 从项目 skills-lock.json 删除条目；文件不存在或无项目时无操作。
func RemoveLockEntry(projectDir, name string) error {
	if projectDir == "" {
		return nil
	}
	lm := lockfile.NewManager(projectDir)
	if !lm.Exists() {
		return nil
	}
	return lm.Remove(name)
}

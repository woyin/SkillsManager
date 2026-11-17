// Package health 提供安装完整性的窄只读扫描：失效/孤立符号链接与丢失的项目记录。
// 扫描本身只读，不修复；问题与 IO 错误分开返回，由调用方决定呈现与修复方式，
// 避免 CLI 与 Web 各自实现分类而出现结论分叉。
//
// Input: fmt, os, path/filepath, github.com/woyin/skills-manager/internal/symlink
// Output: type IssueType, const IssueBrokenSymlink, const IssueOrphanedSymlink, const IssueMissingProject, type Issue, type Report, func Scan
// Pos: 业务层-安装完整性只读扫描
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package health

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/woyin/skills-manager/internal/symlink"
)

// IssueType 是完整性问题的稳定分类；取值同时用于 CLI 文案与 Web JSON 的 type 字段。
type IssueType string

const (
	// IssueBrokenSymlink 表示符号链接目标已不存在。
	IssueBrokenSymlink IssueType = "broken_symlink"
	// IssueOrphanedSymlink 表示符号链接目标不在 Registry 根内，即非 sm 管理的链接。
	IssueOrphanedSymlink IssueType = "orphaned_symlink"
	// IssueMissingProject 表示数据库记录的项目目录已不在磁盘上。
	IssueMissingProject IssueType = "missing_project"
)

// Issue 是一条完整性问题。同一 Type+Path 在报告中只出现一次。
type Issue struct {
	Type IssueType
	Path string
}

// Report 是一次扫描发现的问题集合。IO 错误不进入 Report，而是由 Scan 返回。
type Report struct {
	Issues []Issue
}

// Scan 扫描 dirs 中的符号链接与 knownProjects 记录的项目目录。
//
// 链接判定在扫描边界自行保留错误：Lstat 决定条目类型，Readlink 读目标，
// Stat 跟随目标。目标不存在才算 broken；EACCES、ELOOP 等其他错误直接返回，
// 避免把“暂时不可访问”误报为失效并让 --fix 删除仍有效的链接。
//
// dirs 由调用方解析（CLI 与 Web 都必须使用 tool.GetGlobalSkillDir），重复目录只扫一次。
// registryDir 是 Registry 根，用于判定链接是否仍受 sm 管理。
// 目录不存在视为健康跳过；读目录、读链接或 Stat 的其他错误立即返回，调用方不得
// 把它当成健康状态。符号链接按 broken 优先于 orphaned 分类。
func Scan(dirs []string, registryDir string, knownProjects []string) (Report, error) {
	report := Report{}
	seenIssues := make(map[Issue]bool)
	add := func(issue Issue) {
		if seenIssues[issue] {
			return
		}
		seenIssues[issue] = true
		report.Issues = append(report.Issues, issue)
	}

	seenDirs := make(map[string]bool, len(dirs))
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		key := filepath.Clean(dir)
		if seenDirs[key] {
			continue
		}
		seenDirs[key] = true

		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Report{}, fmt.Errorf("reading skill directory %s: %w", dir, err)
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())

			// 用 Lstat（entry.Info）取得条目本身的状态，不吞错误；
			// symlink.IsSymlink/Verify 把 Lstat/Stat 的 EACCES、ELOOP 等
			// IO 错误折叠成 false，会把“暂时读不到”误报为 broken。
			info, err := entry.Info()
			if err != nil {
				return Report{}, fmt.Errorf("inspecting %s: %w", path, err)
			}
			if info.Mode()&os.ModeSymlink == 0 {
				continue
			}
			if _, err := os.Readlink(path); err != nil {
				return Report{}, fmt.Errorf("reading symlink %s: %w", path, err)
			}
			if _, err := os.Stat(path); err != nil {
				// 只有真正的“目标不存在”才是 broken；权限、循环链接
				// 等 IO 错误必须上报，调用方不得当成健康或可删除。
				if !os.IsNotExist(err) {
					return Report{}, fmt.Errorf("resolving symlink %s: %w", path, err)
				}
				add(Issue{Type: IssueBrokenSymlink, Path: path})
				continue
			}
			// 分类沿用既有 lexical 契约：目标是否落在 Registry 根内。
			if !symlink.PointInside(path, registryDir) {
				add(Issue{Type: IssueOrphanedSymlink, Path: path})
			}
		}
	}

	for _, project := range knownProjects {
		if project == "" {
			continue
		}
		if _, err := os.Stat(project); err != nil {
			if os.IsNotExist(err) {
				add(Issue{Type: IssueMissingProject, Path: project})
				continue
			}
			return Report{}, fmt.Errorf("checking project directory %s: %w", project, err)
		}
	}

	return report, nil
}

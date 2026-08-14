// cmd/cache.go 实现 `sm cache`：查看与清理持久化的远程源缓存。
//
// sm 在 install/update 时会把远程 git 源克隆到 data/sources/ 下做缓存，
// 后续安装复用而非重新克隆。本命令列出这些缓存（含引用计数、大小、是否
// pinned），并支持 --prune 清理无人引用的缓存。
//
// 清理安全性依赖一套可达性分析：sourceCacheRefs 扫描所有全局 agent 目录与
// 数据库记录的项目目录里的符号链接，凡是最终指向某个缓存的，就算该缓存的
// 一次引用；引用为 0 的缓存才允许被 prune 删除。因此只有"装到全局 agent
// 目录"或"装到已记录项目"的技能能保护其源缓存——这也是 --prune 要求 --yes
// 的原因（见 cacheCmd 的错误信息）。
//
// Input: fmt, io, os, os/exec, path/filepath, sort, strings, text/tabwriter, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/home, github.com/woyin/skills-manager/internal/symlink, github.com/woyin/skills-manager/internal/tool
// Output: type sourceCache, var cacheCmd, func sourceCaches, func sourceCacheRefs, func pruneSourceCaches, func writeSourceCaches, func formatBytes
// Pos: 控制层-cache命令实现（远程源缓存查看/清理）
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
)

var (
	cachePrune bool
	cacheYes   bool
)

// sourceCache 列表项：一个远程源缓存的可读表示。
type sourceCache struct {
	Path   string
	Source string
	Ref    string
	Commit string
	Pinned bool // detached HEAD（被 update 跳过）
	Refs   int  // 被多少个已安装链接引用（0 即可 prune）
	Size   int64
}

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Show or prune persistent remote source caches",
	RunE: func(cmd *cobra.Command, args []string) error {
		items, err := sourceCaches()
		if err != nil {
			return err
		}
		if cachePrune {
			if !cacheYes {
				return fmt.Errorf("--prune requires --yes; only links in global agent directories and recorded projects can protect a cache")
			}
			removed, bytes, err := pruneSourceCaches(items)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %d unused cache(s), freed %s\n", removed, formatBytes(bytes))
			return nil
		}
		return writeSourceCaches(cmd.OutOrStdout(), items)
	},
}

// sourceCaches 枚举 data/sources/ 下所有缓存目录，组装为列表项。
// 引用计数委托 sourceCacheRefs 计算；来源/ref 取自 sources-meta 元数据，
// 缺失时回退到 git remote。按 Source 排序输出。
func sourceCaches() ([]sourceCache, error) {
	root := filepath.Join(DataDir, "sources")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	refs := sourceCacheRefs(root)
	items := make([]sourceCache, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
			continue
		}
		size, err := dirSize(path)
		if err != nil {
			return nil, err
		}
		pinned, _ := gitDetached(path)
		meta := readSourceCacheMetadata(filepath.Join(DataDir, "sources-meta", entry.Name()+".json"))
		source := meta.Source
		if source == "" {
			source = gitRemote(path)
		}
		items = append(items, sourceCache{
			Path: path, Source: source, Ref: meta.Ref, Commit: shortHash(gitHeadHash(path)),
			Pinned: pinned, Refs: refs[canonicalPath(path)], Size: size,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Source < items[j].Source })
	return items, nil
}

// sourceCacheRefs 对 cacheRoot 下每个缓存做可达性分析，返回
// "缓存规范化路径 → 引用计数" 映射。这是 prune 判活的核心：
//
// 它扫描两组目录里的符号链接——所有工具的全局 agent 目录，以及数据库
// 记录的项目目录下每个工具的项目级 agent 目录——凡是 PointInside(cacheRoot)
// 的链接，沿 EvalSymlinks 找到其所在的 git 仓库（nearestGitRepo），
// 该仓库的引用数 +1。
//
// 引用为 0 的缓存即被判定为无人使用，可安全 prune。这隐含一条契约：
// 只有装到全局 agent 目录或已记录项目的技能能保护其源缓存；手工放置或
// 装到未记录项目的技能不会被计入。
func sourceCacheRefs(cacheRoot string) map[string]int {
	refs := map[string]int{}
	for _, dir := range cacheScanDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			refs = countLinkRefs(refs, filepath.Join(dir, entry.Name()), cacheRoot)
		}
	}
	return refs
}

// cacheScanDirs 收集所有可能含缓存引用的技能目录：全局 agent 目录 +
// DB 中已记录项目的 agent 技能目录（去重）。
func cacheScanDirs() []string {
	dirs := make([]string, 0, len(tool.AllTools()))
	for _, t := range tool.AllTools() {
		dirs = append(dirs, filepath.Join(home.Dir(), t.SkillDir))
	}
	if database, err := openDB(); err == nil {
		if projects, queryErr := database.GetAllProjects(); queryErr == nil {
			for _, project := range projects {
				for _, t := range tool.AllTools() {
					if dir := tool.GetProjectSkillDir(t, project.Path); dir != "" {
						dirs = append(dirs, dir)
					}
				}
			}
		}
		database.Close()
	}

	seenDirs := map[string]bool{}
	out := dirs[:0]
	for _, dir := range dirs {
		if seenDirs[dir] {
			continue
		}
		seenDirs[dir] = true
		out = append(out, dir)
	}
	return out
}

// countLinkRefs 若 link 是指向 cacheRoot 内 git 仓库的 symlink，则给对应
// 仓库的引用计数加 1 并返回更新后的计数表。
func countLinkRefs(refs map[string]int, link, cacheRoot string) map[string]int {
	if !symlink.IsSymlink(link) || !symlink.PointInside(link, cacheRoot) {
		return refs
	}
	target, err := filepath.EvalSymlinks(link)
	if err != nil {
		return refs
	}
	if repo := nearestGitRepo(target, cacheRoot); repo != "" {
		refs[canonicalPath(repo)]++
	}
	return refs
}

// pruneSourceCaches 删除所有引用计数为 0 的缓存目录及其 sources-meta 元数据，
// 返回（删除数，释放字节数）。引用计数 > 0 的缓存予以保留（见 sourceCacheRefs
// 的判活契约）。调用方应已校验 --yes。
func pruneSourceCaches(items []sourceCache) (int, int64, error) {
	removed := 0
	var bytes int64
	for _, item := range items {
		if item.Refs > 0 {
			continue
		}
		if err := os.RemoveAll(item.Path); err != nil {
			return removed, bytes, fmt.Errorf("removing %s: %w", item.Path, err)
		}
		os.Remove(filepath.Join(DataDir, "sources-meta", filepath.Base(item.Path)+".json"))
		removed++
		bytes += item.Size
	}
	return removed, bytes, nil
}

// writeSourceCaches 以表格打印缓存清单（SOURCE/REF/COMMIT/MODE/REFS/SIZE）及合计。
func writeSourceCaches(out io.Writer, items []sourceCache) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SOURCE\tREF\tCOMMIT\tMODE\tREFS\tSIZE")
	var total int64
	for _, item := range items {
		mode := "tracking"
		if item.Pinned {
			mode = "pinned"
		}
		ref := item.Ref
		if ref == "" {
			ref = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n", item.Source, ref, item.Commit, mode, item.Refs, formatBytes(item.Size))
		total += item.Size
	}
	fmt.Fprintf(w, "\n%d cache(s), %s total\n", len(items), formatBytes(total))
	return w.Flush()
}

// gitRemote 取仓库 origin 远端地址；失败时回退到目录名。
func gitRemote(repo string) string {
	out, err := exec.Command("git", "-C", repo, "remote", "get-url", "origin").Output()
	if err != nil {
		return filepath.Base(repo)
	}
	return strings.TrimSpace(string(out))
}

// dirSize 递归累加 root 下所有普通文件的大小（跳过目录与符号链接）。
func dirSize(root string) (int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err == nil {
			size += info.Size()
		}
		return err
	})
	return size, err
}

// canonicalPath 返回 path 解析符号链接后的绝对路径，供跨进程/跨调用
// 的稳定键（EvalSymlinks 失败时回退到 Clean）。
func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

// formatBytes 把字节数格式化为人类可读的 KiB/MiB/GiB（二进制 1024 进制）。
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for value := n / unit; value >= unit && exp < 3; value /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}

func init() {
	cacheCmd.Flags().BoolVar(&cachePrune, "prune", false, "Remove caches not referenced by any known global or recorded-project skill link")
	cacheCmd.Flags().BoolVarP(&cacheYes, "yes", "y", false, "Confirm cache deletion")
	rootCmd.AddCommand(cacheCmd)
}

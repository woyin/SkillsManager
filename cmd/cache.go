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

type sourceCache struct {
	Path   string
	Source string
	Ref    string
	Commit string
	Pinned bool
	Refs   int
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

func sourceCacheRefs(cacheRoot string) map[string]int {
	refs := map[string]int{}
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
	for _, dir := range dirs {
		if seenDirs[dir] {
			continue
		}
		seenDirs[dir] = true
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			link := filepath.Join(dir, entry.Name())
			if !symlink.IsSymlink(link) || !symlink.PointInside(link, cacheRoot) {
				continue
			}
			target, err := filepath.EvalSymlinks(link)
			if err != nil {
				continue
			}
			if repo := nearestGitRepo(target, cacheRoot); repo != "" {
				refs[canonicalPath(repo)]++
			}
		}
	}
	return refs
}

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

func gitRemote(repo string) string {
	out, err := exec.Command("git", "-C", repo, "remote", "get-url", "origin").Output()
	if err != nil {
		return filepath.Base(repo)
	}
	return strings.TrimSpace(string(out))
}

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

func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

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

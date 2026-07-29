// source_cache.go 管理远程 git 源的持久化缓存（~/.sm/data/sources）。
// 被 install / cache / update 共用：安装时写入，cache 命令展示/清理，
// update 刷新 tracking 模式的缓存。
//
// Input: crypto/sha256, encoding/hex, encoding/json, fmt, os, os/exec, path/filepath, time, github.com/woyin/skills-manager/internal/registry
// Output: type sourceCacheMetadata, func sourceCachePaths, func writeSourceCacheMetadata, func readSourceCacheMetadata, func cachedGitSource
// Pos: 数据层-远程git源缓存
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/woyin/skills-manager/internal/registry"
)

// sourceCacheMetadata 是每个缓存仓库旁的 provenance 元数据。
type sourceCacheMetadata struct {
	Source    string    `json:"source"`
	Ref       string    `json:"ref,omitempty"`
	Commit    string    `json:"commit"`
	CreatedAt time.Time `json:"created_at"`
}

// sourceCachePaths 按 source+ref 派生缓存目录与元数据路径。
// ref 参与 key，因此同一仓库的不同 pin 互不覆盖。
func sourceCachePaths(source, ref string) (string, string) {
	key := source
	if ref != "" {
		key += "\x00" + ref
	}
	sum := sha256.Sum256([]byte(key))
	id := hex.EncodeToString(sum[:])
	return filepath.Join(DataDir, "sources", id), filepath.Join(DataDir, "sources-meta", id+".json")
}

func writeSourceCacheMetadata(path string, meta sourceCacheMetadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readSourceCacheMetadata(path string) sourceCacheMetadata {
	var meta sourceCacheMetadata
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &meta)
	}
	return meta
}

// cachedGitSource 把远程源克隆到 DataDir/sources 下的持久目录。
// 符号链接安装因此在进程退出后仍有效，重复安装可复用同一克隆。
// offline=true 时仅命中已有缓存，绝不发起网络 clone。
func cachedGitSource(source, ref string, offline ...bool) (string, error) {
	dest, metaPath := sourceCachePaths(source, ref)
	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		return dest, nil
	}
	if len(offline) > 0 && offline[0] {
		return "", fmt.Errorf("source not cached for offline install: %s (ref %q)", source, ref)
	}

	// Clone strategy: try shallow clone with --branch <ref> first (fast, aligns
	// with npx skills). If ref is a commit hash not reachable by --branch,
	// fall back to full clone + checkout.
	tmpDir, err := os.MkdirTemp("", "sm-install-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	repoURL, branch, _, ok := registry.ParseTreeURL(source)
	if !ok {
		repoURL = registry.NormalizeGitURL(source)
	}

	cloneDest := filepath.Join(tmpDir, "repo")
	// Prefer tree-URL branch, then explicit ref, else shallow-clone default.
	if branch != "" {
		if err := registry.CloneRepoWithBranch(repoURL, branch, cloneDest); err != nil {
			return "", fmt.Errorf("cloning %s: %w", repoURL, err)
		}
		// Tree URL had a branch but caller also pinned a different ref (commit).
		if ref != "" && ref != branch {
			if out, checkoutErr := exec.Command("git", "-C", cloneDest, "fetch", "--depth=1", "origin", ref).CombinedOutput(); checkoutErr != nil {
				return "", fmt.Errorf("fetching ref %q: %w: %s", ref, checkoutErr, out)
			}
			if out, checkoutErr := exec.Command("git", "-C", cloneDest, "checkout", "--detach", ref).CombinedOutput(); checkoutErr != nil {
				return "", fmt.Errorf("checking out ref %q: %w: %s", ref, checkoutErr, out)
			}
		}
	} else if ref != "" {
		if err := registry.CloneRepoWithBranch(repoURL, ref, cloneDest); err != nil {
			// ref is likely a commit hash — full clone then checkout.
			if err := registry.CloneRepo(repoURL, cloneDest); err != nil {
				return "", fmt.Errorf("cloning %s: %w", repoURL, err)
			}
			if out, checkoutErr := exec.Command("git", "-C", cloneDest, "checkout", "--detach", ref).CombinedOutput(); checkoutErr != nil {
				return "", fmt.Errorf("checking out ref %q: %w: %s", ref, checkoutErr, out)
			}
		}
	} else {
		if err := registry.CloneRepoShallow(repoURL, cloneDest); err != nil {
			return "", fmt.Errorf("cloning %s: %w", repoURL, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return "", fmt.Errorf("creating source cache: %w", err)
	}
	if err := os.Rename(cloneDest, dest); err != nil {
		// 竞态：另一进程已写入同一缓存。
		if _, statErr := os.Stat(filepath.Join(dest, ".git")); statErr == nil {
			return dest, nil
		}
		return "", fmt.Errorf("caching source: %w", err)
	}
	meta := sourceCacheMetadata{Source: source, Ref: ref, Commit: gitHeadHash(dest), CreatedAt: time.Now().UTC()}
	if err := writeSourceCacheMetadata(metaPath, meta); err != nil {
		os.RemoveAll(dest)
		return "", fmt.Errorf("writing source cache metadata: %w", err)
	}
	return dest, nil
}

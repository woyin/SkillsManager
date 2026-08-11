// Package sourcecache owns the persistent cache used for remote git sources.
//
// The cache deliberately has a small interface: callers provide the data
// directory, source and optional ref, and receive a stable checkout path.  The
// implementation owns cache key derivation, clone/ref handling, metadata, and
// offline semantics so those details do not drift between add/install/update.
// Cache paths are intentionally compatible with the historical layout:
//   - <data>/sources/<sha256(source + NUL + ref)>
//   - <data>/sources-meta/<sha256(source + NUL + ref)>.json
package sourcecache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/woyin/skills-manager/internal/registry"
)

// Metadata is the provenance stored beside a cached checkout.
//
// Source and Ref are the exact values supplied to Acquire.  Commit is the
// checkout's HEAD at acquisition time; CreatedAt is UTC for stable, portable
// metadata.  The JSON shape is kept compatible with the original cmd cache.
type Metadata struct {
	Source    string    `json:"source"`
	Ref       string    `json:"ref,omitempty"`
	Commit    string    `json:"commit"`
	CreatedAt time.Time `json:"created_at"`
}

// Result describes an acquired checkout.  Path is always the persistent
// checkout directory.  Metadata is populated from disk on cache hits and
// from the newly written record on cache misses.  Cached reports whether the
// checkout already existed before this call.
type Result struct {
	Path     string
	Metadata Metadata
	Cached   bool
}

// Store is a persistent source-cache implementation rooted at DataDir (the
// caller decides the directory, which keeps this package independent of cmd's
// mutable globals and makes it straightforward to test).
type Store struct {
	dataDir string
}

// New creates a source cache rooted at dataDir.  It does not touch the
// filesystem until Acquire or a metadata operation is requested.
func New(dataDir string) *Store {
	return &Store{dataDir: dataDir}
}

// Key returns the stable cache key for source+ref.  The NUL separator is part
// of the on-disk format and prevents ambiguous concatenations.
func Key(source, ref string) string {
	key := source
	if ref != "" {
		key += "\x00" + ref
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// Paths returns the persistent checkout and metadata paths for source+ref.
// It preserves the historical sources/sources-meta directory names.
func (s *Store) Paths(source, ref string) (cachePath, metadataPath string) {
	root := ""
	if s != nil {
		root = s.dataDir
	}
	id := Key(source, ref)
	return filepath.Join(root, "sources", id), filepath.Join(root, "sources-meta", id+".json")
}

// Paths is a package-level convenience for adapters that do not need to keep
// a Store value.  It is equivalent to New(dataDir).Paths(source, ref).
func Paths(dataDir, source, ref string) (cachePath, metadataPath string) {
	return New(dataDir).Paths(source, ref)
}

// ReadMetadata reads a metadata file.  A malformed or missing file is an
// error; callers that intentionally tolerate old/incomplete caches can ignore
// the error and use the zero Metadata value.
func ReadMetadata(path string) (Metadata, error) {
	var meta Metadata
	data, err := os.ReadFile(path)
	if err != nil {
		return meta, err
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return Metadata{}, err
	}
	return meta, nil
}

// WriteMetadata atomically writes a metadata record, creating its parent
// directory as needed.  A temporary sibling avoids exposing partial JSON to
// concurrent cache readers.
func WriteMetadata(path string, meta Metadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".source-cache-meta-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if err := tmp.Chmod(0644); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// Acquire returns a persistent checkout for source at ref.
//
// A cache hit never invokes git clone/fetch or accesses the network (it may
// inspect HEAD only to fill an incomplete metadata record).  With offline=true,
// a miss returns the stable "source not cached for offline install" error and
// does not create directories or run git.  For online misses the clone/ref
// strategy matches the historical command implementation: tree URL branches
// are preferred, explicit refs try a shallow branch clone then fall back to a
// full clone plus detached checkout (for commit hashes), and no ref uses a
// shallow default-branch clone.
func (s *Store) Acquire(source, ref string, offline bool) (Result, error) {
	if s == nil {
		return Result{}, fmt.Errorf("source cache is nil")
	}
	dest, metaPath := s.Paths(source, ref)
	if isGitCheckout(dest) {
		meta, _ := ReadMetadata(metaPath)
		// Old/incomplete caches may have no commit record.  Fill that one gap
		// from the checkout without rewriting metadata; update owns metadata
		// refreshes for an existing cache.
		if meta.Commit == "" {
			meta.Commit = headCommit(dest)
		}
		return Result{Path: dest, Metadata: meta, Cached: true}, nil
	}
	if offline {
		return Result{}, fmt.Errorf("source not cached for offline install: %s (ref %q)", source, ref)
	}

	cloneDest, err := clone(source, ref)
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(filepath.Dir(cloneDest))

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return Result{}, fmt.Errorf("creating source cache: %w", err)
	}
	if err := os.Rename(cloneDest, dest); err != nil {
		// Another process may have populated this key while we cloned.  Reuse
		// the winner rather than failing or replacing a valid checkout.
		if isGitCheckout(dest) {
			meta, _ := ReadMetadata(metaPath)
			if meta.Commit == "" {
				meta.Commit = headCommit(dest)
			}
			return Result{Path: dest, Metadata: meta, Cached: true}, nil
		}
		return Result{}, fmt.Errorf("caching source: %w", err)
	}

	meta := Metadata{
		Source:    source,
		Ref:       ref,
		Commit:    headCommit(dest),
		CreatedAt: time.Now().UTC(),
	}
	if err := WriteMetadata(metaPath, meta); err != nil {
		_ = os.RemoveAll(dest)
		return Result{}, fmt.Errorf("writing source cache metadata: %w", err)
	}
	return Result{Path: dest, Metadata: meta}, nil
}

// Acquire is a package-level convenience for one-shot callers.  Prefer a
// Store when several operations share the same data directory.
func Acquire(dataDir, source, ref string, offline bool) (Result, error) {
	return New(dataDir).Acquire(source, ref, offline)
}

func isGitCheckout(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

func clone(source, ref string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "sm-install-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}
	cloneDest := filepath.Join(tmpDir, "repo")

	repoURL, branch, _, ok := registry.ParseTreeURL(source)
	if !ok {
		repoURL = registry.NormalizeGitURL(source)
	}

	// The caller owns cleanup through the returned cloneDest's parent.  Every
	// error path below removes the temporary directory before returning.
	cleanupOnError := func(err error) (string, error) {
		_ = os.RemoveAll(tmpDir)
		return "", err
	}

	if branch != "" {
		if err := registry.CloneRepoWithBranch(repoURL, branch, cloneDest); err != nil {
			return cleanupOnError(fmt.Errorf("cloning %s: %w", repoURL, err))
		}
		// Tree URL branch is the default unless the explicit ref selects a
		// different commit/branch.  Preserve the old attached-branch behavior
		// when both values are equal.
		if ref != "" && ref != branch {
			if err := fetchAndCheckout(cloneDest, ref); err != nil {
				return cleanupOnError(err)
			}
		}
	} else if ref != "" {
		if err := registry.CloneRepoWithBranch(repoURL, ref, cloneDest); err != nil {
			// A commit hash (or another ref not advertised as a branch) may not
			// work with --branch.  Full clone then detached checkout matches the
			// previous behavior.
			if err := registry.CloneRepo(repoURL, cloneDest); err != nil {
				return cleanupOnError(fmt.Errorf("cloning %s: %w", repoURL, err))
			}
			if err := checkoutDetached(cloneDest, ref); err != nil {
				return cleanupOnError(err)
			}
		}
	} else if err := registry.CloneRepoShallow(repoURL, cloneDest); err != nil {
		return cleanupOnError(fmt.Errorf("cloning %s: %w", repoURL, err))
	}
	return cloneDest, nil
}

func fetchAndCheckout(repoPath, ref string) error {
	if out, err := exec.Command("git", "-C", repoPath, "fetch", "--depth=1", "origin", ref).CombinedOutput(); err != nil {
		return fmt.Errorf("fetching ref %q: %w: %s", ref, err, out)
	}
	return checkoutDetached(repoPath, ref)
}

func checkoutDetached(repoPath, ref string) error {
	if out, err := exec.Command("git", "-C", repoPath, "checkout", "--detach", ref).CombinedOutput(); err != nil {
		return fmt.Errorf("checking out ref %q: %w: %s", ref, err, out)
	}
	return nil
}

func headCommit(repoPath string) string {
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

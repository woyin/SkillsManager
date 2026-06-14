package registry

import (
	"fmt"
	"os"
	"path/filepath"
)

// CloneToTemp clones source into a fresh temp directory and returns the path
// to the cloned repo (a "repo" subdir of the temp dir). The caller receives
// the temp dir path too so it can defer cleanup; RemoveCloneTemp deletes it.
//
// source may be any form accepted by ParseTreeURL/NormalizeGitURL (GitHub
// shorthand, full HTTPS URL, SSH URL, /tree/<branch>/... path). If source is
// not a git URL, CloneToTemp returns an error.
//
// This consolidates the clone-into-temp boilerplate that cmd/add and cmd/use
// previously duplicated verbatim.
func CloneToTemp(source, tempPrefix string) (repoDir, tempDir string, err error) {
	if !IsGitURL(source) {
		return "", "", fmt.Errorf("not a git source: %s", source)
	}

	repoURL, branch, _, ok := ParseTreeURL(source)
	if !ok {
		repoURL = NormalizeGitURL(source)
	}

	tempDir, err = os.MkdirTemp("", tempPrefix)
	if err != nil {
		return "", "", fmt.Errorf("creating temp dir: %w", err)
	}

	repoDir = filepath.Join(tempDir, "repo")
	if branch != "" {
		err = CloneRepoWithBranch(repoURL, branch, repoDir)
	} else {
		err = CloneRepo(repoURL, repoDir)
	}
	if err != nil {
		os.RemoveAll(tempDir)
		return "", "", fmt.Errorf("cloning %s: %w", repoURL, err)
	}
	return repoDir, tempDir, nil
}

// RemoveCloneTemp removes a temp dir created by CloneToTemp. Safe to call on
// a zero/empty path (no-op), so it can be used in a defer even on the error
// path where no temp dir was allocated.
func RemoveCloneTemp(tempDir string) {
	if tempDir != "" {
		os.RemoveAll(tempDir)
	}
}

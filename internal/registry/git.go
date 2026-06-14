package registry

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/woyin/skills-manager/internal/fsutil"
)

// ── Git clone helpers ──

// CloneRepo clones url into dest (no depth limit). Returns an error if dest
// already exists. Exported so cmd packages share one implementation instead
// of duplicating exec.Command boilerplate.
func CloneRepo(url, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("destination already exists: %s", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	cmd := exec.Command("git", "clone", url, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CloneRepoWithBranch clones a single branch (--depth 1) of url into dest.
// Exported so cmd packages share one implementation.
func CloneRepoWithBranch(url, branch, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("destination already exists: %s", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	args := []string{"clone"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, "--depth", "1", url, dest)
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// copyDir copies src into dest via the shared fsutil helper, enforcing the
// registry's "dest must not exist" contract.
func (r *Registry) copyDir(src, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("destination already exists: %s", dest)
	}
	return fsutil.CopyDir(src, dest)
}

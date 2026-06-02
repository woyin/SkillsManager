// internal/symlink/symlink.go
package symlink

import (
	"fmt"
	"os"
	"path/filepath"
)

// Create creates a symlink at dst pointing to src.
// Creates parent directories if needed.
func Create(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("creating parent dir: %w", err)
	}

	// If symlink already exists with correct target, skip
	if target, err := os.Readlink(dst); err == nil {
		if target == src {
			return nil
		}
		return fmt.Errorf("symlink %s already exists pointing to %s (want %s)", dst, target, src)
	}

	return os.Symlink(src, dst)
}

// IsSymlink returns true if path is a symbolic link.
func IsSymlink(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeSymlink != 0
}

// Verify checks if a symlink exists and its target exists.
func Verify(path string) bool {
	if !IsSymlink(path) {
		return false
	}
	target, err := os.Readlink(path)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	_, err = os.Stat(target)
	return err == nil
}

// RemoveIfBroken removes a broken symlink. Returns true if removed.
func RemoveIfBroken(path string) (bool, error) {
	if !IsSymlink(path) {
		return false, nil
	}
	if Verify(path) {
		return false, nil
	}
	return true, os.Remove(path)
}

// FindPointingTo finds all symlinks in searchDir that point to target.
func FindPointingTo(searchDir, target string) ([]string, error) {
	var results []string

	err := filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return nil
		}
		linkTarget, err := os.Readlink(path)
		if err != nil {
			return nil
		}
		if !filepath.IsAbs(linkTarget) {
			linkTarget = filepath.Join(filepath.Dir(path), linkTarget)
		}
		if linkTarget == target {
			results = append(results, path)
		}
		return nil
	})

	return results, err
}

// RemoveAll removes a symlink or directory.
func RemoveAll(path string) error {
	return os.RemoveAll(path)
}
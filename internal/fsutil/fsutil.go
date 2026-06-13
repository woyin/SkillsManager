// Package fsutil provides small filesystem helpers shared across the sm
// packages. It centralizes the directory-copy logic that was previously
// duplicated in internal/registry and cmd/add.
package fsutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// skipDirs are directory entries never copied (version-control metadata and
// dependency trees would just waste space when cloning a skill into the
// registry or an agent directory).
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
}

// CopyDir copies the tree rooted at src into dest. It follows symlinks (copying
// their target contents), preserves file modes, streams file contents rather
// than buffering them in memory, and skips .git/node_modules directories.
//
// dest must not already exist; this mirrors the registry's existing contract
// so callers detect a prior install via the "destination already exists" error.
func CopyDir(src, dest string) error {
	srcInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}

	// If src is a symlink, resolve and copy the target tree.
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		target, err := resolveSymlink(src)
		if err != nil {
			return err
		}
		return CopyDir(target, dest)
	}

	if err := os.MkdirAll(dest, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() && skipDirs[entry.Name()] {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		// Follow symlinks at the entry level too.
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := resolveSymlink(srcPath)
			if err != nil {
				return err
			}
			targetInfo, err := os.Stat(target)
			if err != nil {
				return fmt.Errorf("stat symlink target %s: %w", target, err)
			}
			if targetInfo.IsDir() {
				if err := CopyDir(target, destPath); err != nil {
					return err
				}
			} else {
				if err := copyFile(target, destPath); err != nil {
					return err
				}
			}
		} else if entry.IsDir() {
			if err := CopyDir(srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, destPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolveSymlink returns the absolute path a symlink points at.
func resolveSymlink(link string) (string, error) {
	target, err := os.Readlink(link)
	if err != nil {
		return "", fmt.Errorf("reading symlink %s: %w", link, err)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(link), target)
	}
	return target, nil
}

// copyFile streams a single file from src to dest, preserving its mode.
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	srcInfo, _ := os.Stat(src)
	return os.Chmod(dest, srcInfo.Mode())
}

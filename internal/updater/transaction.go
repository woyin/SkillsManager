// Package updater provides the filesystem transaction used when a single
// source refreshes multiple Registry originals.
//
// A source update is a unit of consistency: every destination is staged and
// validated before any destination is replaced. If a commit step fails, all
// destinations that were already moved are restored from their backups. The
// package deliberately knows nothing about Registry metadata or lint policy;
// callers provide a prepare hook for metadata and a validate hook for domain
// validation.
package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/woyin/skills-manager/internal/fsutil"
)

// Target describes one source directory and the destination it should replace.
// Destinations must be distinct and must not overlap one another.
type Target struct {
	Name        string
	SourceDir   string
	Destination string
}

// Hooks customize the domain-specific parts of a transaction.
//
// Prepare runs after SourceDir has been copied to the staging directory and
// before validation. It is intended for writing provenance metadata into the
// staged tree. Validate runs against the staged tree; any error aborts the
// transaction before an existing destination is touched.
type Hooks struct {
	Prepare  func(Target, string) error
	Validate func(Target, string) error
}

// Apply stages, validates, and commits all targets as one filesystem
// transaction. A successful call replaces every destination and returns the
// number of committed targets. On any error it returns zero and restores every
// destination that was moved during commit.
func Apply(targets []Target, hooks Hooks) (int, error) {
	if len(targets) == 0 {
		return 0, nil
	}
	if err := validateTargets(targets); err != nil {
		return 0, err
	}

	tx := transaction{targets: targets, hooks: hooks}
	if err := tx.stage(); err != nil {
		tx.cleanup()
		return 0, err
	}
	if err := tx.commit(); err != nil {
		rollbackErr := tx.rollback()
		tx.cleanup()
		if rollbackErr != nil {
			return 0, fmt.Errorf("committing update: %w (rollback failed: %v)", err, rollbackErr)
		}
		return 0, fmt.Errorf("committing update: %w", err)
	}
	tx.cleanup()
	return len(targets), nil
}

type stagedTarget struct {
	target  Target
	staged  string
	backup  string
	hadDest bool
	moved   bool
}

type transaction struct {
	targets []Target
	hooks   Hooks
	staged  []stagedTarget
}

func validateTargets(targets []Target) error {
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		if target.SourceDir == "" {
			return fmt.Errorf("update target %q has an empty source directory", target.Name)
		}
		if target.Destination == "" {
			return fmt.Errorf("update target %q has an empty destination", target.Name)
		}
		dest, err := filepath.Abs(filepath.Clean(target.Destination))
		if err != nil {
			return fmt.Errorf("resolve destination for %q: %w", target.Name, err)
		}
		if _, ok := seen[dest]; ok {
			return fmt.Errorf("duplicate update destination %q", target.Destination)
		}
		seen[dest] = struct{}{}
		if _, err := os.Stat(target.SourceDir); err != nil {
			return fmt.Errorf("source for %q is unavailable: %w", target.Name, err)
		}
	}

	// Reject nested destinations. Committing one destination would otherwise
	// move or remove another target's path and make rollback ambiguous.
	for i := 0; i < len(targets); i++ {
		for j := i + 1; j < len(targets); j++ {
			if pathsOverlap(targets[i].Destination, targets[j].Destination) {
				return fmt.Errorf("update destinations overlap: %q and %q", targets[i].Destination, targets[j].Destination)
			}
		}
	}
	return nil
}

func (tx *transaction) stage() error {
	for _, target := range tx.targets {
		parent := filepath.Dir(target.Destination)
		if err := os.MkdirAll(parent, 0755); err != nil {
			return fmt.Errorf("create staging parent for %q: %w", target.Name, err)
		}

		staged, err := os.MkdirTemp(parent, ".sm-update-stage-*")
		if err != nil {
			return fmt.Errorf("create staging directory for %q: %w", target.Name, err)
		}
		// CopyDir requires a destination that does not exist. Keep the unique
		// path reservation from MkdirTemp, then recreate it as the copy root.
		if err := os.Remove(staged); err != nil {
			os.RemoveAll(staged)
			return fmt.Errorf("prepare staging directory for %q: %w", target.Name, err)
		}
		if err := fsutil.CopyDir(target.SourceDir, staged); err != nil {
			os.RemoveAll(staged)
			return fmt.Errorf("stage %q: %w", target.Name, err)
		}

		entry := stagedTarget{target: target, staged: staged}
		tx.staged = append(tx.staged, entry)
		if tx.hooks.Prepare != nil {
			if err := tx.hooks.Prepare(target, staged); err != nil {
				return fmt.Errorf("prepare %q: %w", target.Name, err)
			}
		}
		if tx.hooks.Validate != nil {
			if err := tx.hooks.Validate(target, staged); err != nil {
				return fmt.Errorf("validate %q: %w", target.Name, err)
			}
		}
	}
	return nil
}

func (tx *transaction) commit() error {
	for i := range tx.staged {
		entry := &tx.staged[i]
		if _, err := os.Lstat(entry.target.Destination); err == nil {
			backup, backupErr := temporarySibling(entry.target.Destination, ".sm-update-backup-*")
			if backupErr != nil {
				return fmt.Errorf("reserve backup for %q: %w", entry.target.Name, backupErr)
			}
			if err := os.Rename(entry.target.Destination, backup); err != nil {
				os.RemoveAll(backup)
				return fmt.Errorf("backup %q: %w", entry.target.Name, err)
			}
			entry.backup = backup
			entry.hadDest = true
		}
		if err := os.Rename(entry.staged, entry.target.Destination); err != nil {
			return fmt.Errorf("replace %q: %w", entry.target.Name, err)
		}
		entry.moved = true
	}
	return nil
}

func (tx *transaction) rollback() error {
	var errs []string
	for i := len(tx.staged) - 1; i >= 0; i-- {
		entry := &tx.staged[i]
		if entry.moved {
			if err := os.RemoveAll(entry.target.Destination); err != nil {
				errs = append(errs, fmt.Sprintf("remove new %q: %v", entry.target.Name, err))
			}
		}
		if entry.hadDest {
			if err := os.Rename(entry.backup, entry.target.Destination); err != nil {
				errs = append(errs, fmt.Sprintf("restore %q: %v", entry.target.Name, err))
			} else {
				entry.backup = ""
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func (tx *transaction) cleanup() {
	for _, entry := range tx.staged {
		if entry.staged != "" {
			_ = os.RemoveAll(entry.staged)
		}
		if entry.backup != "" {
			_ = os.RemoveAll(entry.backup)
		}
	}
}

func temporarySibling(path, pattern string) (string, error) {
	file, err := os.CreateTemp(filepath.Dir(path), pattern)
	if err != nil {
		return "", err
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}

func pathsOverlap(a, b string) bool {
	absA, errA := filepath.Abs(filepath.Clean(a))
	absB, errB := filepath.Abs(filepath.Clean(b))
	if errA != nil || errB != nil {
		return false
	}
	return absA == absB ||
		strings.HasPrefix(absA, absB+string(filepath.Separator)) ||
		strings.HasPrefix(absB, absA+string(filepath.Separator))
}

package installer

// placement.go owns the filesystem contract for putting one skill directory
// in an agent's skill directory.  Keeping this contract here gives profile
// and registry installs one implementation, and gives Direct Install a seam
// it can use without duplicating target/conflict/fallback logic.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/woyin/skills-manager/internal/concurrency"
	"github.com/woyin/skills-manager/internal/fsutil"
	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/tool"
)

// PlacementMode controls how a source directory is materialized at a target.
// SymlinkMode is the default and preserves the registry-first install model.
type PlacementMode uint8

const (
	SymlinkMode PlacementMode = iota
	CopyMode
)

// ModeSymlink and ModeCopy are descriptive aliases for callers that prefer
// the shorter names.  Keep the aliases in this package so the mode remains a
// single contract when Direct Install starts using Placement.
const (
	ModeSymlink = SymlinkMode
	ModeCopy    = CopyMode
)

// SymlinkFallback controls what happens when a requested symlink cannot be
// created.  CopyFallback is intentionally opt-in: existing profile and
// registry installs historically report symlink errors, while Direct Install
// can opt in to the more permissive npx-skills-compatible behavior.
type SymlinkFallback uint8

const (
	NoSymlinkFallback SymlinkFallback = iota
	CopyOnSymlinkFailure
)

// FallbackNone/FallbackCopy are aliases suitable for option literals.
const (
	FallbackNone = NoSymlinkFallback
	FallbackCopy = CopyOnSymlinkFailure
)

// ConflictPolicy controls an existing destination.
//
// Prompt applies to an existing symlink with a different target.  A regular
// file or directory remains an error under Prompt, matching Installer's
// previous behavior.  Replace is useful for copy/direct installs; Skip keeps
// the existing entity and reports an unapplied result; Error never prompts.
type ConflictPolicy uint8

const (
	PromptOnConflict ConflictPolicy = iota
	ReplaceOnConflict
	SkipOnConflict
	ErrorOnConflict
)

// ConflictPolicy aliases make option literals read naturally while retaining
// one enum for the implementation.
const (
	ConflictPrompt  = PromptOnConflict
	ConflictReplace = ReplaceOnConflict
	ConflictSkip    = SkipOnConflict
	ConflictError   = ErrorOnConflict
)

// TargetScope determines whether target directories are resolved from the
// user's home or a project root.
type TargetScope uint8

const (
	ProjectScope TargetScope = iota
	GlobalScope
)

// PlacementTarget is one agent's resolved skill directory.  Directory is
// absolute for global scope and follows tool.GetProjectSkillDir for project
// scope.  Name is informational and lets callers report which agent owns the
// destination without re-deriving it from a path.
type PlacementTarget struct {
	Name      string
	Directory string
}

// TargetDirectories resolves the target roots for tools in a given scope.
// Tools without a project-level directory are omitted, exactly as the old
// Installer did.  The order is stable and follows the input tool slice.
func TargetDirectories(tools []tool.Tool, projectDir string, scope TargetScope) []PlacementTarget {
	result := make([]PlacementTarget, 0, len(tools))
	for _, candidate := range tools {
		directory := ""
		if scope == GlobalScope {
			directory = candidate.SkillDir
			if directory != "" && !filepath.IsAbs(directory) {
				directory = filepath.Join(home.Dir(), directory)
			}
		} else {
			directory = tool.GetProjectSkillDir(candidate, projectDir)
		}
		if directory == "" {
			continue
		}
		result = append(result, PlacementTarget{Name: candidate.Name, Directory: directory})
	}
	return result
}

// ResolveTargetDirectories is a descriptive alias for TargetDirectories.
func ResolveTargetDirectories(tools []tool.Tool, projectDir string, scope TargetScope) []PlacementTarget {
	return TargetDirectories(tools, projectDir, scope)
}

// PlacementOptions configures a Placement engine.
//
// Input and Output are only used for PromptOnConflict.  CreateSymlink and
// CopyDirectory are optional seams for deterministic tests and for callers
// that need to instrument filesystem work; nil uses os.Symlink and
// fsutil.CopyDir respectively.
type PlacementOptions struct {
	Mode          PlacementMode
	Fallback      SymlinkFallback
	Conflict      ConflictPolicy
	Input         io.Reader
	Output        io.Writer
	CreateSymlink func(target, destination string) error
	CopyDirectory func(source, destination string) error
	RejectOverlap bool
}

// PlacementRequest is one independent batch placement.  Label is optional
// metadata for callers that need to associate an outcome with an agent/skill;
// Placement itself only uses Source and Destination.
type PlacementRequest struct {
	Source      string
	Destination string
	Label       string
}

// PlacementOutcome preserves request order while carrying either a successful
// result or the error for one batch item.  PlaceMany deliberately does not
// stop on an individual error: Direct Install historically reports per-agent
// failures while allowing other jobs to finish.
type PlacementOutcome struct {
	Request PlacementRequest
	Result  *PlacementResult
	Err     error
}

// PlaceMany applies independent placements concurrently and returns outcomes
// in request order.  Results are left uncommitted so a caller that wants a
// transaction can roll them back; callers that do not need rollback should
// call Commit on successful outcomes after observing them.
func (p *Placement) PlaceMany(requests []PlacementRequest, maxWorkers int) []PlacementOutcome {
	outcomes := make([]PlacementOutcome, len(requests))
	if len(requests) == 0 {
		return outcomes
	}
	if maxWorkers <= 0 {
		maxWorkers = 1
	}
	concurrency.RunIndexed(len(requests), maxWorkers, func(i int) {
		request := requests[i]
		result, err := p.Place(request.Source, request.Destination)
		outcomes[i] = PlacementOutcome{Request: request, Result: result, Err: err}
	})
	return outcomes
}

// CopySkill is the compatibility helper for command code that needs a
// stand-alone copy.  It uses the same replacement/snapshot contract as
// CopyMode placement and commits immediately.
func CopySkill(source, destination string) error {
	result, err := NewPlacement(PlacementOptions{
		Mode:          CopyMode,
		Conflict:      ReplaceOnConflict,
		RejectOverlap: true,
	}).Place(source, destination)
	if err != nil {
		return err
	}
	return result.Commit()
}

// SourceDestinationOverlapError identifies the safety guard that rejects a
// source and destination containing one another.  Command layers can use
// errors.As to retain a tailored warning without reimplementing the guard.
type SourceDestinationOverlapError struct {
	Source      string
	Destination string
}

func (e *SourceDestinationOverlapError) Error() string {
	return fmt.Sprintf("source %s overlaps destination %s", e.Source, e.Destination)
}

// Placement is a small, stateful filesystem engine.  A PlacementResult keeps
// enough state to either commit a replacement or roll it back.  The engine is
// safe to reuse for multiple destinations; options are immutable after
// construction.
type Placement struct {
	options PlacementOptions
}

// NewPlacement constructs a placement engine.  Symlink mode, prompt conflicts
// and no fallback are the zero-value behavior, matching existing Installer
// semantics.  Copy mode defaults to replacement because Copy Install has
// always overwritten an existing copy/symlink.
func NewPlacement(options PlacementOptions) *Placement {
	if options.Input == nil {
		options.Input = os.Stdin
	}
	if options.Output == nil {
		options.Output = os.Stderr
	}
	if options.CreateSymlink == nil {
		options.CreateSymlink = os.Symlink
	}
	if options.CopyDirectory == nil {
		options.CopyDirectory = fsutil.CopyDir
	}
	if options.Mode == CopyMode && options.Conflict == PromptOnConflict {
		options.Conflict = ReplaceOnConflict
	}
	return &Placement{options: options}
}

// PlacementResult describes one attempted placement.
//
// Applied is true for a successful placement and for an idempotent no-op;
// Changed distinguishes a filesystem mutation and is what transactional
// callers should retain for rollback.  ActualMode reports CopyMode when a
// symlink request used CopyOnSymlinkFailure.
type PlacementResult struct {
	Source        string
	Destination   string
	RequestedMode PlacementMode
	ActualMode    PlacementMode
	Applied       bool
	Changed       bool
	Fallback      bool

	state *placementState
}

// Commit releases any saved destination snapshot.  It is safe to call more
// than once and is a no-op for an idempotent placement.
func (r *PlacementResult) Commit() error {
	if r == nil || r.state == nil {
		return nil
	}
	return r.state.commit()
}

// Rollback restores the destination to the state it had before Place.  It is
// safe to call more than once and is a no-op for an unapplied/idempotent
// placement.  Rollback is deliberately explicit so a caller can compose
// several placements into a transaction without the placement package
// knowing about profile/MCP persistence.
func (r *PlacementResult) Rollback() error {
	if r == nil || r.state == nil {
		return nil
	}
	return r.state.rollback()
}

type placementState struct {
	destination string
	backup      string
	backupDir   string
	done        bool
}

func (s *placementState) commit() error {
	if s == nil || s.done {
		return nil
	}
	s.done = true
	if s.backupDir == "" {
		return nil
	}
	return os.RemoveAll(s.backupDir)
}

func (s *placementState) rollback() error {
	if s == nil || s.done {
		return nil
	}
	s.done = true

	var errs []error
	if err := os.RemoveAll(s.destination); err != nil {
		errs = append(errs, fmt.Errorf("removing replacement %s: %w", s.destination, err))
	}
	if s.backup == "" {
		return errors.Join(errs...)
	}
	if err := os.Rename(s.backup, s.destination); err != nil {
		errs = append(errs, fmt.Errorf("restoring previous %s: %w", s.destination, err))
	}
	if s.backupDir != "" {
		if err := os.RemoveAll(s.backupDir); err != nil {
			errs = append(errs, fmt.Errorf("removing placement backup: %w", err))
		}
	}
	return errors.Join(errs...)
}

// Place materializes source at destination according to the engine options.
// Source paths are made absolute for symlinks, preserving the Installer's
// absolute-target contract.  A conflict that the policy skips returns a
// non-nil result with Applied=false and nil error.
func (p *Placement) Place(source, destination string) (*PlacementResult, error) {
	if p == nil {
		p = NewPlacement(PlacementOptions{})
	}
	absSource, err := filepath.Abs(source)
	if err != nil {
		return nil, fmt.Errorf("resolving source %q: %w", source, err)
	}
	absDestination, err := filepath.Abs(destination)
	if err != nil {
		return nil, fmt.Errorf("resolving destination %q: %w", destination, err)
	}
	if p.options.RejectOverlap && pathsOverlap(absSource, absDestination) {
		return nil, &SourceDestinationOverlapError{Source: absSource, Destination: absDestination}
	}

	result := &PlacementResult{
		Source:        absSource,
		Destination:   absDestination,
		RequestedMode: p.options.Mode,
		ActualMode:    p.options.Mode,
	}

	if err := os.MkdirAll(filepath.Dir(absDestination), 0755); err != nil {
		return nil, fmt.Errorf("creating parent dir: %w", err)
	}

	existing, err := inspectDestination(absDestination)
	if err != nil {
		return nil, err
	}
	if existing.exists && p.options.Mode == SymlinkMode && existing.symlinkTarget != "" {
		if normalizeLinkTarget(absDestination, existing.symlinkTarget) == absSource {
			result.Applied = true
			return result, nil
		}
	}

	if existing.exists {
		allow, err := p.resolveConflict(absDestination, existing, absSource)
		if err != nil {
			return nil, err
		}
		if !allow {
			return result, nil
		}
	}

	state, err := snapshotDestination(absDestination, existing.exists)
	if err != nil {
		return nil, err
	}
	result.state = state

	if p.options.Mode == CopyMode {
		if err := p.copy(absSource, absDestination); err != nil {
			_ = state.rollback()
			return nil, fmt.Errorf("copying %s to %s: %w", absSource, absDestination, err)
		}
		result.Applied = true
		result.Changed = true
		return result, nil
	}

	if err := p.options.CreateSymlink(absSource, absDestination); err == nil {
		result.Applied = true
		result.Changed = true
		return result, nil
	} else if p.options.Fallback != CopyOnSymlinkFailure {
		_ = state.rollback()
		return nil, fmt.Errorf("creating symlink %s -> %s: %w", absDestination, absSource, err)
	} else {
		// A failed Symlink call should not leave a destination behind, but a
		// custom filesystem or a race may have done so.  Remove only the path
		// that this operation owns before attempting the copy fallback.
		if removeErr := os.RemoveAll(absDestination); removeErr != nil {
			_ = state.rollback()
			return nil, fmt.Errorf("symlink failed (%v), removing destination for copy fallback: %w", err, removeErr)
		}
		if copyErr := p.copy(absSource, absDestination); copyErr != nil {
			_ = state.rollback()
			return nil, fmt.Errorf("symlink failed (%v); copying fallback to %s: %w", err, absDestination, copyErr)
		}
		result.Applied = true
		result.Changed = true
		result.ActualMode = CopyMode
		result.Fallback = true
		return result, nil
	}
}

func (p *Placement) copy(source, destination string) error {
	return p.options.CopyDirectory(source, destination)
}

type destinationInfo struct {
	exists        bool
	symlinkTarget string
}

func inspectDestination(destination string) (destinationInfo, error) {
	info, err := os.Lstat(destination)
	if err != nil {
		if os.IsNotExist(err) {
			return destinationInfo{}, nil
		}
		return destinationInfo{}, fmt.Errorf("inspecting destination %s: %w", destination, err)
	}
	result := destinationInfo{exists: true}
	if info.Mode()&os.ModeSymlink != 0 {
		target, readErr := os.Readlink(destination)
		if readErr != nil {
			return destinationInfo{}, fmt.Errorf("reading existing symlink %s: %w", destination, readErr)
		}
		result.symlinkTarget = target
	}
	return result, nil
}

func (p *Placement) resolveConflict(destination string, existing destinationInfo, source string) (bool, error) {
	policy := p.options.Conflict
	if policy == ReplaceOnConflict {
		return true, nil
	}
	if policy == SkipOnConflict {
		return false, nil
	}
	if policy == ErrorOnConflict {
		return false, fmt.Errorf("%s already exists", destination)
	}

	// PromptOnConflict preserves the old Installer contract: only a symlink
	// can be interactively replaced; a real file or directory is an error.
	if existing.symlinkTarget == "" {
		return false, fmt.Errorf("%s already exists and is not a symlink", destination)
	}
	return p.confirmReplace(destination, normalizeLinkTarget(destination, existing.symlinkTarget), source), nil
}

func (p *Placement) confirmReplace(destination, existingTarget, source string) bool {
	fmt.Fprintf(p.options.Output, "warning: %s already points to %s (want %s)\n", destination, existingTarget, source)
	fmt.Fprint(p.options.Output, "Replace it? [y/N]: ")
	var answer string
	if _, err := fmt.Fscan(p.options.Input, &answer); err != nil {
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

// snapshotDestination moves an existing destination to a sibling temporary
// directory.  Keeping the snapshot beside the destination avoids cross-device
// rename failures and lets Rollback restore regular directories/files as well
// as symlinks.  A nil snapshot means this placement created a new path.
func snapshotDestination(destination string, exists bool) (*placementState, error) {
	state := &placementState{destination: destination}
	if !exists {
		return state, nil
	}
	backupDir, err := os.MkdirTemp(filepath.Dir(destination), ".sm-placement-backup-*")
	if err != nil {
		return nil, fmt.Errorf("creating destination backup: %w", err)
	}
	backup := filepath.Join(backupDir, filepath.Base(destination))
	if err := os.Rename(destination, backup); err != nil {
		_ = os.RemoveAll(backupDir)
		return nil, fmt.Errorf("saving existing destination %s: %w", destination, err)
	}
	state.backup = backup
	state.backupDir = backupDir
	return state, nil
}

func normalizeLinkTarget(link, target string) string {
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(link), target)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return filepath.Clean(target)
	}
	return filepath.Clean(abs)
}

func pathsOverlap(source, destination string) bool {
	source = filepath.Clean(source)
	destination = filepath.Clean(destination)
	return pathContains(source, destination) || pathContains(destination, source)
}

// PathsOverlap reports whether source and destination are equal or one is
// nested inside the other.  It is exported for command layers that need to
// preserve a specialized diagnostic while keeping path semantics centralized.
func PathsOverlap(source, destination string) bool {
	absSource, sourceErr := filepath.Abs(filepath.Clean(source))
	absDestination, destinationErr := filepath.Abs(filepath.Clean(destination))
	if sourceErr != nil || destinationErr != nil {
		return false
	}
	return pathsOverlap(absSource, absDestination)
}

func pathContains(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

package installer

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/woyin/skills-manager/internal/fsutil"
	"github.com/woyin/skills-manager/internal/tool"
)

func makePlacementSource(t *testing.T, contents string) string {
	t.Helper()
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
	return source
}

func TestPlacementSymlinkRollbackAndCommit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	source := makePlacementSource(t, "new")
	destination := filepath.Join(t.TempDir(), "agent", "skills", "skill")
	placement := NewPlacement(PlacementOptions{RejectOverlap: true})

	result, err := placement.Place(source, destination)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if !result.Applied || !result.Changed || result.ActualMode != SymlinkMode {
		t.Fatalf("unexpected result: %+v", result)
	}
	target, err := os.Readlink(destination)
	if err != nil || target != source {
		t.Fatalf("symlink target = %q, err=%v; want %q", target, err, source)
	}
	if err := result.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination should be removed by rollback, err=%v", err)
	}

	result, err = placement.Place(source, destination)
	if err != nil {
		t.Fatalf("second Place: %v", err)
	}
	if err := result.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	// Repeating the operation is an applied no-op.  Its rollback must not
	// remove the pre-existing link.
	repeat, err := placement.Place(source, destination)
	if err != nil {
		t.Fatalf("idempotent Place: %v", err)
	}
	if !repeat.Applied || repeat.Changed {
		t.Fatalf("expected idempotent result, got %+v", repeat)
	}
	if err := repeat.Rollback(); err != nil {
		t.Fatalf("idempotent Rollback: %v", err)
	}
	if _, err := os.Lstat(destination); err != nil {
		t.Fatalf("idempotent rollback removed destination: %v", err)
	}
}

func TestPlacementConflictRollbackRestoresExistingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	root := t.TempDir()
	oldSource := filepath.Join(root, "old")
	if err := os.MkdirAll(oldSource, 0755); err != nil {
		t.Fatal(err)
	}
	newSource := makePlacementSource(t, "new")
	destination := filepath.Join(root, "agent", "skill")
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(oldSource, destination); err != nil {
		t.Fatal(err)
	}

	placement := NewPlacement(PlacementOptions{
		Input:    bytes.NewBufferString("y\n"),
		Output:   &bytes.Buffer{},
		Conflict: PromptOnConflict,
	})
	result, err := placement.Place(newSource, destination)
	if err != nil {
		t.Fatalf("Place replacement: %v", err)
	}
	if !result.Applied || !result.Changed {
		t.Fatalf("expected replacement, got %+v", result)
	}
	if err := result.Rollback(); err != nil {
		t.Fatalf("Rollback replacement: %v", err)
	}
	target, err := os.Readlink(destination)
	if err != nil || target != oldSource {
		t.Fatalf("rollback target = %q, err=%v; want %q", target, err, oldSource)
	}
}

func TestPlacementCopyReplacementRollbackRestoresDirectory(t *testing.T) {
	root := t.TempDir()
	oldSource := filepath.Join(root, "old")
	if err := os.MkdirAll(oldSource, 0755); err != nil {
		t.Fatal(err)
	}
	newSource := makePlacementSource(t, "new")
	destination := filepath.Join(root, "agent", "skill")
	if err := os.MkdirAll(destination, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "old.txt"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewPlacement(PlacementOptions{Mode: CopyMode}).Place(newSource, destination)
	if err != nil {
		t.Fatalf("copy Place: %v", err)
	}
	if result.ActualMode != CopyMode || !result.Changed {
		t.Fatalf("unexpected copy result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(destination, "SKILL.md")); err != nil {
		t.Fatalf("new copy missing: %v", err)
	}
	if err := result.Rollback(); err != nil {
		t.Fatalf("copy Rollback: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "old.txt")); err != nil {
		t.Fatalf("old directory not restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("new copy survived rollback: %v", err)
	}
}

func TestPlacementSymlinkFallbackUsesCopy(t *testing.T) {
	source := makePlacementSource(t, "fallback")
	destination := filepath.Join(t.TempDir(), "agent", "skill")
	var symlinkCalls, copyCalls int
	placement := NewPlacement(PlacementOptions{
		Fallback: CopyOnSymlinkFailure,
		CreateSymlink: func(_, _ string) error {
			symlinkCalls++
			return os.ErrPermission
		},
		CopyDirectory: func(source, destination string) error {
			copyCalls++
			return fsutil.CopyDir(source, destination)
		},
	})
	result, err := placement.Place(source, destination)
	if err != nil {
		t.Fatalf("fallback Place: %v", err)
	}
	if symlinkCalls != 1 || copyCalls != 1 || !result.Fallback || result.ActualMode != CopyMode {
		t.Fatalf("unexpected fallback result: calls=(%d,%d), result=%+v", symlinkCalls, copyCalls, result)
	}
	data, err := os.ReadFile(filepath.Join(destination, "SKILL.md"))
	if err != nil || string(data) != "fallback" {
		t.Fatalf("fallback copy contents = %q, err=%v", data, err)
	}
	if err := result.Rollback(); err != nil {
		t.Fatalf("fallback Rollback: %v", err)
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("fallback destination survived rollback: %v", err)
	}
}

func TestPlacementConflictPolicies(t *testing.T) {
	source := makePlacementSource(t, "source")
	destination := filepath.Join(t.TempDir(), "skill")
	if err := os.WriteFile(destination, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := NewPlacement(PlacementOptions{Conflict: PromptOnConflict}).Place(source, destination); err == nil {
		t.Fatal("PromptOnConflict should reject a regular file")
	}
	skipped, err := NewPlacement(PlacementOptions{Conflict: SkipOnConflict}).Place(source, destination)
	if err != nil {
		t.Fatalf("SkipOnConflict: %v", err)
	}
	if skipped.Applied || skipped.Changed {
		t.Fatalf("skip should be unapplied and unchanged: %+v", skipped)
	}
}

func TestTargetDirectoriesResolvesScopeAndSkipsUnsupportedProjectTargets(t *testing.T) {
	base := t.TempDir()
	tools := []tool.Tool{
		{Name: "project", SkillDir: filepath.Join(base, "global"), ProjectSkillDir: filepath.Join(".agent", "skills")},
		{Name: "global-only", SkillDir: filepath.Join(base, "global-only")},
	}
	project := filepath.Join(base, "project")
	projectTargets := TargetDirectories(tools, project, ProjectScope)
	if len(projectTargets) != 1 || projectTargets[0].Name != "project" || projectTargets[0].Directory != filepath.Join(project, ".agent", "skills") {
		t.Fatalf("project targets = %+v", projectTargets)
	}
	globalTargets := TargetDirectories(tools, project, GlobalScope)
	if len(globalTargets) != 2 || globalTargets[0].Directory != tools[0].SkillDir || globalTargets[1].Directory != tools[1].SkillDir {
		t.Fatalf("global targets = %+v", globalTargets)
	}
}

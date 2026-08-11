package updater

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, root, name, body string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func readSkill(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestApplyStagesAndCommitsAllTargets(t *testing.T) {
	sources := t.TempDir()
	destinations := t.TempDir()
	firstSource := writeTree(t, sources, "first", "first-new")
	secondSource := writeTree(t, sources, "second", "second-new")
	firstDest := writeTree(t, destinations, "first", "first-old")
	secondDest := writeTree(t, destinations, "second", "second-old")

	prepared := map[string]bool{}
	count, err := Apply([]Target{
		{Name: "first", SourceDir: firstSource, Destination: firstDest},
		{Name: "second", SourceDir: secondSource, Destination: secondDest},
	}, Hooks{
		Prepare: func(target Target, staged string) error {
			prepared[target.Name] = true
			return os.WriteFile(filepath.Join(staged, "metadata"), []byte("prepared"), 0644)
		},
		Validate: func(target Target, staged string) error {
			if _, err := os.Stat(filepath.Join(staged, "metadata")); err != nil {
				return err
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if count != 2 || !prepared["first"] || !prepared["second"] {
		t.Fatalf("count/prepared = %d/%v", count, prepared)
	}
	if got := readSkill(t, firstDest); got != "first-new" {
		t.Fatalf("first destination = %q", got)
	}
	if got := readSkill(t, secondDest); got != "second-new" {
		t.Fatalf("second destination = %q", got)
	}
	for _, dest := range []string{firstDest, secondDest} {
		if _, err := os.Stat(filepath.Join(dest, "metadata")); err != nil {
			t.Fatalf("metadata missing from %s: %v", dest, err)
		}
	}
}

func TestApplyValidationFailureLeavesEveryDestinationUntouched(t *testing.T) {
	sources := t.TempDir()
	destinations := t.TempDir()
	firstSource := writeTree(t, sources, "first", "first-new")
	secondSource := writeTree(t, sources, "second", "second-new")
	firstDest := writeTree(t, destinations, "first", "first-old")
	secondDest := writeTree(t, destinations, "second", "second-old")

	count, err := Apply([]Target{
		{Name: "first", SourceDir: firstSource, Destination: firstDest},
		{Name: "second", SourceDir: secondSource, Destination: secondDest},
	}, Hooks{
		Validate: func(target Target, staged string) error {
			if target.Name == "second" {
				return errors.New("invalid staged skill")
			}
			return nil
		},
	})
	if err == nil || count != 0 {
		t.Fatalf("Apply = count %d, err %v; want zero count and error", count, err)
	}
	if !strings.Contains(err.Error(), "validate \"second\"") {
		t.Fatalf("error = %v", err)
	}
	if got := readSkill(t, firstDest); got != "first-old" {
		t.Fatalf("first destination changed after validation failure: %q", got)
	}
	if got := readSkill(t, secondDest); got != "second-old" {
		t.Fatalf("second destination changed after validation failure: %q", got)
	}
}

func TestApplyCommitFailureRollsBackEarlierDestination(t *testing.T) {
	sources := t.TempDir()
	destinations := t.TempDir()
	firstSource := writeTree(t, sources, "first", "first-new")
	secondSource := writeTree(t, sources, "second", "second-new")
	firstDest := writeTree(t, destinations, "first", "first-old")
	secondDest := filepath.Join(destinations, "blocked", "second")
	if err := os.MkdirAll(filepath.Dir(secondDest), 0755); err != nil {
		t.Fatal(err)
	}

	// Both targets stage successfully. Before commit reaches the second target,
	// turn its parent into a regular file; reserving the second backup then
	// fails, exercising rollback of the first already-committed destination.
	validate := func(target Target, _ string) error {
		if target.Name != "second" {
			return nil
		}
		parent := filepath.Dir(secondDest)
		if err := os.RemoveAll(parent); err != nil {
			return err
		}
		return os.WriteFile(parent, []byte("blocked"), 0644)
	}

	count, err := Apply([]Target{
		{Name: "first", SourceDir: firstSource, Destination: firstDest},
		{Name: "second", SourceDir: secondSource, Destination: secondDest},
	}, Hooks{Validate: validate})
	if err == nil || count != 0 {
		t.Fatalf("Apply = count %d, err %v; want zero count and error", count, err)
	}
	if got := readSkill(t, firstDest); got != "first-old" {
		t.Fatalf("first destination not restored: %q", got)
	}
}

func TestApplyRejectsOverlappingDestinations(t *testing.T) {
	root := t.TempDir()
	source := writeTree(t, root, "source", "content")
	parent := filepath.Join(root, "dest")
	if _, err := Apply([]Target{
		{Name: "parent", SourceDir: source, Destination: parent},
		{Name: "child", SourceDir: source, Destination: filepath.Join(parent, "child")},
	}, Hooks{}); err == nil {
		t.Fatal("expected overlapping destination error")
	}
}

func TestApplyRejectsInvalidTargetsAndCleansUpPrepareFailure(t *testing.T) {
	if count, err := Apply(nil, Hooks{}); err != nil || count != 0 {
		t.Fatalf("empty Apply = %d, %v", count, err)
	}
	for _, target := range []Target{
		{Name: "missing source", Destination: filepath.Join(t.TempDir(), "dest")},
		{Name: "missing destination", SourceDir: t.TempDir()},
		{Name: "unavailable source", SourceDir: filepath.Join(t.TempDir(), "missing"), Destination: filepath.Join(t.TempDir(), "dest")},
	} {
		if _, err := Apply([]Target{target}, Hooks{}); err == nil {
			t.Fatalf("Apply should reject %+v", target)
		}
	}

	root := t.TempDir()
	source := writeTree(t, root, "source", "new")
	destination := writeTree(t, root, "destination", "old")
	count, err := Apply([]Target{{Name: "skill", SourceDir: source, Destination: destination}}, Hooks{
		Prepare: func(Target, string) error { return errors.New("cannot prepare") },
	})
	if err == nil || count != 0 || !strings.Contains(err.Error(), "prepare \"skill\"") {
		t.Fatalf("prepare failure = count %d, err %v", count, err)
	}
	if got := readSkill(t, destination); got != "old" {
		t.Fatalf("prepare failure changed destination: %q", got)
	}
}

func TestApplyRejectsDuplicateDestinations(t *testing.T) {
	root := t.TempDir()
	source := writeTree(t, root, "source", "content")
	destination := filepath.Join(root, "destination")
	if _, err := Apply([]Target{
		{Name: "first", SourceDir: source, Destination: destination},
		{Name: "second", SourceDir: source, Destination: destination},
	}, Hooks{}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate destinations error = %v", err)
	}
}

func BenchmarkApplyMultiSourceUpdate(b *testing.B) {
	sources := b.TempDir()
	destinations := b.TempDir()
	targets := make([]Target, 24)
	for i := range targets {
		name := fmt.Sprintf("skill-%02d", i)
		source := filepath.Join(sources, name)
		if err := os.MkdirAll(source, 0755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("benchmark"), 0644); err != nil {
			b.Fatal(err)
		}
		targets[i] = Target{Name: name, SourceDir: source, Destination: filepath.Join(destinations, name)}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if count, err := Apply(targets, Hooks{}); err != nil || count != len(targets) {
			b.Fatalf("Apply = %d, %v", count, err)
		}
	}
}

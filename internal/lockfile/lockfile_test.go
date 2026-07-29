package lockfile

import (
	"os"
	"path/filepath"
	"testing"
)

// TestManagerLoadMissing verifies Load returns an empty lock when the file doesn't exist.
func TestManagerLoadMissing(t *testing.T) {
	dir := t.TempDir()
	lm := NewManager(dir)

	lock, err := lm.Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if lock.Version != currentVersion {
		t.Errorf("version = %d, want %d", lock.Version, currentVersion)
	}
	if len(lock.Skills) != 0 {
		t.Errorf("expected empty skills map, got %d entries", len(lock.Skills))
	}
}

// TestManagerSaveLoadRoundTrip verifies Save then Load preserves entries.
func TestManagerSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	lm := NewManager(dir)

	lock := &LocalLock{
		Version: currentVersion,
		Skills: map[string]*SkillEntry{
			"my-skill": {
				Source:       "owner/repo",
				SourceType:   "github",
				SourceURL:    "https://github.com/owner/repo",
				SkillPath:    "skills/my-skill/SKILL.md",
				ComputedHash: "abc123",
			},
		},
	}

	if err := lm.Save(lock); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := lm.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	entry, ok := loaded.Skills["my-skill"]
	if !ok {
		t.Fatal("my-skill not found in loaded lock")
	}
	if entry.Source != "owner/repo" {
		t.Errorf("Source = %q, want %q", entry.Source, "owner/repo")
	}
	if entry.ComputedHash != "abc123" {
		t.Errorf("ComputedHash = %q, want %q", entry.ComputedHash, "abc123")
	}
}

// TestManagerUpsert verifies Upsert adds and updates entries.
func TestManagerUpsert(t *testing.T) {
	dir := t.TempDir()
	lm := NewManager(dir)

	err := lm.Upsert("skill-a", &SkillEntry{Source: "owner/repo", SourceType: "github"})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	lock, _ := lm.Load()
	if len(lock.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(lock.Skills))
	}

	// Update existing entry
	err = lm.Upsert("skill-a", &SkillEntry{Source: "owner/repo2", SourceType: "github"})
	if err != nil {
		t.Fatalf("Upsert update: %v", err)
	}

	lock, _ = lm.Load()
	if lock.Skills["skill-a"].Source != "owner/repo2" {
		t.Errorf("Source = %q, want %q", lock.Skills["skill-a"].Source, "owner/repo2")
	}
}

// TestManagerRemove verifies Remove deletes entries.
func TestManagerRemove(t *testing.T) {
	dir := t.TempDir()
	lm := NewManager(dir)

	lm.Upsert("skill-a", &SkillEntry{Source: "owner/repo"})
	lm.Upsert("skill-b", &SkillEntry{Source: "owner/repo"})

	err := lm.Remove("skill-a")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	lock, _ := lm.Load()
	if _, ok := lock.Skills["skill-a"]; ok {
		t.Error("skill-a should have been removed")
	}
	if _, ok := lock.Skills["skill-b"]; !ok {
		t.Error("skill-b should still exist")
	}
}

// TestManagerExists verifies Exists reports file presence.
func TestManagerExists(t *testing.T) {
	dir := t.TempDir()
	lm := NewManager(dir)

	if lm.Exists() {
		t.Error("Exists should be false before Save")
	}

	lm.Upsert("x", &SkillEntry{Source: "s"})

	if !lm.Exists() {
		t.Error("Exists should be true after Save")
	}
}

// TestSortedNames verifies skill names are returned sorted.
func TestSortedNames(t *testing.T) {
	lock := &LocalLock{
		Skills: map[string]*SkillEntry{
			"zebra":  {},
			"apple":  {},
			"monkey": {},
		},
	}
	names := lock.SortedNames()
	expected := []string{"apple", "monkey", "zebra"}
	if len(names) != len(expected) {
		t.Fatalf("got %d names, want %d", len(names), len(expected))
	}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("names[%d] = %q, want %q", i, names[i], want)
		}
	}
}

// TestComputeHash verifies hashing is stable and content-sensitive.
func TestComputeHash(t *testing.T) {
	dir := t.TempDir()

	// Create a skill directory
	skillDir := filepath.Join(dir, "my-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My Skill"), 0644)
	os.WriteFile(filepath.Join(skillDir, "helper.md"), []byte("help"), 0644)

	hash1, err := ComputeHash(skillDir)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}

	// Same content → same hash
	hash2, err := ComputeHash(skillDir)
	if err != nil {
		t.Fatalf("ComputeHash second: %v", err)
	}
	if hash1 != hash2 {
		t.Error("hash changed for identical content")
	}

	// Different content → different hash
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Changed"), 0644)
	hash3, err := ComputeHash(skillDir)
	if err != nil {
		t.Fatalf("ComputeHash third: %v", err)
	}
	if hash1 == hash3 {
		t.Error("hash unchanged after content modification")
	}
}

// TestComputeHashSkipsGitDir verifies .git directory is excluded from hash.
func TestComputeHashSkipsGitDir(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill"), 0644)

	hash1, _ := ComputeHash(skillDir)

	// Add a .git directory with content
	gitDir := filepath.Join(skillDir, ".git")
	os.MkdirAll(gitDir, 0755)
	os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main"), 0644)

	hash2, _ := ComputeHash(skillDir)

	if hash1 != hash2 {
		t.Error("hash changed when .git content was added (should be skipped)")
	}
}

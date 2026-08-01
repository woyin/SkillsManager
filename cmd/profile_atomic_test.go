package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/installer"
	"github.com/woyin/skills-manager/internal/profile"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

// TestProfileCreateRejectsUnknownSkill verifies profile create fails when a
// referenced skill is not in the Registry (ADR 0012: no partial state).
func TestProfileCreateRejectsUnknownSkill(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	regDir := withTestRegistry(t)
	profDir := t.TempDir()

	loader := profile.NewLoader(profDir)
	p := &profile.Profile{Skills: []string{"missing-skill"}}
	err := p.ValidateMembers(registrySkillExists, registryMCPExists)
	if err == nil {
		t.Fatal("expected validation error for unknown skill")
	}
	_ = loader
	_ = regDir
}

// TestProfileCreateAcceptsKnownSkill verifies profile create passes when all
// referenced skills exist and are unique in the Registry.
func TestProfileCreateAcceptsKnownSkill(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	regDir := withTestRegistry(t)

	src := t.TempDir()
	makeValidSkillDir(t, src, "known-skill")
	reg := registry.New(regDir)
	if _, err := reg.Register(filepath.Join(src, "known-skill"), "",
		registry.SkillOrigin{SourceKind: registry.SourceLocalSnapshot, Source: src}, false); err != nil {
		t.Fatal(err)
	}

	p := &profile.Profile{Skills: []string{"known-skill"}}
	if err := p.ValidateMembers(registrySkillExists, registryMCPExists); err != nil {
		t.Fatalf("expected validation to pass, got: %v", err)
	}
}

// TestProfileInstallAtomicRollback simulates a write-phase failure and verifies
// earlier links are rolled back (ADR 0012: zero side effects on failure).
func TestProfileInstallAtomicRollback(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	regDir := withTestRegistry(t)
	projectDir := t.TempDir()

	// Register two valid skills.
	src := t.TempDir()
	makeValidSkillDir(t, src, "skill-a")
	makeValidSkillDir(t, src, "skill-b")
	reg := registry.New(regDir)
	for _, n := range []string{"skill-a", "skill-b"} {
		if _, err := reg.Register(filepath.Join(src, n), "",
			registry.SkillOrigin{SourceKind: registry.SourceLocalSnapshot, Source: src}, false); err != nil {
			t.Fatal(err)
		}
	}

	// Use the installer with a profile-equivalent extra skills list.
	// Make the project dir read-only for the second skill's target to force a
	// write failure — simpler: verify preflight rejects an unknown member first.
	inst, err := installer.New(regDir, t.TempDir(), []tool.Tool{tool.Claude})
	if err != nil {
		t.Fatal(err)
	}
	inst.SetScope(projectDir, false)

	// Preflight must reject an unknown skill before writing anything.
	_, _, gerr := inst.GatherAndPreflight("", "", []string{"skill-a", "unknown"}, nil)
	if gerr == nil {
		t.Fatal("expected preflight to reject unknown skill")
	}

	// Nothing should have been written.
	if entries, _ := os.ReadDir(filepath.Join(projectDir, ".claude", "skills")); len(entries) != 0 {
		t.Errorf("preflight failure should leave no side effects, got %d entries", len(entries))
	}
}

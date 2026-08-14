package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/project"
)

// setupPlanCommand sets RegistryDir and ProfilesDir to temp dirs and returns
// the project dir. Mirrors status_test.go's approach.
func setupPlanCommand(t *testing.T) string {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()

	if old := RegistryDir; old != "" {
		t.Cleanup(func() { RegistryDir = old })
	}
	RegistryDir = filepath.Join(t.TempDir(), "registry")
	ProfilesDir = filepath.Join(t.TempDir(), "profiles")
	if old := DataDir; old != "" {
		t.Cleanup(func() { DataDir = old })
	}
	DataDir = filepath.Join(t.TempDir(), "data")
	_ = os.MkdirAll(filepath.Join(RegistryDir, "skills", "global"), 0755)
	return t.TempDir()
}

func mkRegistrySkill(t *testing.T, name string) {
	t.Helper()
	dir := filepath.Join(RegistryDir, "skills", "global", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name+"\n"), 0644)
}

func TestPlanCommandBootstrap(t *testing.T) {
	projectDir := setupPlanCommand(t)
	mkRegistrySkill(t, "foo")

	planDir = projectDir
	planJSON = false
	planCheck = false
	planApply = false
	t.Cleanup(resetPlanFlags())

	var buf bytes.Buffer
	oldOut := planOut
	planOut = &buf
	defer func() { planOut = oldOut }()

	if err := runPlanCommand(); err != nil {
		t.Fatalf("runPlanCommand: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "bootstrap") {
		t.Errorf("expected bootstrap status, got:\n%s", out)
	}
}

func TestPlanCommandJSONShowsProposal(t *testing.T) {
	projectDir := setupPlanCommand(t)
	mkRegistrySkill(t, "foo")

	planDir = projectDir
	planJSON = true
	planCheck = false
	planApply = false
	t.Cleanup(resetPlanFlags())

	var buf bytes.Buffer
	oldOut := planOut
	planOut = &buf
	defer func() { planOut = oldOut }()

	if err := runPlanCommand(); err != nil {
		t.Fatalf("runPlanCommand: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"project"`) || !strings.Contains(out, `"check"`) {
		t.Errorf("expected JSON fields, got:\n%s", out)
	}
}

func TestPlanCommandApplyBootstrapRequiresTarget(t *testing.T) {
	projectDir := setupPlanCommand(t)
	planDir = projectDir
	planJSON = false
	planCheck = false
	planApply = true
	planProfile = ""
	planSkills = nil
	t.Cleanup(resetPlanFlags())
	t.Cleanup(func() { planProfile = ""; planSkills = nil })

	if err := runPlanCommand(); err == nil {
		t.Fatal("expected error when applying bootstrap plan without target")
	} else if !strings.Contains(err.Error(), "explicit curation target") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func resetPlanFlags() func() {
	old := func() {
		planDir = ""
		planJSON = false
		planCheck = false
		planApply = false
		planProfile = ""
		planSkills = nil
	}
	return old
}

func TestPlanCommandApplyRemovesOwnedLinkInstall(t *testing.T) {
	projectDir := setupPlanCommand(t)
	mkRegistrySkill(t, "foo")
	linkHost := filepath.Join(projectDir, ".claude", "skills")
	if err := os.MkdirAll(linkHost, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(RegistryDir, "skills", "global", "foo"), filepath.Join(linkHost, "foo")); err != nil {
		t.Fatal(err)
	}
	// Write .sm.json with a curation block that owns foo under claude.
	cfg := `{"curation":{"managed":{"claude":["foo"]}}}`
	if err := os.WriteFile(filepath.Join(projectDir, ".sm.json"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	planDir = projectDir
	planJSON = false
	planCheck = false
	planApply = true
	planProfile = ""
	planSkills = nil
	t.Cleanup(resetPlanFlags())

	var buf bytes.Buffer
	oldOut := planOut
	planOut = &buf
	defer func() { planOut = oldOut }()

	if err := runPlanCommand(); err != nil {
		t.Fatalf("runPlanCommand: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(linkHost, "foo")); !os.IsNotExist(err) {
		t.Errorf("expected owned Link Install foo to be removed")
	}
	out := buf.String()
	if !strings.Contains(out, "Applied curation plan") {
		t.Errorf("expected applied message, got:\n%s", out)
	}
}

func TestPlanCommandApplyBootstrapWithSkillTarget(t *testing.T) {
	projectDir := setupPlanCommand(t)
	mkRegistrySkill(t, "foo")
	placementLabel := "foo"
	_ = placementLabel

	planDir = projectDir
	planJSON = false
	planCheck = false
	planApply = true
	planProfile = ""
	planSkills = []string{"foo"}
	t.Cleanup(resetPlanFlags())
	t.Cleanup(func() { planProfile = ""; planSkills = nil })

	var buf bytes.Buffer
	oldOut := planOut
	planOut = &buf
	defer func() { planOut = oldOut }()

	if err := runPlanCommand(); err != nil {
		t.Fatalf("runPlanCommand: %v", err)
	}
	// Explicit target should be written, and foo installed + owned.
	pm := project.NewManager(projectDir)
	cfg, err := pm.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Skills) != 1 || cfg.Skills[0] != "foo" {
		t.Errorf("expected explicit skill target foo in .sm.json, got %+v", cfg.Skills)
	}
	if cfg.Curation == nil || cfg.Curation.Baseline == nil || len(cfg.Curation.Baseline.Skills) != 1 {
		t.Errorf("expected baseline recorded, got %+v", cfg.Curation)
	}
	// foo is a global-category skill and should have been installed + owned
	// in at least the default claude project skill dir.
	linkPath := filepath.Join(projectDir, filepath.FromSlash(".claude/skills/foo"))
	if _, err := os.Lstat(linkPath); err != nil {
		t.Errorf("expected foo installed as symlink: %v", err)
	}
	if cfg.Curation.Managed == nil || len(cfg.Curation.Managed["claude"]) == 0 {
		t.Errorf("expected foo recorded as owned under claude, got %+v", cfg.Curation.Managed)
	}
	out := buf.String()
	if !strings.Contains(out, "Applied curation plan") {
		t.Errorf("expected applied message, got:\n%s", out)
	}
}

func TestPlanCommandBootstrapShowsRecommendations(t *testing.T) {
	projectDir := setupPlanCommand(t)
	mkRegistrySkill(t, "foo")
	if err := os.MkdirAll(ProfilesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ProfilesDir, "web.json"),
		[]byte(`{"skills":["foo"],"mcp":[]}`), 0644); err != nil {
		t.Fatal(err)
	}

	planDir = projectDir
	planJSON = false
	planCheck = false
	planApply = false
	t.Cleanup(resetPlanFlags())

	var buf bytes.Buffer
	oldOut := planOut
	planOut = &buf
	defer func() { planOut = oldOut }()

	if err := runPlanCommand(); err != nil {
		t.Fatalf("runPlanCommand: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Available profiles") || !strings.Contains(out, "web") {
		t.Errorf("expected recommended profile web, got:\n%s", out)
	}
	if !strings.Contains(out, "Available registry skills") || !strings.Contains(out, "foo") {
		t.Errorf("expected recommended skill foo, got:\n%s", out)
	}
}

func TestPlanCommandApplyBootstrapWithProfileTarget(t *testing.T) {
	projectDir := setupPlanCommand(t)
	mkRegistrySkill(t, "foo")
	if err := os.MkdirAll(ProfilesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ProfilesDir, "web.json"),
		[]byte(`{"skills":["foo"],"mcp":[]}`), 0644); err != nil {
		t.Fatal(err)
	}

	planDir = projectDir
	planJSON = false
	planCheck = false
	planApply = true
	planProfile = "web"
	planSkills = nil
	t.Cleanup(resetPlanFlags())
	t.Cleanup(func() { planProfile = ""; planSkills = nil })

	var buf bytes.Buffer
	oldOut := planOut
	planOut = &buf
	defer func() { planOut = oldOut }()

	if err := runPlanCommand(); err != nil {
		t.Fatalf("runPlanCommand: %v", err)
	}

	pm := project.NewManager(projectDir)
	cfg, err := pm.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Profile != "web" {
		t.Errorf("expected explicit profile target web, got %q", cfg.Profile)
	}
	if cfg.Curation == nil || cfg.Curation.Baseline == nil || cfg.Curation.Baseline.Profile != "web" {
		t.Errorf("expected baseline for profile web, got %+v", cfg.Curation)
	}
	// The profile's foo skill should be installed and owned under claude.
	linkPath := filepath.Join(projectDir, filepath.FromSlash(".claude/skills/foo"))
	if _, err := os.Lstat(linkPath); err != nil {
		t.Errorf("expected foo installed as symlink: %v", err)
	}
	if cfg.Curation.Managed == nil || len(cfg.Curation.Managed["claude"]) == 0 {
		t.Errorf("expected foo owned under claude, got %+v", cfg.Curation.Managed)
	}
}

func TestPlanCommandFreshProjectAfterApplyIsNotBootstrap(t *testing.T) {
	projectDir := setupPlanCommand(t)
	mkRegistrySkill(t, "foo")

	// First apply sets an explicit target.
	planDir = projectDir
	planJSON = false
	planCheck = false
	planApply = true
	planProfile = ""
	planSkills = []string{"foo"}
	t.Cleanup(resetPlanFlags())
	t.Cleanup(func() { planProfile = ""; planSkills = nil })

	var buf bytes.Buffer
	oldOut := planOut
	planOut = &buf
	defer func() { planOut = oldOut }()
	if err := runPlanCommand(); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// A fresh plan run on the now-curated project must not be bootstrap.
	planJSON = true
	planApply = false
	buf.Reset()
	if err := runPlanCommand(); err != nil {
		t.Fatalf("second plan: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, `"bootstrap": true`) {
		t.Errorf("expected non-bootstrap after explicit target set, got:\n%s", out)
	}
}

func TestPlanCommandCheckFailsWhenBaselineUnsatisfied(t *testing.T) {
	projectDir := setupPlanCommand(t)
	mkRegistrySkill(t, "foo")
	// Baseline wants foo, but it is not installed.
	if err := os.WriteFile(filepath.Join(projectDir, ".sm.json"),
		[]byte(`{"skills":["foo"]}`), 0644); err != nil {
		t.Fatal(err)
	}

	planDir = projectDir
	planJSON = false
	planCheck = true
	planApply = false
	t.Cleanup(resetPlanFlags())

	var buf bytes.Buffer
	oldOut := planOut
	planOut = &buf
	defer func() { planOut = oldOut }()

	if err := runPlanCommand(); err == nil {
		t.Fatal("expected --check to fail when baseline member is missing")
	} else if !strings.Contains(err.Error(), "not satisfied") {
		t.Fatalf("unexpected check error: %v", err)
	}
}

func TestPlanCommandCheckPassesWhenBaselineSatisfied(t *testing.T) {
	projectDir := setupPlanCommand(t)
	mkRegistrySkill(t, "foo")
	linkHost := filepath.Join(projectDir, ".claude", "skills")
	if err := os.MkdirAll(linkHost, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(RegistryDir, "skills", "global", "foo"), filepath.Join(linkHost, "foo")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".sm.json"),
		[]byte(`{"skills":["foo"]}`), 0644); err != nil {
		t.Fatal(err)
	}

	planDir = projectDir
	planJSON = false
	planCheck = true
	planApply = false
	t.Cleanup(resetPlanFlags())

	var buf bytes.Buffer
	oldOut := planOut
	planOut = &buf
	defer func() { planOut = oldOut }()

	if err := runPlanCommand(); err != nil {
		t.Fatalf("expected --check to pass, got: %v", err)
	}
}

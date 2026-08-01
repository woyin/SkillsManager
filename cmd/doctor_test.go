// cmd/doctor_test.go
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDoctor(t *testing.T) {
	// Save original values
	origRegistryDir := RegistryDir
	origDataDir := DataDir
	origProfilesDir := ProfilesDir
	defer func() {
		RegistryDir = origRegistryDir
		DataDir = origDataDir
		ProfilesDir = origProfilesDir
	}()

	// Setup test environment
	tmpDir := t.TempDir()
	RegistryDir = filepath.Join(tmpDir, "registry")
	DataDir = filepath.Join(tmpDir, "data")
	ProfilesDir = filepath.Join(tmpDir, "profiles")

	// Create test directories
	for _, dir := range []string{
		filepath.Join(RegistryDir, "skills"),
		filepath.Join(RegistryDir, "mcp"),
		ProfilesDir,
		DataDir,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
	}

	results := runDoctor()

	// Should have results for various checks
	if len(results) == 0 {
		t.Fatal("expected some check results, got none")
	}

	// Check that we have directory checks
	foundDirCheck := false
	for _, r := range results {
		if r.Name == "Registry (skills)" || r.Name == "Registry (mcp)" {
			foundDirCheck = true
			if r.Status != "pass" {
				t.Errorf("expected directory check to pass, got %s: %s", r.Status, r.Message)
			}
		}
	}
	if !foundDirCheck {
		t.Error("expected directory check results")
	}
}

func TestCheckCLITools(t *testing.T) {
	results := checkCLITools()

	// Should have results for Git, Claude, Codex, Go
	if len(results) < 4 {
		t.Errorf("expected at least 4 CLI tool checks, got %d", len(results))
	}

	// Git should be found on most systems
	foundGit := false
	for _, r := range results {
		if r.Name == "Git" {
			foundGit = true
			// Git may or may not be installed, so we don't assert status
			break
		}
	}
	if !foundGit {
		t.Error("expected Git check result")
	}
}

func TestCheckDirectories(t *testing.T) {
	// Save original values
	origRegistryDir := RegistryDir
	origDataDir := DataDir
	origProfilesDir := ProfilesDir
	defer func() {
		RegistryDir = origRegistryDir
		DataDir = origDataDir
		ProfilesDir = origProfilesDir
	}()

	// Setup test environment
	tmpDir := t.TempDir()
	RegistryDir = filepath.Join(tmpDir, "registry")
	DataDir = filepath.Join(tmpDir, "data")
	ProfilesDir = filepath.Join(tmpDir, "profiles")

	// Create test directories
	for _, dir := range []string{
		filepath.Join(RegistryDir, "skills"),
		filepath.Join(RegistryDir, "mcp"),
		ProfilesDir,
		DataDir,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
	}

	results := checkDirectories()

	// All directories should pass
	for _, r := range results {
		if r.Status == "fail" {
			t.Errorf("directory check failed: %s - %s", r.Name, r.Message)
		}
	}
}

func TestCheckDatabase(t *testing.T) {
	// Save original value
	origDataDir := DataDir
	defer func() {
		DataDir = origDataDir
	}()

	// Setup test environment with no database
	tmpDir := t.TempDir()
	DataDir = tmpDir

	results := checkDatabase()

	// Should warn about missing database
	foundDBCheck := false
	for _, r := range results {
		if r.Name == "Database" {
			foundDBCheck = true
			if r.Status != "warn" {
				t.Errorf("expected database check to warn about missing db, got %s: %s", r.Status, r.Message)
			}
		}
	}
	if !foundDBCheck {
		t.Error("expected database check result")
	}
}

func TestCheckEnvironment(t *testing.T) {
	results := checkEnvironment()

	// Should have results for environment variables
	if len(results) == 0 {
		t.Fatal("expected environment check results, got none")
	}

	// Check that HOME is checked
	foundHOME := false
	for _, r := range results {
		if r.Name == "HOME" {
			foundHOME = true
			if r.Status != "pass" {
				t.Errorf("expected HOME check to pass, got %s: %s", r.Status, r.Message)
			}
		}
	}
	if !foundHOME {
		t.Error("expected HOME check result")
	}
}

func TestPrintDoctorResults(t *testing.T) {
	results := []checkResult{
		{Name: "Test1", Status: "pass", Message: "all good"},
		{Name: "Test2", Status: "warn", Message: "minor issue"},
		{Name: "Test3", Status: "fail", Message: "something broke"},
	}

	// Should not panic
	err := printDoctorResults(results)
	if err == nil {
		t.Error("expected error due to failed check")
	}
}

func TestDoctorCmdRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "doctor" {
			found = true
			break
		}
	}
	if !found {
		t.Error("doctor command not registered on root command")
	}
}

func TestCheckRegistryIntegrityDuplicateNames(t *testing.T) {
	origRegistryDir := RegistryDir
	defer func() { RegistryDir = origRegistryDir }()

	tmpDir := t.TempDir()
	RegistryDir = filepath.Join(tmpDir, "registry")

	// Same skill name in two categories → global-uniqueness conflict.
	for _, cat := range []string{"global", "codex-only"} {
		dir := filepath.Join(RegistryDir, "skills", cat, "demo")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: demo\ndescription: a demo skill\n---\n# demo"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	results := checkRegistryIntegrity()

	var conflict *checkResult
	for i := range results {
		if results[i].Name == "Registry conflicts" {
			conflict = &results[i]
			break
		}
	}
	if conflict == nil {
		t.Fatal("expected Registry conflicts check result")
	}
	if conflict.Status != "fail" {
		t.Errorf("status = %s, want fail (message: %s)", conflict.Status, conflict.Message)
	}
	if !strings.Contains(conflict.Message, "demo") {
		t.Errorf("message should mention duplicate name 'demo', got: %s", conflict.Message)
	}
}

func TestCheckRegistryIntegrityOrphanWarnsSnapshotPasses(t *testing.T) {
	origRegistryDir := RegistryDir
	defer func() { RegistryDir = origRegistryDir }()

	tmpDir := t.TempDir()
	RegistryDir = filepath.Join(tmpDir, "registry")

	writeSkill := func(name string) string {
		dir := filepath.Join(RegistryDir, "skills", "global", name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"),
			[]byte("---\nname: "+name+"\ndescription: skill "+name+"\n---\n# "+name), 0644); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	// Orphan: skill dir exists but no .sm-origin.json.
	writeSkill("orphan-skill")

	// Snapshot: valid local-snapshot origin.
	snapDir := writeSkill("snap-skill")
	snapOrigin := `{
  "schema_version": 1,
  "source_kind": "local-snapshot",
  "source": "` + snapDir + `"
}`
	if err := os.WriteFile(filepath.Join(snapDir, ".sm-origin.json"), []byte(snapOrigin), 0644); err != nil {
		t.Fatal(err)
	}

	results := checkRegistryIntegrity()

	var orphan *checkResult
	var meta *checkResult
	for i := range results {
		switch results[i].Name {
		case "Registry orphans":
			orphan = &results[i]
		case "Registry metadata":
			meta = &results[i]
		}
	}
	if orphan == nil {
		t.Fatal("expected Registry orphans check result")
	}
	if orphan.Status != "warn" {
		t.Errorf("orphan status = %s, want warn (message: %s)", orphan.Status, orphan.Message)
	}
	if !strings.Contains(orphan.Message, "orphan-skill") {
		t.Errorf("orphan message should mention orphan-skill, got: %s", orphan.Message)
	}
	if meta == nil {
		t.Fatal("expected Registry metadata check result")
	}
	if meta.Status != "pass" {
		t.Errorf("metadata status = %s, want pass (message: %s)", meta.Status, meta.Message)
	}
}

func TestCheckRegistryIntegrityCleanRegistryPasses(t *testing.T) {
	origRegistryDir := RegistryDir
	defer func() { RegistryDir = origRegistryDir }()

	tmpDir := t.TempDir()
	RegistryDir = filepath.Join(tmpDir, "registry")

	// One clean snapshot skill: no conflicts, no orphans, valid metadata.
	dir := filepath.Join(RegistryDir, "skills", "global", "clean-skill")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"),
		[]byte("---\nname: clean-skill\ndescription: a clean skill\n---\n# clean"), 0644); err != nil {
		t.Fatal(err)
	}
	origin := `{
  "schema_version": 1,
  "source_kind": "local-snapshot",
  "source": "` + dir + `"
}`
	if err := os.WriteFile(filepath.Join(dir, ".sm-origin.json"), []byte(origin), 0644); err != nil {
		t.Fatal(err)
	}

	results := checkRegistryIntegrity()
	for i := range results {
		r := results[i]
		if r.Name == "Registry conflicts" && r.Status != "pass" {
			t.Errorf("conflicts status = %s, want pass (message: %s)", r.Status, r.Message)
		}
		if r.Name == "Registry orphans" && r.Status != "pass" {
			t.Errorf("orphans status = %s, want pass (message: %s)", r.Status, r.Message)
		}
		if r.Name == "Registry metadata" && r.Status != "pass" {
			t.Errorf("metadata status = %s, want pass (message: %s)", r.Status, r.Message)
		}
	}
}

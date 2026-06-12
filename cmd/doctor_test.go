// cmd/doctor_test.go
package cmd

import (
	"os"
	"path/filepath"
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

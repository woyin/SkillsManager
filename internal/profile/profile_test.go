package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProfile(t *testing.T) {
	dir := t.TempDir()
	profileData := Profile{
		Skills: []string{"global", "cloudflare"},
		MCP:    []string{"cloudflare", "github"},
	}
	data, _ := json.Marshal(profileData)
	os.WriteFile(filepath.Join(dir, "cloudflare.json"), data, 0644)

	loader := NewLoader(dir)
	profile, err := loader.Load("cloudflare")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(profile.Skills) != 2 {
		t.Errorf("Expected 2 skills, got %d", len(profile.Skills))
	}
	if len(profile.MCP) != 2 {
		t.Errorf("Expected 2 MCP, got %d", len(profile.MCP))
	}
}

func TestLoadNonExistentProfile(t *testing.T) {
	dir := t.TempDir()
	loader := NewLoader(dir)
	_, err := loader.Load("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent profile")
	}
}

func TestListProfiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "b.json"), []byte("{}"), 0644)

	loader := NewLoader(dir)
	names, err := loader.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("Expected 2 profiles, got %d", len(names))
	}
}

func TestSaveProfile(t *testing.T) {
	dir := t.TempDir()
	loader := NewLoader(dir)

	p := &Profile{
		Skills: []string{"global", "cloudflare"},
		MCP:    []string{"cloudflare-mcp"},
	}

	if err := loader.Save("test-profile", p); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file was created
	loaded, err := loader.Load("test-profile")
	if err != nil {
		t.Fatalf("Load after Save failed: %v", err)
	}
	if len(loaded.Skills) != 2 {
		t.Errorf("Expected 2 skills, got %d", len(loaded.Skills))
	}
	if len(loaded.MCP) != 1 {
		t.Errorf("Expected 1 MCP, got %d", len(loaded.MCP))
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested", "profiles")
	loader := NewLoader(nested)

	p := &Profile{Skills: []string{"s1"}}
	if err := loader.Save("test", p); err != nil {
		t.Fatalf("Save should create directory: %v", err)
	}

	if _, err := os.Stat(filepath.Join(nested, "test.json")); err != nil {
		t.Errorf("Profile file should exist: %v", err)
	}
}

func TestDeleteProfile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "to-delete.json"), []byte(`{"skills":[]}`), 0644)

	loader := NewLoader(dir)
	if err := loader.Delete("to-delete"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	if _, err := os.Stat(filepath.Join(dir, "to-delete.json")); !os.IsNotExist(err) {
		t.Error("Profile file should be deleted")
	}
}

func TestDeleteNonExistentProfile(t *testing.T) {
	dir := t.TempDir()
	loader := NewLoader(dir)

	err := loader.Delete("nonexistent")
	if err == nil {
		t.Error("Expected error deleting non-existent profile")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{invalid json`), 0644)

	loader := NewLoader(dir)
	_, err := loader.Load("bad")
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestListProfilesEmpty(t *testing.T) {
	dir := t.TempDir()
	loader := NewLoader(dir)

	names, err := loader.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("Expected 0 profiles, got %d", len(names))
	}
}

func TestListProfilesIgnoresNonJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "valid.json"), []byte(`{}`), 0644)
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)

	loader := NewLoader(dir)
	names, err := loader.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(names) != 1 {
		t.Errorf("Expected 1 profile, got %d", len(names))
	}
}

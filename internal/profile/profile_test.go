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

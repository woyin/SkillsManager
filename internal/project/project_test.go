package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	configData := `{"profile":"cloudflare","skills":["extra-skill"],"mcp":["extra-mcp"]}`
	os.WriteFile(filepath.Join(dir, ".sm.json"), []byte(configData), 0644)

	mgr := NewManager(dir)
	config, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if config.Profile != "cloudflare" {
		t.Errorf("Expected profile 'cloudflare', got '%s'", config.Profile)
	}
	if len(config.Skills) != 1 || config.Skills[0] != "extra-skill" {
		t.Errorf("Expected [extra-skill], got %v", config.Skills)
	}
}

func TestWriteProjectConfig(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)

	config := &Config{
		Profile: "frontend",
		Skills:  []string{"design"},
		MCP:     []string{"github"},
	}
	err := mgr.Save(config)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load after save failed: %v", err)
	}
	if loaded.Profile != "frontend" {
		t.Errorf("Expected profile 'frontend', got '%s'", loaded.Profile)
	}
}

func TestNoConfigReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)

	config, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if config.Profile != "" {
		t.Errorf("Expected empty profile, got '%s'", config.Profile)
	}
}

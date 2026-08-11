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

func TestReadProjectConfigCuration(t *testing.T) {
	dir := t.TempDir()
	configData := `{"profile":"cloudflare","curation":{"baseline":{"profile":"cloudflare","skills":["extra-skill"],"mcp":["extra-mcp"]},"managed":{"claude":["foo"]}}}`
	os.WriteFile(filepath.Join(dir, ".sm.json"), []byte(configData), 0644)

	mgr := NewManager(dir)
	config, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if config.Curation == nil {
		t.Fatal("expected curation to be present")
	}
	if config.Curation.Baseline == nil || config.Curation.Baseline.Profile != "cloudflare" {
		t.Errorf("expected baseline profile 'cloudflare', got %+v", config.Curation.Baseline)
	}
	if len(config.Curation.Baseline.Skills) != 1 || config.Curation.Baseline.Skills[0] != "extra-skill" {
		t.Errorf("expected baseline skills, got %v", config.Curation.Baseline.Skills)
	}
	if managed := config.Curation.Managed["claude"]; len(managed) != 1 || managed[0] != "foo" {
		t.Errorf("expected managed claude=[foo], got %v", managed)
	}
}

func TestReadProjectConfigNoCuration(t *testing.T) {
	dir := t.TempDir()
	configData := `{"profile":"cloudflare"}`
	os.WriteFile(filepath.Join(dir, ".sm.json"), []byte(configData), 0644)

	mgr := NewManager(dir)
	config, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if config.Curation != nil {
		t.Errorf("expected nil curation for legacy config, got %+v", config.Curation)
	}
}

func TestSaveProjectConfigWithCuration(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)

	config := &Config{
		Profile: "frontend",
		Skills:  []string{"design"},
		MCP:     []string{"github"},
		Curation: &Curation{
			Baseline: &Baseline{Profile: "frontend", Skills: []string{"design"}, MCP: []string{"github"}},
			Managed:  map[string][]string{"codex": {"format-rules"}},
		},
	}
	err := mgr.Save(config)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load after save failed: %v", err)
	}
	if loaded.Curation == nil {
		t.Fatal("expected curation after save")
	}
	if loaded.Curation.Baseline == nil || loaded.Curation.Baseline.Profile != "frontend" {
		t.Errorf("unexpected baseline: %+v", loaded.Curation.Baseline)
	}
	if managed := loaded.Curation.Managed["codex"]; len(managed) != 1 || managed[0] != "format-rules" {
		t.Errorf("unexpected managed: %v", loaded.Curation.Managed)
	}
}

func TestCurationIsOwnedByAgent(t *testing.T) {
	tests := []struct {
		name    string
		managed map[string][]string
		agent   string
		skill   string
		want    bool
	}{
		{"owned", map[string][]string{"claude": {"foo"}}, "claude", "foo", true},
		{"wrong agent", map[string][]string{"claude": {"foo"}}, "codex", "foo", false},
		{"wrong skill", map[string][]string{"claude": {"foo"}}, "claude", "bar", false},
		{"nil", nil, "claude", "foo", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Curation{Managed: tt.managed}
			if got := c.IsOwned(tt.agent, tt.skill); got != tt.want {
				t.Errorf("IsOwned(%q,%q)=%v want %v", tt.agent, tt.skill, got, tt.want)
			}
		})
	}
}

func TestAddOwned(t *testing.T) {
	c := &Curation{Managed: map[string][]string{}}
	c.AddOwned("claude", "foo")
	c.AddOwned("claude", "foo") // idempotent
	c.AddOwned("codex", "bar")
	if got := c.Managed["claude"]; len(got) != 1 || got[0] != "foo" {
		t.Errorf("claude managed = %v, want [foo]", got)
	}
	if got := c.Managed["codex"]; len(got) != 1 || got[0] != "bar" {
		t.Errorf("codex managed = %v, want [bar]", got)
	}
}

func TestKeysAreManaged(t *testing.T) {
	c := &Curation{Managed: map[string][]string{"claude": {"foo"}, "codex": {"bar"}}}
	keys := c.Keys()
	if len(keys) != 2 {
		t.Fatalf("Keys() = %v, want 2 entries", keys)
	}
}

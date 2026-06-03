package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/registry"
)

func TestWriteRegistryListCanFilterSkillsOnly(t *testing.T) {
	reg := setupListRegistry(t)
	var out bytes.Buffer

	if err := writeRegistryList(&out, reg, true, false); err != nil {
		t.Fatalf("writeRegistryList failed: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "SKILLS:") {
		t.Fatalf("Expected skills section, got:\n%s", text)
	}
	if strings.Contains(text, "MCP:") {
		t.Fatalf("Did not expect MCP section, got:\n%s", text)
	}
	if !strings.Contains(text, "cloudflare") || !strings.Contains(text, "global") {
		t.Fatalf("Expected skill categories, got:\n%s", text)
	}
}

func TestWriteRegistryListCanFilterMCPOnly(t *testing.T) {
	reg := setupListRegistry(t)
	var out bytes.Buffer

	if err := writeRegistryList(&out, reg, false, true); err != nil {
		t.Fatalf("writeRegistryList failed: %v", err)
	}

	text := out.String()
	if strings.Contains(text, "SKILLS:") {
		t.Fatalf("Did not expect skills section, got:\n%s", text)
	}
	if !strings.Contains(text, "MCP:") {
		t.Fatalf("Expected MCP section, got:\n%s", text)
	}
	if !strings.Contains(text, "github") {
		t.Fatalf("Expected MCP entry, got:\n%s", text)
	}
}

func setupListRegistry(t *testing.T) *registry.Registry {
	t.Helper()

	dir := t.TempDir()
	for _, path := range []string{
		filepath.Join(dir, "skills", "global", "base-skill"),
		filepath.Join(dir, "skills", "cloudflare", "worker-skill"),
		filepath.Join(dir, "mcp"),
	} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatalf("creating registry path: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp", "github.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("writing mcp: %v", err)
	}

	return registry.New(dir)
}

// internal/prompt/prompt_test.go
package prompt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListPrompts(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	// Create some prompt sets
	ps1 := &PromptSet{Name: "default", Prompts: map[string]string{"CLAUDE.md": "# Default"}}
	ps2 := &PromptSet{Name: "cloudflare", Prompts: map[string]string{"CLAUDE.md": "# Cloudflare"}}

	if err := m.Save(ps1); err != nil {
		t.Fatalf("saving ps1: %v", err)
	}
	if err := m.Save(ps2); err != nil {
		t.Fatalf("saving ps2: %v", err)
	}

	names, err := m.List()
	if err != nil {
		t.Fatalf("listing: %v", err)
	}

	if len(names) != 2 {
		t.Errorf("expected 2 prompt sets, got %d", len(names))
	}
}

func TestLoadPrompt(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	ps := &PromptSet{
		Name:    "test",
		Prompts: map[string]string{"CLAUDE.md": "# Test", "AGENTS.md": "# Agents"},
	}
	if err := m.Save(ps); err != nil {
		t.Fatalf("saving: %v", err)
	}

	loaded, err := m.Load("test")
	if err != nil {
		t.Fatalf("loading: %v", err)
	}

	if loaded.Name != "test" {
		t.Errorf("expected name 'test', got %q", loaded.Name)
	}
	if loaded.Prompts["CLAUDE.md"] != "# Test" {
		t.Errorf("unexpected CLAUDE.md content: %q", loaded.Prompts["CLAUDE.md"])
	}
}

func TestLoadPromptNotFound(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	_, err := m.Load("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent prompt set")
	}
}

func TestDeletePrompt(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	ps := &PromptSet{Name: "test", Prompts: map[string]string{"CLAUDE.md": "# Test"}}
	if err := m.Save(ps); err != nil {
		t.Fatalf("saving: %v", err)
	}

	if err := m.Delete("test"); err != nil {
		t.Fatalf("deleting: %v", err)
	}

	names, err := m.List()
	if err != nil {
		t.Fatalf("listing: %v", err)
	}

	if len(names) != 0 {
		t.Errorf("expected 0 prompt sets after delete, got %d", len(names))
	}
}

func TestDeletePromptNotFound(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	err := m.Delete("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent prompt set")
	}
}

func TestApplyPrompt(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)

	ps := &PromptSet{
		Name: "test",
		Prompts: map[string]string{
			"CLAUDE.md": "# Claude",
			"AGENTS.md": "# Agents",
		},
	}
	if err := m.Save(ps); err != nil {
		t.Fatalf("saving: %v", err)
	}

	if err := m.Apply(projectDir, "test", nil); err != nil {
		t.Fatalf("applying: %v", err)
	}

	// Check files were created
	for _, filename := range []string{"CLAUDE.md", "AGENTS.md"} {
		path := filepath.Join(projectDir, filename)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("%s not created", filename)
		}
	}
}

func TestApplyPromptWithTools(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)

	ps := &PromptSet{
		Name: "test",
		Prompts: map[string]string{
			"CLAUDE.md": "# Claude",
			"AGENTS.md": "# Agents",
		},
	}
	if err := m.Save(ps); err != nil {
		t.Fatalf("saving: %v", err)
	}

	if err := m.Apply(projectDir, "test", []string{"CLAUDE.md"}); err != nil {
		t.Fatalf("applying: %v", err)
	}

	// Only CLAUDE.md should be created
	if _, err := os.Stat(filepath.Join(projectDir, "CLAUDE.md")); os.IsNotExist(err) {
		t.Error("CLAUDE.md not created")
	}
	if _, err := os.Stat(filepath.Join(projectDir, "AGENTS.md")); err == nil {
		t.Error("AGENTS.md should not be created")
	}
}

func TestCreateFromProject(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)

	// Create prompt files
	os.WriteFile(filepath.Join(projectDir, "CLAUDE.md"), []byte("# Claude"), 0644)
	os.WriteFile(filepath.Join(projectDir, "AGENTS.md"), []byte("# Agents"), 0644)

	if err := m.CreateFromProject(projectDir, "my-prompt", nil); err != nil {
		t.Fatalf("creating: %v", err)
	}

	ps, err := m.Load("my-prompt")
	if err != nil {
		t.Fatalf("loading: %v", err)
	}

	if ps.Prompts["CLAUDE.md"] != "# Claude" {
		t.Errorf("unexpected CLAUDE.md content: %q", ps.Prompts["CLAUDE.md"])
	}
	if ps.Prompts["AGENTS.md"] != "# Agents" {
		t.Errorf("unexpected AGENTS.md content: %q", ps.Prompts["AGENTS.md"])
	}
}

func TestCreateFromProjectNoFiles(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)

	err := m.CreateFromProject(projectDir, "my-prompt", nil)
	if err == nil {
		t.Error("expected error when no prompt files found")
	}
}

func TestSaveEmptyName(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	ps := &PromptSet{Name: "", Prompts: map[string]string{"CLAUDE.md": "# Test"}}
	err := m.Save(ps)
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestApplyPromptWithFuzzyMatch(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)

	ps := &PromptSet{
		Name: "test",
		Prompts: map[string]string{
			"CLAUDE.md": "# Claude",
		},
	}
	if err := m.Save(ps); err != nil {
		t.Fatalf("saving: %v", err)
	}

	// Apply with tool name without .md extension
	if err := m.Apply(projectDir, "test", []string{"CLAUDE"}); err != nil {
		t.Fatalf("applying with fuzzy match: %v", err)
	}

	if _, err := os.Stat(filepath.Join(projectDir, "CLAUDE.md")); os.IsNotExist(err) {
		t.Error("CLAUDE.md should be created via fuzzy match")
	}
}

func TestApplyPromptSkipsUnknown(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)

	ps := &PromptSet{
		Name: "test",
		Prompts: map[string]string{
			"CLAUDE.md": "# Claude",
		},
	}
	if err := m.Save(ps); err != nil {
		t.Fatalf("saving: %v", err)
	}

	// Apply with unknown tool — should skip silently
	if err := m.Apply(projectDir, "test", []string{"UNKNOWN.md"}); err != nil {
		t.Fatalf("applying with unknown tool should not error: %v", err)
	}
}

func TestApplyPromptNotFound(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)

	err := m.Apply(projectDir, "nonexistent", nil)
	if err == nil {
		t.Error("expected error for nonexistent prompt set")
	}
}

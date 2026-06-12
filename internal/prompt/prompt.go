// internal/prompt/prompt.go
package prompt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PromptSet represents a collection of prompt files for different tools
type PromptSet struct {
	Name    string            `json:"name"`
	Prompts map[string]string `json:"prompts"` // filename -> content
}

// Manager manages prompt sets
type Manager struct {
	dir string
}

// NewManager creates a new prompt manager
func NewManager(dir string) *Manager {
	return &Manager{dir: dir}
}

// List returns all available prompt set names
func (m *Manager) List() ([]string, error) {
	if _, err := os.Stat(m.dir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, fmt.Errorf("reading prompts directory: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			name := strings.TrimSuffix(entry.Name(), ".json")
			names = append(names, name)
		}
	}

	return names, nil
}

// Load loads a prompt set by name
func (m *Manager) Load(name string) (*PromptSet, error) {
	path := m.getPath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("prompt set %q not found", name)
		}
		return nil, fmt.Errorf("reading prompt set: %w", err)
	}

	var ps PromptSet
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, fmt.Errorf("parsing prompt set: %w", err)
	}

	return &ps, nil
}

// Save saves a prompt set
func (m *Manager) Save(ps *PromptSet) error {
	if ps.Name == "" {
		return fmt.Errorf("prompt set name cannot be empty")
	}

	if err := os.MkdirAll(m.dir, 0755); err != nil {
		return fmt.Errorf("creating prompts directory: %w", err)
	}

	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling prompt set: %w", err)
	}

	path := m.getPath(ps.Name)
	return os.WriteFile(path, data, 0644)
}

// Delete deletes a prompt set by name
func (m *Manager) Delete(name string) error {
	path := m.getPath(name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("prompt set %q not found", name)
	}
	return os.Remove(path)
}

// Apply applies a prompt set to a project directory
func (m *Manager) Apply(projectDir, name string, tools []string) error {
	ps, err := m.Load(name)
	if err != nil {
		return err
	}

	// If no tools specified, apply all
	if len(tools) == 0 {
		for filename := range ps.Prompts {
			tools = append(tools, filename)
		}
	}

	for _, tool := range tools {
		content, ok := ps.Prompts[tool]
		if !ok {
			// Try with .md extension
			found := false
			for filename, c := range ps.Prompts {
				if strings.TrimSuffix(filename, ".md") == tool {
					content = c
					tool = filename
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		path := filepath.Join(projectDir, tool)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", tool, err)
		}
	}

	return nil
}

// CreateFromProject creates a prompt set from existing prompt files in a project
func (m *Manager) CreateFromProject(projectDir, name string, filenames []string) error {
	if len(filenames) == 0 {
		filenames = []string{"CLAUDE.md", "AGENTS.md", "GEMINI.md"}
	}

	ps := &PromptSet{
		Name:    name,
		Prompts: make(map[string]string),
	}

	for _, filename := range filenames {
		path := filepath.Join(projectDir, filename)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("reading %s: %w", filename, err)
		}
		ps.Prompts[filename] = string(data)
	}

	if len(ps.Prompts) == 0 {
		return fmt.Errorf("no prompt files found in %s", projectDir)
	}

	return m.Save(ps)
}

// getPath returns the file path for a prompt set
func (m *Manager) getPath(name string) string {
	return filepath.Join(m.dir, name+".json")
}

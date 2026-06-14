package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ── MCP management ──

// AddMCP copies an MCP JSON file into the registry.
func (r *Registry) AddMCP(source string) error {
	name := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	if err := os.MkdirAll(r.mcpDir(), 0755); err != nil {
		return err
	}

	if IsGitURL(source) {
		destDir := filepath.Join(r.mcpDir(), name)
		if _, err := os.Stat(destDir); err == nil {
			return fmt.Errorf("MCP %q already exists in registry", name)
		}
		if err := CloneRepo(NormalizeGitURL(source), destDir); err != nil {
			return err
		}
		if _, err := findMCPDefinition(destDir); err != nil {
			os.RemoveAll(destDir)
			return err
		}
		return nil
	}

	dest := filepath.Join(r.mcpDir(), name+".json")

	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("MCP %q already exists in registry", name)
	}

	definitionPath := source
	if info, err := os.Stat(source); err == nil && info.IsDir() {
		definitionPath, err = findMCPDefinition(source)
		if err != nil {
			return err
		}
	}

	data, err := os.ReadFile(definitionPath)
	if err != nil {
		return fmt.Errorf("reading source: %w", err)
	}

	// Validate JSON
	if err := validateMCPDefinition(data); err != nil {
		return err
	}

	return os.WriteFile(dest, data, 0644)
}

func findMCPDefinition(dir string) (string, error) {
	for _, name := range []string{".mcp.json", "mcp.json"} {
		path := filepath.Join(dir, name)
		if data, err := os.ReadFile(path); err == nil && validateMCPDefinition(data) == nil {
			return path, nil
		}
	}

	var found string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if validateMCPDefinition(data) == nil {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("no MCP definition JSON found in %s", dir)
	}
	return found, nil
}

func validateMCPDefinition(data []byte) error {
	var test map[string]interface{}
	if err := json.Unmarshal(data, &test); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if _, ok := test["mcpServers"].(map[string]interface{}); !ok {
		return fmt.Errorf("invalid MCP definition: missing mcpServers")
	}
	return nil
}

// RemoveMCP removes an MCP definition from the registry.
func (r *Registry) RemoveMCP(name string) error {
	path := filepath.Join(r.mcpDir(), name+".json")
	if _, err := os.Stat(path); err == nil {
		return os.Remove(path)
	}
	dir := filepath.Join(r.mcpDir(), name)
	if _, err := os.Stat(dir); err == nil {
		return os.RemoveAll(dir)
	}
	return fmt.Errorf("MCP %q not found", name)
}

// ListMCP returns all MCP names in the registry.
func (r *Registry) ListMCP() ([]string, error) {
	entries, err := os.ReadDir(r.mcpDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name()[:len(e.Name())-5])
			continue
		}
		if e.IsDir() {
			if _, err := findMCPDefinition(filepath.Join(r.mcpDir(), e.Name())); err == nil {
				names = append(names, e.Name())
			}
		}
	}
	return names, nil
}

func (r *Registry) ListMCPDetails() ([]ItemDetail, error) {
	entries, err := os.ReadDir(r.mcpDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []ItemDetail{}, nil
		}
		return nil, err
	}

	items := make([]ItemDetail, 0)
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			name := entry.Name()[:len(entry.Name())-5]
			path := filepath.Join(r.mcpDir(), entry.Name())
			items = append(items, itemDetail(name, "", path))
			continue
		}
		if entry.IsDir() {
			dir := filepath.Join(r.mcpDir(), entry.Name())
			definitionPath, err := findMCPDefinition(dir)
			if err != nil {
				continue
			}
			detail := itemDetail(entry.Name(), "", dir)
			detail.Path = definitionPath
			items = append(items, detail)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items, nil
}

// GetMCPPath returns the absolute path to an MCP JSON file.
func (r *Registry) GetMCPPath(name string) string {
	path := filepath.Join(r.mcpDir(), name+".json")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	dir := filepath.Join(r.mcpDir(), name)
	if definitionPath, err := findMCPDefinition(dir); err == nil {
		return definitionPath
	}
	return path
}

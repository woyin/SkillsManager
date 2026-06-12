package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Profile struct {
	Skills []string `json:"skills"`
	MCP    []string `json:"mcp"`
}

// Config is an alias for Profile for backward compatibility
type Config = Profile

type Loader struct {
	dir string
}

func NewLoader(dir string) *Loader {
	return &Loader{dir: dir}
}

func (l *Loader) Load(name string) (*Profile, error) {
	path := filepath.Join(l.dir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("profile %q not found: %w", name, err)
	}

	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("invalid profile %q: %w", name, err)
	}
	return &p, nil
}

func (l *Loader) Save(name string, p *Profile) error {
	if err := os.MkdirAll(l.dir, 0755); err != nil {
		return fmt.Errorf("creating profiles directory: %w", err)
	}

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling profile: %w", err)
	}

	path := filepath.Join(l.dir, name+".json")
	return os.WriteFile(path, data, 0644)
}

func (l *Loader) List() ([]string, error) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name()[:len(e.Name())-5])
		}
	}
	return names, nil
}

func (l *Loader) Delete(name string) error {
	path := filepath.Join(l.dir, name+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("profile %q not found", name)
	}
	return os.Remove(path)
}

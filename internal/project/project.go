package project

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const configFileName = ".sm.json"

type Config struct {
	Profile string   `json:"profile,omitempty"`
	Skills  []string `json:"skills,omitempty"`
	MCP     []string `json:"mcp,omitempty"`
}

type Manager struct {
	dir string
}

func NewManager(dir string) *Manager {
	return &Manager{dir: dir}
}

func (m *Manager) configPath() string {
	return filepath.Join(m.dir, configFileName)
}

func (m *Manager) Load() (*Config, error) {
	data, err := os.ReadFile(m.configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (m *Manager) Save(config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.configPath(), data, 0644)
}

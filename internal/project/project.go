// Package project 读写项目目录下的 .sm.json 配置文件。
//
// .sm.json 描述某个项目的 sm 配置：基础 profile 以及在该 profile 之上
// 叠加的额外技能与 MCP。它是 `sm install` / `sm status` 的输入。
package project

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// configFileName 是项目配置文件名。
const configFileName = ".sm.json"

// Config 是 .sm.json 的磁盘表示：基础 profile + 额外技能/MCP。
type Config struct {
	Profile string   `json:"profile,omitempty"`
	Skills  []string `json:"skills,omitempty"`
	MCP     []string `json:"mcp,omitempty"`
}

// Manager 读写固定目录下的 .sm.json。
type Manager struct {
	dir string
}

// NewManager 返回操作 dir/.sm.json 的 Manager。
func NewManager(dir string) *Manager {
	return &Manager{dir: dir}
}

// configPath 返回 .sm.json 的绝对路径。
func (m *Manager) configPath() string {
	return filepath.Join(m.dir, configFileName)
}

// Load 读取 .sm.json。文件不存在时返回空 Config（不报错）。
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

// Save 把 config 序列化为缩进 JSON 写入 .sm.json。
func (m *Manager) Save(config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.configPath(), data, 0644)
}

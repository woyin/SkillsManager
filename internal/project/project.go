// Package project 读写项目目录下的 .sm.json 配置文件。
//
// .sm.json 描述某个项目的 sm 配置：基础 profile 以及在该 profile 之上
// 叠加的额外技能与 MCP。它是 `sm install` / `sm status` 的输入。
//
// Input: encoding/json, os, path/filepath
// Output: type Config, type Manager, func NewManager
// Pos: 业务层-项目配置.sm.json
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// configFileName 是项目配置文件名。
const configFileName = ".sm.json"

// Config 是 .sm.json 的磁盘表示：基础 profile + 额外技能/MCP。
// Curation 记录分层 Curation Baseline 的快照与计划拥有的 Link Install 集合。
type Config struct {
	Profile string   `json:"profile,omitempty"`
	Skills  []string `json:"skills,omitempty"`
	MCP     []string `json:"mcp,omitempty"`

	// Curation 记录 Curation 元数据（ADR 0021/0023/0028）。
	// 旧配置没有该字段时为 nil，表示尚无显式 Curation Baseline。
	Curation *Curation `json:"curation,omitempty"`
}

// Curation 是 .sm.json 中 curation 块的磁盘表示。
// Baseline 快照被确认计划所基于的显式组成（profile + 额外 skills/mcp）；
// Managed 记录计划拥有的项目级 Link Install（agent → skill 名），
// 是唯一允许计划移除的实体（ADR 0023）。
type Curation struct {
	Baseline *Baseline           `json:"baseline,omitempty"`
	Managed  map[string][]string `json:"managed,omitempty"`
}

// Baseline 快照显式 Curation Baseline。
// 与 Config 相同字段，表示"该显式目标"。
type Baseline struct {
	Profile string   `json:"profile,omitempty"`
	Skills  []string `json:"skills,omitempty"`
	MCP     []string `json:"mcp,omitempty"`
}

// IsOwned 报告 (agent, skill) 是否登记为由计划拥有的项目级 Link Install。
func (c *Curation) IsOwned(agent, skill string) bool {
	if c == nil {
		return false
	}
	for _, s := range c.Managed[agent] {
		if s == skill {
			return true
		}
	}
	return false
}

// AddOwned 把 (agent, skill) 登记为计划拥有（去重）。
func (c *Curation) AddOwned(agent, skill string) {
	if c.Managed == nil {
		c.Managed = map[string][]string{}
	}
	for _, s := range c.Managed[agent] {
		if s == skill {
			return
		}
	}
	c.Managed[agent] = append(c.Managed[agent], skill)
}

// Keys 返回 Managed 中出现的全部 agent 名。
func (c *Curation) Keys() []string {
	keys := make([]string, 0, len(c.Managed))
	for k := range c.Managed {
		keys = append(keys, k)
	}
	return keys
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

// ResolveProjectDir 解析"项目目录"：flagDir 非空时直接采用，否则回退到当前
// 工作目录（os.Getwd）。这是 install / list / status / uninstall / update 等
// 命令共用的样板逻辑，集中在此避免各处重复 Getwd + 错误包装。
//
// 返回的 error 已带 "getting working directory" 上下文，调用方可直接 return。
func ResolveProjectDir(flagDir string) (string, error) {
	if flagDir != "" {
		return flagDir, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	return wd, nil
}

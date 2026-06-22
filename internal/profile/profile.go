// Package profile 管理"技能 profile"：一组命名的 skills + MCP，
// 项目可引用某个 profile 作为基础配置。
//
// profile 以 JSON 文件形式存放在固定目录下，文件名即 profile 名。
package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Profile 是一组命名的 skills 与 MCP，作为项目配置的基础。
type Profile struct {
	Skills []string `json:"skills"`
	MCP    []string `json:"mcp"`
}

// Config 是 Profile 的别名，保留以向后兼容旧调用方。
type Config = Profile

// Loader 从固定目录读取 profile JSON 文件。
type Loader struct {
	dir string
}

// NewLoader 返回以 dir 为根的 Loader。
func NewLoader(dir string) *Loader {
	return &Loader{dir: dir}
}

// Load 读取名为 name 的 profile；文件不存在或 JSON 非法都返回错误。
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

// Save 把 p 序列化为缩进 JSON 写入 name.json（目录不存在则创建）。
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

// List 返回所有 profile 名（去 .json 后缀）。
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

// Delete 删除名为 name 的 profile；不存在则返回错误。
func (l *Loader) Delete(name string) error {
	path := filepath.Join(l.dir, name+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("profile %q not found", name)
	}
	return os.Remove(path)
}

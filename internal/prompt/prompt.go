// internal/prompt/prompt.go
// Package prompt 管理"prompt set"：一组面向不同 AI 编程助手的
// 提示词文件（CLAUDE.md、AGENTS.md、GEMINI.md 等）。
//
// prompt set 以 JSON 形式存放，可整体应用到某个项目目录，
// 也可从现有项目的提示词文件中创建。
//
// Input: encoding/json, fmt, os, path/filepath, strings
// Output: type PromptSet, type Manager, func NewManager
// Pos: 业务层-提示词管理
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package prompt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PromptSet 表示一组面向不同工具的提示词文件（文件名 → 内容）。
type PromptSet struct {
	Name    string            `json:"name"`
	Prompts map[string]string `json:"prompts"`
}

// Manager 管理某个目录下的全部 prompt set。
type Manager struct {
	dir string
}

// NewManager 构造一个以 dir 为根的 Manager。
func NewManager(dir string) *Manager {
	return &Manager{dir: dir}
}

// List 返回全部可用 prompt set 名（去 .json 后缀）。
// 目录不存在时返回 nil, nil。
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

// Load 按名读取一个 prompt set；不存在或 JSON 非法都返回错误。
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

// Save 持久化一个 prompt set（目录不存在则创建）。
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

// Delete 按名删除一个 prompt set；不存在则报错。
func (m *Manager) Delete(name string) error {
	path := m.getPath(name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("prompt set %q not found", name)
	}
	return os.Remove(path)
}

// Apply 把名为 name 的 prompt set 应用到 projectDir。
// tools 为空时应用其中全部文件；否则只应用指定工具对应的文件。
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

// CreateFromProject 从 projectDir 现有的提示词文件构建 prompt set。
// filenames 为空时默认读取 CLAUDE.md / AGENTS.md / GEMINI.md。
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

// getPath 返回某 prompt set 的磁盘路径。
func (m *Manager) getPath(name string) string {
	return filepath.Join(m.dir, name+".json")
}

// Package lockfile 管理 skills-lock.json —— 项目本地的技能锁文件。
//
// skills-lock.json 记录项目内已安装技能的来源信息（source、sourceType、
// skillPath、computedHash），使安装可复现：团队成员或 CI 可从锁文件
// 恢复完全一致的技能集（sm install --from-lock），对齐 npx skills 的
// experimental_install 与 skills-lock.json 约定。
//
// 文件位置：项目根目录下的 skills-lock.json（可提交到版本库）。
//
// Input: crypto/sha256, encoding/hex, encoding/json, fmt, io, os, path/filepath, sort
// Output: type LocalLock, type SkillEntry, type Manager, func NewManager, func ComputeHash
// Pos: 业务层-项目本地技能锁文件
package lockfile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// lockFileName 是项目本地锁文件的文件名。
const lockFileName = "skills-lock.json"

// currentVersion 是锁文件格式版本。
const currentVersion = 1

// SkillEntry 描述单个已安装技能的锁文件条目。
type SkillEntry struct {
	Source       string `json:"source"`
	SourceType   string `json:"sourceType"`
	SourceURL    string `json:"sourceUrl,omitempty"`
	SkillPath    string `json:"skillPath,omitempty"`
	Ref          string `json:"ref,omitempty"`
	ComputedHash string `json:"computedHash"`
	PluginName   string `json:"pluginName,omitempty"`
}

// LocalLock 是 skills-lock.json 的磁盘表示。
type LocalLock struct {
	Version int                    `json:"version"`
	Skills  map[string]*SkillEntry `json:"skills"`
}

// Manager 读写固定项目目录下的 skills-lock.json。
type Manager struct {
	dir string
}

// NewManager 返回操作 dir/skills-lock.json 的 Manager。
func NewManager(dir string) *Manager {
	return &Manager{dir: dir}
}

// Path 返回锁文件绝对路径。
func (m *Manager) Path() string {
	return filepath.Join(m.dir, lockFileName)
}

// Exists 报告锁文件是否存在。
func (m *Manager) Exists() bool {
	_, err := os.Stat(m.Path())
	return err == nil
}

// Load 读取 skills-lock.json。文件不存在时返回空锁（不报错）。
func (m *Manager) Load() (*LocalLock, error) {
	data, err := os.ReadFile(m.Path())
	if err != nil {
		if os.IsNotExist(err) {
			return &LocalLock{Version: currentVersion, Skills: make(map[string]*SkillEntry)}, nil
		}
		return nil, err
	}

	var lock LocalLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", lockFileName, err)
	}
	if lock.Version < currentVersion {
		return &LocalLock{Version: currentVersion, Skills: make(map[string]*SkillEntry)}, nil
	}
	if lock.Skills == nil {
		lock.Skills = make(map[string]*SkillEntry)
	}
	return &lock, nil
}

// Save 把锁序列化为缩进 JSON 写入 skills-lock.json。
func (m *Manager) Save(lock *LocalLock) error {
	if lock.Skills == nil {
		lock.Skills = make(map[string]*SkillEntry)
	}
	lock.Version = currentVersion

	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(m.Path(), data, 0644)
}

// Upsert 添加或更新一个技能条目。已存在则覆盖。
func (m *Manager) Upsert(name string, entry *SkillEntry) error {
	lock, err := m.Load()
	if err != nil {
		return err
	}
	lock.Skills[name] = entry
	return m.Save(lock)
}

// UpsertMany 批量添加/更新技能条目。
func (m *Manager) UpsertMany(entries map[string]*SkillEntry) error {
	lock, err := m.Load()
	if err != nil {
		return err
	}
	for name, entry := range entries {
		lock.Skills[name] = entry
	}
	return m.Save(lock)
}

// Remove 移除一个技能条目。不存在时不报错。
func (m *Manager) Remove(name string) error {
	lock, err := m.Load()
	if err != nil {
		return err
	}
	delete(lock.Skills, name)
	return m.Save(lock)
}

// RemoveMany 批量移除技能条目。
func (m *Manager) RemoveMany(names []string) error {
	lock, err := m.Load()
	if err != nil {
		return err
	}
	for _, name := range names {
		delete(lock.Skills, name)
	}
	return m.Save(lock)
}

// SortedNames 返回锁文件中所有技能名（按字典序）。
func (l *LocalLock) SortedNames() []string {
	names := make([]string, 0, len(l.Skills))
	for name := range l.Skills {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ComputeHash 计算目录下所有文件内容的 SHA-256，返回十六进制摘要。
// 跳过 .git 目录。按相对路径排序后逐文件哈希，保证内容稳定。
func ComputeHash(dir string) (string, error) {
	h := sha256.New()

	var paths []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walking %s: %w", dir, err)
	}

	sort.Strings(paths)

	for _, rel := range paths {
		relNorm := filepath.ToSlash(rel)
		h.Write([]byte(relNorm))
		h.Write([]byte{0})

		f, err := os.Open(filepath.Join(dir, rel))
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", rel, err)
		}
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return "", fmt.Errorf("hashing %s: %w", rel, err)
		}
		f.Close()
		h.Write([]byte{0})
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// Package registry 的 mcp.go 实现对 MCP（Model Context Protocol）服务
// 定义的增删查与路径解析。
//
// MCP 条目以两种形态存在于注册表：
//  1. 单文件： mcp/<name>.json —— 直接拷贝自本地源；
//  2. 目录： mcp/<name>/     —— 来自 git 克隆，内含 .mcp.json 或 mcp.json。
//
// Input: encoding/json, fmt, os, path/filepath, sort, strings
// Output: type ServerTransport, func AddMCP, func RemoveMCP, func ListMCP, func ListMCPDetails, func GetMCPPath, func MCPServerTransports
// Pos: 数据层-MCP定义
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AddMCP 把一个 MCP 定义拷贝进注册表。
// source 可以是：
//   - git URL：克隆到 mcp/<name>/，要求内含 MCP 定义 JSON；
//   - 本地 .json 文件：拷贝为 mcp/<name>.json；
//   - 本地目录：在其中查找 MCP 定义 JSON 后拷贝。
//
// 若同名 MCP 已存在，返回错误（调用方需先显式移除）。
func (r *Registry) AddMCP(source string) error {
	// 名称取源路径基名（去掉扩展名）。
	name := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	if err := os.MkdirAll(r.mcpDir(), 0755); err != nil {
		return err
	}

	// git 源：克隆到目录形态。
	if IsGitURL(source) {
		destDir := filepath.Join(r.mcpDir(), name)
		if _, err := os.Stat(destDir); err == nil {
			return fmt.Errorf("MCP %q already exists in registry", name)
		}
		if err := CloneRepoShallow(NormalizeGitURL(source), destDir); err != nil {
			return err
		}
		// 校验克隆结果中确实包含 MCP 定义。
		if _, err := findMCPDefinition(destDir); err != nil {
			os.RemoveAll(destDir)
			return err
		}
		return nil
	}

	// 本地源：统一拷贝成 mcp/<name>.json。
	dest := filepath.Join(r.mcpDir(), name+".json")
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("MCP %q already exists in registry", name)
	}

	definitionPath := source
	// 若源是目录，在其中查找 MCP 定义文件。
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

	// 写入前校验 JSON 形状：必须含 mcpServers 对象。
	if err := validateMCPDefinition(data); err != nil {
		return err
	}

	return os.WriteFile(dest, data, 0644)
}

// findMCPDefinition 在 dir 中查找 MCP 定义 JSON。
// 查找顺序：
//  1. .mcp.json / mcp.json（约定优先）；
//  2. 任意第一个通过校验的 *.json 文件。
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

// mcpFile 是 validateMCPDefinition 需要的最小结构：顶层对象含
// mcpServers 对象。
type mcpFile struct {
	MCPServers map[string]any `json:"mcpServers"`
}

// validateMCPDefinition 校验 data 是否为合法的 MCP 定义 JSON。
func validateMCPDefinition(data []byte) error {
	var m mcpFile
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if m.MCPServers == nil {
		return fmt.Errorf("invalid MCP definition: missing mcpServers")
	}
	return nil
}

// RemoveMCP 从注册表移除一个 MCP 定义（支持文件与目录两种形态）。
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

// ListMCP 返回注册表中所有 MCP 名称（已排序不保证；调用方可自行排序）。
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
		// 单文件形态：去掉 .json 后缀。
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name()[:len(e.Name())-5])
			continue
		}
		// 目录形态：内含 MCP 定义才算。
		if e.IsDir() {
			if _, err := findMCPDefinition(filepath.Join(r.mcpDir(), e.Name())); err == nil {
				names = append(names, e.Name())
			}
		}
	}
	return names, nil
}

// ListMCPDetails 返回带详情的 MCP 列表，按名称排序。
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
			// 目录形态下 Path 指向实际定义文件而非目录本身。
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

// GetMCPPath 返回 MCP 的定义文件路径（单文件或目录内查找）。
// 未找到时返回回退路径（不报错），调用方通常随后用 os.Stat 判断。
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

// ServerTransport 描述一个 MCP server 的 transport 摘要。
type ServerTransport struct {
	Server    string // server 名（mcpServers 的 key）
	Transport string // "stdio" | "http" | "sse" | "unknown"
	Detail    string // stdio: command；http/sse: url；unknown: ""
}

// mcpServerEntry 用 map[string]any 接住单个 server 条目，字段命名因 agent
// 而异（command/url/type/env...），不做强类型绑定以兼容变体。
type mcpServerEntry = map[string]any

// MCPServerTransports 解析一个 MCP 定义文件，返回其全部 server 的 transport 摘要。
// 返回顺序按 server 名排序（map 无序，确定性输出便于展示与测试）。
// 解析失败返回 error，调用方可降级展示。
func MCPServerTransports(defPath string) ([]ServerTransport, error) {
	data, err := os.ReadFile(defPath)
	if err != nil {
		return nil, err
	}
	var m mcpFile
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if m.MCPServers == nil {
		return nil, fmt.Errorf("missing mcpServers")
	}

	names := make([]string, 0, len(m.MCPServers))
	for name := range m.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)

	transports := make([]ServerTransport, 0, len(names))
	for _, name := range names {
		entry, _ := m.MCPServers[name].(mcpServerEntry)
		transports = append(transports, classifyTransport(name, entry))
	}
	return transports, nil
}

// classifyTransport 按字段存在性保守推断 transport 类型。
//   - command → stdio
//   - url + type=sse → sse
//   - url（无 type 或 type=http）→ http
//   - 其余 → unknown
func classifyTransport(name string, e mcpServerEntry) ServerTransport {
	st := ServerTransport{Server: name, Transport: "unknown"}
	if e == nil {
		return st
	}
	if cmd, ok := e["command"].(string); ok && cmd != "" {
		st.Transport = "stdio"
		st.Detail = cmd
		return st
	}
	url, hasURL := e["url"].(string)
	typ, _ := e["type"].(string)
	if hasURL && url != "" {
		switch strings.ToLower(typ) {
		case "sse":
			st.Transport = "sse"
		default: // "http" 或未声明都按 http（2025 规范的默认 streamable HTTP）
			st.Transport = "http"
		}
		st.Detail = url
	}
	return st
}

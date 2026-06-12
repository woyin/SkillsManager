// internal/registry/registry.go
package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Special directories that have fixed install targets
const (
	Global       = "global"
	CodexOnly    = "codex-only"
	ClaudeOnly   = "claude-only"
	GeminiOnly   = "gemini-only"
	OpenCodeOnly = "opencode-only"
	HermesOnly   = "hermes-only"
	OpenClawOnly = "openclaw-only"
)

var specialDirs = map[string]bool{
	Global:       true,
	CodexOnly:    true,
	ClaudeOnly:   true,
	GeminiOnly:   true,
	OpenCodeOnly: true,
	HermesOnly:   true,
	OpenClawOnly: true,
}

type Registry struct {
	dir string
}

type ItemDetail struct {
	Name        string `json:"name"`
	Category    string `json:"category,omitempty"`
	Path        string `json:"path"`
	SourceURL   string `json:"source_url,omitempty"`
	LastUpdated string `json:"last_updated"`
}

func New(dir string) *Registry {
	return &Registry{dir: dir}
}

func (r *Registry) skillsDir() string {
	return filepath.Join(r.dir, "skills")
}

func (r *Registry) mcpDir() string {
	return filepath.Join(r.dir, "mcp")
}

// IsSpecialDir returns true if the category is a special directory.
func IsSpecialDir(category string) bool {
	return specialDirs[category]
}

// SkillNameFromPath extracts the name from a path or URL.
func SkillNameFromPath(source string) string {
	// For GitHub URLs like github.com/user/repo/path/to/skill
	// take the last path segment
	source = strings.TrimRight(source, "/")
	parts := strings.Split(source, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return source
}

// IsGitURL returns true if source looks like a git URL.
func IsGitURL(source string) bool {
	return strings.HasPrefix(source, "github.com/") ||
		strings.HasPrefix(source, "https://github.com/") ||
		strings.HasPrefix(source, "git@") ||
		strings.HasSuffix(source, ".git")
}

// normalizeGitURL converts shorthand to full URL.
func normalizeGitURL(source string) string {
	if strings.HasPrefix(source, "github.com/") {
		return "https://" + source
	}
	return source
}

// AddSkill adds a skill to the registry. If special is non-empty, it overrides category.
// For GitHub URLs, it clones. For local paths, it copies.
func (r *Registry) AddSkill(source, category, special string) error {
	name := SkillNameFromPath(source)
	if name == "" {
		return fmt.Errorf("cannot determine skill name from source: %s", source)
	}

	var destCategory string
	if special != "" {
		destCategory = special
	} else if category != "" {
		destCategory = category
	} else {
		return fmt.Errorf("must specify category or --global/--codex/--claude")
	}

	dest := filepath.Join(r.skillsDir(), destCategory, name)

	if IsGitURL(source) {
		return r.cloneRepo(normalizeGitURL(source), dest)
	}
	return r.copyDir(source, dest)
}

func (r *Registry) cloneRepo(url, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("destination already exists: %s", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	cmd := exec.Command("git", "clone", url, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (r *Registry) copyDir(src, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("destination already exists: %s", dest)
	}
	return copyDirRecursive(src, dest)
}

func copyDirRecursive(src, dest string) error {
	srcInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}

	// Handle symlinks: copy the symlink target contents (follow the link)
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return fmt.Errorf("reading symlink %s: %w", src, err)
		}
		// Resolve relative symlinks
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(src), target)
		}
		return copyDirRecursive(target, dest)
	}

	if err := os.MkdirAll(dest, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		// Check if entry is a symlink
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// Follow symlink: resolve and copy the target
			target, err := os.Readlink(srcPath)
			if err != nil {
				return fmt.Errorf("reading symlink %s: %w", srcPath, err)
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(srcPath), target)
			}
			targetInfo, err := os.Stat(target)
			if err != nil {
				return fmt.Errorf("stat symlink target %s: %w", target, err)
			}
			if targetInfo.IsDir() {
				if err := copyDirRecursive(target, destPath); err != nil {
					return err
				}
			} else {
				if err := copyFile(target, destPath); err != nil {
					return err
				}
			}
		} else if entry.IsDir() {
			if err := copyDirRecursive(srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, destPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	srcInfo, _ := os.Stat(src)
	return os.Chmod(dest, srcInfo.Mode())
}

// RemoveSkill removes a skill from the registry.
func (r *Registry) RemoveSkill(name, category, special string) error {
	var dir string
	if special != "" {
		dir = filepath.Join(r.skillsDir(), special, name)
	} else if category != "" {
		dir = filepath.Join(r.skillsDir(), category, name)
	} else {
		// Search all categories
		found, err := r.findSkillDir(name)
		if err != nil {
			return err
		}
		if found == "" {
			return fmt.Errorf("skill %q not found in registry", name)
		}
		dir = found
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("skill not found: %s", dir)
	}
	return os.RemoveAll(dir)
}

func (r *Registry) findSkillDir(name string) (string, error) {
	skillsDir := r.skillsDir()
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return "", err
	}
	for _, cat := range entries {
		if !cat.IsDir() {
			continue
		}
		candidate := filepath.Join(skillsDir, cat.Name(), name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", nil
}

// ListSkills returns all skills grouped by category.
func (r *Registry) ListSkills() (map[string][]string, error) {
	result := make(map[string][]string)
	skillsDir := r.skillsDir()

	categories, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}

	for _, cat := range categories {
		if !cat.IsDir() {
			continue
		}
		skills, err := os.ReadDir(filepath.Join(skillsDir, cat.Name()))
		if err != nil {
			continue
		}
		for _, s := range skills {
			if s.IsDir() && s.Name() != ".gitkeep" {
				result[cat.Name()] = append(result[cat.Name()], s.Name())
			}
		}
	}
	return result, nil
}

func (r *Registry) ListSkillDetails() (map[string][]ItemDetail, error) {
	result := make(map[string][]ItemDetail)
	skillsDir := r.skillsDir()

	categories, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}

	for _, cat := range categories {
		if !cat.IsDir() {
			continue
		}
		categoryDir := filepath.Join(skillsDir, cat.Name())
		skills, err := os.ReadDir(categoryDir)
		if err != nil {
			continue
		}
		for _, skill := range skills {
			if !skill.IsDir() || skill.Name() == ".gitkeep" {
				continue
			}
			path := filepath.Join(categoryDir, skill.Name())
			result[cat.Name()] = append(result[cat.Name()], itemDetail(skill.Name(), cat.Name(), path))
		}
		sort.Slice(result[cat.Name()], func(i, j int) bool {
			return result[cat.Name()][i].Name < result[cat.Name()][j].Name
		})
	}

	return result, nil
}

// GetSkillPath returns the absolute path to a skill in the registry.
func (r *Registry) GetSkillPath(name, category, special string) (string, error) {
	if special != "" {
		path := filepath.Join(r.skillsDir(), special, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("skill %q not found in %s", name, special)
	}
	if category != "" {
		path := filepath.Join(r.skillsDir(), category, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("skill %q not found in %s", name, category)
	}
	// Search all
	found, err := r.findSkillDir(name)
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("skill %q not found in registry", name)
	}
	return found, nil
}

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
		if err := r.cloneRepo(normalizeGitURL(source), destDir); err != nil {
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

// Dir returns the registry root directory.
func (r *Registry) Dir() string {
	return r.dir
}

func itemDetail(name, category, path string) ItemDetail {
	info, _ := os.Stat(path)
	lastUpdated := ""
	if info != nil {
		lastUpdated = info.ModTime().UTC().Format("2006-01-02T15:04:05Z")
	}
	return ItemDetail{
		Name:        name,
		Category:    category,
		Path:        path,
		SourceURL:   gitRemoteURL(path),
		LastUpdated: lastUpdated,
	}
}

func gitRemoteURL(path string) string {
	configPath := filepath.Join(path, ".git", "config")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	inOrigin := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inOrigin = trimmed == `[remote "origin"]`
			continue
		}
		if inOrigin && strings.HasPrefix(trimmed, "url") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// internal/registry/registry.go
package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Special directories that have fixed install targets
const (
	Global     = "global"
	CodexOnly  = "codex-only"
	ClaudeOnly = "claude-only"
)

var specialDirs = map[string]bool{
	Global:     true,
	CodexOnly:  true,
	ClaudeOnly: true,
}

type Registry struct {
	dir string
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
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
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

		if entry.IsDir() {
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
	dest := filepath.Join(r.mcpDir(), name+".json")

	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("MCP %q already exists in registry", name)
	}

	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("reading source: %w", err)
	}

	// Validate JSON
	var test map[string]interface{}
	if err := json.Unmarshal(data, &test); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	return os.WriteFile(dest, data, 0644)
}

// RemoveMCP removes an MCP definition from the registry.
func (r *Registry) RemoveMCP(name string) error {
	path := filepath.Join(r.mcpDir(), name+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("MCP %q not found", name)
	}
	return os.Remove(path)
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
		}
	}
	return names, nil
}

// GetMCPPath returns the absolute path to an MCP JSON file.
func (r *Registry) GetMCPPath(name string) string {
	return filepath.Join(r.mcpDir(), name+".json")
}

// Dir returns the registry root directory.
func (r *Registry) Dir() string {
	return r.dir
}
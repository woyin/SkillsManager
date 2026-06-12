// internal/registry/registry.go
package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
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

// DiscoveredSkill represents a skill found in a repository
type DiscoveredSkill struct {
	Name        string
	Description string
	Path        string // Path to the skill directory
	SkillMDPath string // Path to the SKILL.md file
}

// ── Plugin manifest types ──

// pluginMarketplace represents a .claude-plugin/marketplace.json file.
type pluginMarketplace struct {
	Metadata pluginMetadata   `json:"metadata"`
	Plugins  []pluginManifest `json:"plugins"`
}

// pluginMetadata holds marketplace-level metadata.
type pluginMetadata struct {
	PluginRoot string `json:"pluginRoot"`
}

// pluginManifest represents a single plugin definition.
type pluginManifest struct {
	Name       string   `json:"name"`
	Source     string   `json:"source"`
	Skills     []string `json:"skills"`
	PluginRoot string   `json:"pluginRoot,omitempty"`
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

// ── Source parsing and normalization ──

// SkillNameFromPath extracts the name from a path or URL.
// For tree URLs, extracts the last path component (the skill name).
func SkillNameFromPath(source string) string {
	source = strings.TrimRight(source, "/")
	// For tree URLs, the skill name is the last segment
	if idx := strings.Index(source, "/tree/"); idx >= 0 {
		treePath := source[idx+6:] // after "/tree/"
		parts := strings.Split(treePath, "/")
		if len(parts) > 0 {
			name := parts[len(parts)-1]
			name = strings.TrimSuffix(name, ".git")
			if name != "" {
				return name
			}
		}
		// Fallback: use the repo name
		source = source[:idx]
	}
	parts := strings.Split(source, "/")
	if len(parts) > 0 {
		name := parts[len(parts)-1]
		// Strip .git suffix
		name = strings.TrimSuffix(name, ".git")
		return name
	}
	return source
}

// IsGitURL returns true if source looks like a git URL.
// Supports: GitHub shorthand (owner/repo), full URLs, GitLab, SSH, and generic git URLs.
func IsGitURL(source string) bool {
	// SSH URLs
	if strings.HasPrefix(source, "git@") {
		return true
	}
	// .git suffix
	if strings.HasSuffix(source, ".git") {
		return true
	}
	// HTTPS URLs to known hosts
	for _, prefix := range []string{
		"https://github.com/",
		"https://gitlab.com/",
		"https://bitbucket.org/",
		"http://github.com/",
		"http://gitlab.com/",
		"http://bitbucket.org/",
	} {
		if strings.HasPrefix(source, prefix) {
			return true
		}
	}
	// GitHub shorthand: owner/repo (but not local paths)
	if isGitHubShorthand(source) {
		return true
	}
	return false
}

// isGitHubShorthand checks if source is in owner/repo format
func isGitHubShorthand(source string) bool {
	// Must not start with . or / (local paths)
	if strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") {
		return false
	}
	// Must not contain : (SSH URLs handled separately)
	if strings.Contains(source, ":") {
		return false
	}
	// Must have exactly one / and look like owner/repo
	parts := strings.Split(source, "/")
	if len(parts) < 2 || len(parts) > 4 { // owner/repo or owner/repo/tree/path
		return false
	}
	// Owner and repo should not be empty
	for _, p := range parts[:2] {
		if p == "" {
			return false
		}
	}
	return true
}

// normalizeGitURL converts shorthand to full URL.
func normalizeGitURL(source string) string {
	if strings.HasPrefix(source, "github.com/") {
		return "https://" + source
	}
	if isGitHubShorthand(source) {
		// owner/repo → https://github.com/owner/repo
		// Strip any /tree/ path first
		base := source
		if idx := strings.Index(source, "/tree/"); idx >= 0 {
			base = source[:idx]
		}
		return "https://github.com/" + base
	}
	return source
}

// ParseTreeURL extracts repo URL and sub-path from a tree URL.
// Example: https://github.com/owner/repo/tree/main/skills/my-skill
// Returns: (https://github.com/owner/repo, main, skills/my-skill)
func ParseTreeURL(source string) (repoURL, branch, subPath string, ok bool) {
	// Handle GitHub shorthand with tree: owner/repo/tree/branch/path
	if !strings.Contains(source, "://") && !strings.HasPrefix(source, "git@") {
		if strings.Contains(source, "/tree/") {
			// Looks like owner/repo/tree/... shorthand
			parts := strings.SplitN(source, "/", 3)
			if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
				source = "https://github.com/" + source
			}
		}
	}

	for _, host := range []string{"https://github.com/", "https://gitlab.com/", "https://bitbucket.org/"} {
		if strings.HasPrefix(source, host) {
			rest := source[len(host):]
			parts := strings.SplitN(rest, "/", 3) // owner, repo, tree/...
			if len(parts) < 2 {
				return "", "", "", false
			}
			repoURL = host + parts[0] + "/" + parts[1]
			// Strip .git from repo name
			repoURL = strings.TrimSuffix(repoURL, ".git")

			if len(parts) < 3 {
				return repoURL, "", "", true
			}

			treePath := parts[2]
			if !strings.HasPrefix(treePath, "tree/") {
				return repoURL, "", "", true
			}

			treePath = treePath[5:] // strip "tree/"
			branchAndPath := strings.SplitN(treePath, "/", 2)
			branch = branchAndPath[0]
			if len(branchAndPath) > 1 {
				subPath = branchAndPath[1]
			}
			return repoURL, branch, subPath, true
		}
	}
	return "", "", "", false
}

// DiscoverSkills finds all SKILL.md files in a directory and returns discovered skills.
func DiscoverSkills(dir string) ([]DiscoveredSkill, error) {
	var skills []DiscoveredSkill

	// Standard skill discovery paths (from vercel-labs/skills)
	containerDirs := []string{
		".",
		"skills",
		"skills/.curated",
		"skills/.experimental",
		"skills/.system",
		".agents/skills",
		".claude/skills",
		".codex/skills",
		".gemini/skills",
	}

	seen := make(map[string]bool)

	for _, container := range containerDirs {
		containerPath := filepath.Join(dir, container)
		if _, err := os.Stat(containerPath); err != nil {
			continue
		}

		// Walk one level deep for flat layout: container/name/SKILL.md
		entries, err := os.ReadDir(containerPath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == ".git" || entry.Name() == "node_modules" {
				continue
			}

			skillDir := filepath.Join(containerPath, entry.Name())
			skillMD := filepath.Join(skillDir, "SKILL.md")
			if _, err := os.Stat(skillMD); err == nil {
				name := entry.Name()
				if !seen[name] {
					seen[name] = true
					desc := parseSkillDescription(skillMD)
					skills = append(skills, DiscoveredSkill{
						Name:        name,
						Description: desc,
						Path:        skillDir,
						SkillMDPath: skillMD,
					})
				}
			}

			// Also walk one more level for catalog layout: container/category/name/SKILL.md
			subEntries, err := os.ReadDir(skillDir)
			if err != nil {
				continue
			}
			for _, subEntry := range subEntries {
				if !subEntry.IsDir() {
					continue
				}
				subSkillDir := filepath.Join(skillDir, subEntry.Name())
				subSkillMD := filepath.Join(subSkillDir, "SKILL.md")
				if _, err := os.Stat(subSkillMD); err == nil {
					name := subEntry.Name()
					if !seen[name] {
						seen[name] = true
						desc := parseSkillDescription(subSkillMD)
						skills = append(skills, DiscoveredSkill{
							Name:        name,
							Description: desc,
							Path:        subSkillDir,
							SkillMDPath: subSkillMD,
						})
					}
				}
			}
		}
	}

	// ── Plugin manifest discovery ──
	// Check for .claude-plugin/marketplace.json and .claude-plugin/plugin.json
	// (and equivalent .codex-plugin, .agents-plugin directories)
	for _, pluginDirName := range []string{".claude-plugin", ".codex-plugin", ".agents-plugin", ".gemini-plugin"} {
		pluginDir := filepath.Join(dir, pluginDirName)

		// Try marketplace.json (multi-plugin manifest)
		marketplacePath := filepath.Join(pluginDir, "marketplace.json")
		if data, err := os.ReadFile(marketplacePath); err == nil {
			var marketplace pluginMarketplace
			if err := json.Unmarshal(data, &marketplace); err == nil {
				pluginRoot := dir
				if marketplace.Metadata.PluginRoot != "" {
					resolved := marketplace.Metadata.PluginRoot
					if !filepath.IsAbs(resolved) {
						resolved = filepath.Join(dir, resolved)
					}
					pluginRoot = resolved
				}
				for _, plugin := range marketplace.Plugins {
					for _, skillRelPath := range plugin.Skills {
						skillPath := skillRelPath
						if !filepath.IsAbs(skillPath) {
							skillPath = filepath.Join(pluginRoot, skillPath)
						}
						skillMD := filepath.Join(skillPath, "SKILL.md")
						if _, err := os.Stat(skillMD); err == nil {
							name := filepath.Base(skillPath)
							if !seen[name] {
								seen[name] = true
								desc := parseSkillDescription(skillMD)
								skills = append(skills, DiscoveredSkill{
									Name:        name,
									Description: desc,
									Path:        skillPath,
									SkillMDPath: skillMD,
								})
							}
						}
					}
				}
			}
		}

		// Try plugin.json (single-plugin manifest)
		pluginJSONPath := filepath.Join(pluginDir, "plugin.json")
		if data, err := os.ReadFile(pluginJSONPath); err == nil {
			var plugin pluginManifest
			if err := json.Unmarshal(data, &plugin); err == nil {
				pluginRoot := dir
				if plugin.PluginRoot != "" {
					resolved := plugin.PluginRoot
					if !filepath.IsAbs(resolved) {
						resolved = filepath.Join(dir, resolved)
					}
					pluginRoot = resolved
				}
				for _, skillRelPath := range plugin.Skills {
					skillPath := skillRelPath
					if !filepath.IsAbs(skillPath) {
						skillPath = filepath.Join(pluginRoot, skillPath)
					}
					skillMD := filepath.Join(skillPath, "SKILL.md")
					if _, err := os.Stat(skillMD); err == nil {
						name := filepath.Base(skillPath)
						if !seen[name] {
							seen[name] = true
							desc := parseSkillDescription(skillMD)
							skills = append(skills, DiscoveredSkill{
								Name:        name,
								Description: desc,
								Path:        skillPath,
								SkillMDPath: skillMD,
							})
						}
					}
				}
			}
		}
	}

	// If nothing found in standard locations, check root for SKILL.md
	rootMD := filepath.Join(dir, "SKILL.md")
	if _, err := os.Stat(rootMD); err == nil && !seen["."] {
		name := filepath.Base(dir)
		desc := parseSkillDescription(rootMD)
		skills = append(skills, DiscoveredSkill{
			Name:        name,
			Description: desc,
			Path:        dir,
			SkillMDPath: rootMD,
		})
	}

	return skills, nil
}

// parseSkillDescription reads the YAML frontmatter of a SKILL.md file and extracts the description.
func parseSkillDescription(skillMDPath string) string {
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		return ""
	}

	content := string(data)
	// Look for YAML frontmatter between --- markers
	if !strings.HasPrefix(content, "---") {
		return ""
	}
	rest := content[3:]
	endIdx := strings.Index(rest, "---")
	if endIdx < 0 {
		return ""
	}
	frontmatter := rest[:endIdx]

	// Simple YAML parsing for description field
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "description:") {
			desc := strings.TrimPrefix(line, "description:")
			desc = strings.TrimSpace(desc)
			// Remove surrounding quotes if present
			if len(desc) >= 2 && (desc[0] == '"' || desc[0] == '\'') && desc[len(desc)-1] == desc[0] {
				desc = desc[1 : len(desc)-1]
			}
			return desc
		}
	}
	return ""
}

// ── Skill management ──

// AddSkill adds a skill to the registry. If special is non-empty, it overrides category.
// For GitHub URLs, it clones. For local paths, it copies.
// If skillNames is non-empty, only those skills are extracted from the cloned repo.
func (r *Registry) AddSkill(source, category, special string) error {
	return r.AddSkillWithOptions(source, category, special, nil, false)
}

// AddSkillWithOptions adds a skill with extended options.
// skillNames: if non-empty, only extract these skills from the source.
// copyMode: if true, copy files instead of keeping the git clone.
func (r *Registry) AddSkillWithOptions(source, category, special string, skillNames []string, copyMode bool) error {
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
		repoURL, branch, subPath, isTree := ParseTreeURL(source)
		if !isTree {
			repoURL = normalizeGitURL(source)
		}

		if subPath != "" || len(skillNames) > 0 {
			// Clone to temp, extract specific skills
			return r.cloneAndExtract(repoURL, branch, subPath, dest, destCategory, skillNames, copyMode)
		}

		if copyMode {
			return r.cloneAndCopy(repoURL, branch, dest)
		}
		return r.cloneRepoWithBranch(repoURL, branch, dest)
	}
	return r.copyDir(source, dest)
}

// cloneAndExtract clones a repo to a temp dir, then copies specific skills or a sub-path.
func (r *Registry) cloneAndExtract(repoURL, branch, subPath, dest, category string, skillNames []string, copyMode bool) error {
	tmpDir, err := os.MkdirTemp("", "sm-clone-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cloneDest := filepath.Join(tmpDir, "repo")
	if err := r.cloneRepoWithBranch(repoURL, branch, cloneDest); err != nil {
		return fmt.Errorf("cloning %s: %w", repoURL, err)
	}

	if subPath != "" {
		// Copy specific sub-path
		src := filepath.Join(cloneDest, subPath)
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("path %q not found in repository", subPath)
		}
		return r.copyDir(src, dest)
	}

	if len(skillNames) > 0 {
		// Discover skills and copy only requested ones
		discovered, err := DiscoverSkills(cloneDest)
		if err != nil {
			return fmt.Errorf("discovering skills: %w", err)
		}

		discoveredMap := make(map[string]DiscoveredSkill)
		for _, s := range discovered {
			discoveredMap[s.Name] = s
		}

		for _, name := range skillNames {
			if name == "*" {
				// Copy all discovered skills
				for _, s := range discovered {
					skillDest := filepath.Join(r.skillsDir(), category, s.Name)
					if err := r.copyDir(s.Path, skillDest); err != nil {
						fmt.Fprintf(os.Stderr, "warning: skipping skill %q: %v\n", s.Name, err)
						continue
					}
				}
				return nil
			}
			s, ok := discoveredMap[name]
			if !ok {
				return fmt.Errorf("skill %q not found in repository", name)
			}
			skillDest := filepath.Join(r.skillsDir(), category, s.Name)
			if err := r.copyDir(s.Path, skillDest); err != nil {
				return fmt.Errorf("copying skill %q: %w", name, err)
			}
		}
		return nil
	}

	return nil
}

// cloneAndCopy clones a repo and copies the result (no .git directory).
func (r *Registry) cloneAndCopy(repoURL, branch, dest string) error {
	tmpDir, err := os.MkdirTemp("", "sm-clone-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cloneDest := filepath.Join(tmpDir, "repo")
	if err := r.cloneRepoWithBranch(repoURL, branch, cloneDest); err != nil {
		return fmt.Errorf("cloning %s: %w", repoURL, err)
	}

	// Remove .git directory before copying
	os.RemoveAll(filepath.Join(cloneDest, ".git"))
	return r.copyDir(cloneDest, dest)
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

func (r *Registry) cloneRepoWithBranch(url, branch, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("destination already exists: %s", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	args := []string{"clone"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, "--depth", "1", url, dest)
	cmd := exec.Command("git", args...)
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

		// Skip .git directories
		if entry.IsDir() && entry.Name() == ".git" {
			continue
		}

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

// ── Skill removal ──

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

// ── Skill listing ──

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

// ── MCP management ──

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

// ── URL utilities (exported for tests) ──

// Ensure url package is used (for future URL parsing needs)
var _ = url.Parse

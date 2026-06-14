package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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
			repoURL = NormalizeGitURL(source)
		}

		if subPath != "" || len(skillNames) > 0 {
			// Clone to temp, extract specific skills
			return r.cloneAndExtract(repoURL, branch, subPath, dest, destCategory, skillNames, copyMode)
		}

		if copyMode {
			return r.cloneAndCopy(repoURL, branch, dest)
		}
		return CloneRepoWithBranch(repoURL, branch, dest)
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
	if err := CloneRepoWithBranch(repoURL, branch, cloneDest); err != nil {
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
	if err := CloneRepoWithBranch(repoURL, branch, cloneDest); err != nil {
		return fmt.Errorf("cloning %s: %w", repoURL, err)
	}

	// Remove .git directory before copying
	os.RemoveAll(filepath.Join(cloneDest, ".git"))
	return r.copyDir(cloneDest, dest)
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
		found, err := r.FindSkillDir(name)
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

// FindSkillDir searches all category directories under the registry for a
// skill named name and returns its path, or "" if not found. Exported so
// cmd packages (update) share one implementation.
func (r *Registry) FindSkillDir(name string) (string, error) {
	path, _, err := r.FindSkillWithCategory(name)
	return path, err
}

// FindSkillWithCategory is like FindSkillDir but also returns the category
// directory the skill lives in. Both return "" when the skill is not present.
// Exported so the installer (which needs the category to pick target tools)
// doesn't re-implement the category walk.
func (r *Registry) FindSkillWithCategory(name string) (path, category string, err error) {
	skillsDir := r.skillsDir()
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return "", "", err
	}
	for _, cat := range entries {
		if !cat.IsDir() {
			continue
		}
		candidate := filepath.Join(skillsDir, cat.Name(), name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, cat.Name(), nil
		}
	}
	return "", "", nil
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
	found, err := r.FindSkillDir(name)
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("skill %q not found in registry", name)
	}
	return found, nil
}

// ── helpers shared by skill and MCP listing ──

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

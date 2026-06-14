package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
)

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
					fm := parseSkillFrontmatter(skillMD)
					skills = append(skills, DiscoveredSkill{
						Name:        name,
						Description: fm.Description,
						Path:        skillDir,
						SkillMDPath: skillMD,
						Internal:    fm.Internal,
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
						fm := parseSkillFrontmatter(subSkillMD)
						skills = append(skills, DiscoveredSkill{
							Name:        name,
							Description: fm.Description,
							Path:        subSkillDir,
							SkillMDPath: subSkillMD,
							Internal:    fm.Internal,
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
								fm := parseSkillFrontmatter(skillMD)
								skills = append(skills, DiscoveredSkill{
									Name:        name,
									Description: fm.Description,
									Path:        skillPath,
									SkillMDPath: skillMD,
									Internal:    fm.Internal,
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
							fm := parseSkillFrontmatter(skillMD)
							skills = append(skills, DiscoveredSkill{
								Name:        name,
								Description: fm.Description,
								Path:        skillPath,
								SkillMDPath: skillMD,
								Internal:    fm.Internal,
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
		fm := parseSkillFrontmatter(rootMD)
		skills = append(skills, DiscoveredSkill{
			Name:        name,
			Description: fm.Description,
			Path:        dir,
			SkillMDPath: rootMD,
			Internal:    fm.Internal,
		})
	}

	// Filter out internal skills unless INSTALL_INTERNAL_SKILLS is set.
	if !internalSkillsVisible() {
		filtered := skills[:0]
		for _, s := range skills {
			if !s.Internal {
				filtered = append(filtered, s)
			}
		}
		skills = filtered
	}

	return skills, nil
}

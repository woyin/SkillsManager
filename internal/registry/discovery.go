// Package registry 的 discovery.go 负责在克隆的仓库（或本地目录）中
// 扫描出所有可识别的技能（SKILL.md）。
//
// 支持三种仓库布局：
//   1. 扁平布局： container/<name>/SKILL.md
//   2. 目录布局： container/<category>/<name>/SKILL.md
//   3. 插件清单： .{claude,codex,agents,gemini}-plugin/ 下的
//      marketplace.json 或 plugin.json
//
// 此外，若仓库根直接存在 SKILL.md，则整个目录被视为单个技能。
package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// 标准技能发现路径（取自 vercel-labs/skills 约定）。
// 遍历顺序即优先级：先匹配的容器先收录，后续同名技能会被忽略。
var skillContainerDirs = []string{
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

// 插件清单可能所在的目录名（不同 AI 代理使用不同前缀）。
var pluginManifestDirs = []string{".claude-plugin", ".codex-plugin", ".agents-plugin", ".gemini-plugin"}

// DiscoverSkills 在 dir 下查找所有 SKILL.md，返回已发现的技能列表。
// 内部技能（metadata.internal: true）默认会被过滤掉，除非环境变量
// INSTALL_INTERNAL_SKILLS 被设为真值。
func DiscoverSkills(dir string) ([]DiscoveredSkill, error) {
	var skills []DiscoveredSkill

	// seen 用于按技能名去重；由于容器遍历有顺序，先发现者优先。
	seen := make(map[string]bool)

	for _, container := range skillContainerDirs {
		containerPath := filepath.Join(dir, container)
		// 仅当容器目录存在才进入；os.Stat 比 ReadDir 更廉价，能在
		// 大多数容器（通常不存在）的情况下提前短路。
		if _, err := os.Stat(containerPath); err != nil {
			continue
		}

		entries, err := os.ReadDir(containerPath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			// 仅处理目录；显式跳过版本控制与依赖目录。
			if !entry.IsDir() || entry.Name() == ".git" || entry.Name() == "node_modules" {
				continue
			}

			skillDir := filepath.Join(containerPath, entry.Name())

			// 扁平布局：container/<name>/SKILL.md
			tryAddSkill(skillDir, entry.Name(), seen, &skills)

			// 目录布局：container/<category>/<name>/SKILL.md
			subEntries, err := os.ReadDir(skillDir)
			if err != nil {
				continue
			}
			for _, subEntry := range subEntries {
				if !subEntry.IsDir() {
					continue
				}
				subSkillDir := filepath.Join(skillDir, subEntry.Name())
				tryAddSkill(subSkillDir, subEntry.Name(), seen, &skills)
			}
		}
	}

	// ── 插件清单发现 ──
	// 检查 .claude-plugin / .codex-plugin / .agents-plugin / .gemini-plugin
	// 目录下的 marketplace.json（多插件）与 plugin.json（单插件）。
	for _, pluginDirName := range pluginManifestDirs {
		pluginDir := filepath.Join(dir, pluginDirName)

		// marketplace.json：一个清单可声明多个插件。
		if data, err := os.ReadFile(filepath.Join(pluginDir, "marketplace.json")); err == nil {
			var marketplace pluginMarketplace
			if err := json.Unmarshal(data, &marketplace); err == nil {
				pluginRoot := resolvePluginRoot(dir, marketplace.Metadata.PluginRoot)
				addPluginSkills(marketplace.Plugins, pluginRoot, seen, &skills)
			}
		}

		// plugin.json：单个插件定义。
		if data, err := os.ReadFile(filepath.Join(pluginDir, "plugin.json")); err == nil {
			var plugin pluginManifest
			if err := json.Unmarshal(data, &plugin); err == nil {
				pluginRoot := resolvePluginRoot(dir, plugin.PluginRoot)
				addPluginSkills([]pluginManifest{plugin}, pluginRoot, seen, &skills)
			}
		}
	}

	// 若以上标准位置都未发现技能，则把仓库根目录下的 SKILL.md 视为
	// 整个仓库即一个技能。
	rootMD := filepath.Join(dir, "SKILL.md")
	if !seen["."] {
		if _, err := os.Stat(rootMD); err == nil {
			fm := parseSkillFrontmatter(rootMD)
			seen["."] = true
			skills = append(skills, DiscoveredSkill{
				Name:        filepath.Base(dir),
				Description: fm.Description,
				Path:        dir,
				SkillMDPath: rootMD,
				Internal:    fm.Internal,
			})
		}
	}

	// 过滤内部技能（除非 INSTALL_INTERNAL_SKILLS 为真值）。
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

// tryAddSkill 尝试把 skillDir 作为技能目录加入列表。
// 若该目录无 SKILL.md，或者同名技能已收录，则什么都不做。
//
// 采用 os.Stat 做存在性预检，绝大多数候选目录没有 SKILL.md，
// 廉价的 Stat 让我们避免昂贵的 ReadFile 打开/关闭开销。
func tryAddSkill(skillDir, name string, seen map[string]bool, skills *[]DiscoveredSkill) {
	skillMD := filepath.Join(skillDir, "SKILL.md")
	if _, err := os.Stat(skillMD); err != nil {
		return
	}
	if seen[name] {
		return
	}
	seen[name] = true
	fm := parseSkillFrontmatter(skillMD)
	*skills = append(*skills, DiscoveredSkill{
		Name:        name,
		Description: fm.Description,
		Path:        skillDir,
		SkillMDPath: skillMD,
		Internal:    fm.Internal,
	})
}

// resolvePluginRoot 把清单声明的 pluginRoot（可能是相对路径）解析为
// 绝对路径；为空时回退为仓库根 dir。
func resolvePluginRoot(dir, pluginRoot string) string {
	if pluginRoot == "" {
		return dir
	}
	if filepath.IsAbs(pluginRoot) {
		return pluginRoot
	}
	return filepath.Join(dir, pluginRoot)
}

// addPluginSkills 遍历清单声明的若干插件，把其中每个技能相对路径解析为
// 绝对路径，存在 SKILL.md 即收录。
func addPluginSkills(plugins []pluginManifest, pluginRoot string, seen map[string]bool, skills *[]DiscoveredSkill) {
	for _, plugin := range plugins {
		for _, relPath := range plugin.Skills {
			skillPath := relPath
			if !filepath.IsAbs(skillPath) {
				skillPath = filepath.Join(pluginRoot, relPath)
			}
			tryAddSkill(skillPath, filepath.Base(skillPath), seen, skills)
		}
	}
}

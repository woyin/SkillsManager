// Package registry 的 discovery.go 负责在克隆的仓库（或本地目录）中
// 扫描出所有可识别的技能（SKILL.md）。
//
// 支持三种仓库布局：
//  1. 扁平布局： container/<name>/SKILL.md
//  2. 目录布局： container/<category>/<name>/SKILL.md
//  3. 插件清单： .{claude,codex,agents,gemini}-plugin/ 下的
//     marketplace.json 或 plugin.json
//
// 此外，若仓库根直接存在 SKILL.md，则整个目录被视为单个技能。
//
// Input: encoding/json, os, path/filepath
// Output: func DiscoverSkills
// Pos: 数据层-技能发现
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// 标准技能发现路径（取自 vercel-labs/skills 约定 + npx skills 的
// AGENT_PROJECT_SKILL_DIRS）。遍历顺序即优先级：先匹配的容器先收录，
// 后续同名技能会被忽略。
var skillContainerDirs = []string{
	".",
	"skills",
	"skills/.curated",
	"skills/.experimental",
	"skills/.system",
	// Agent-specific project skill dirs (aligned with npx skills).
	".agents/skills",
	".claude/skills",
	".codex/skills",
	".gemini/skills",
	".cline/skills",
	".codebuddy/skills",
	".commandcode/skills",
	".continue/skills",
	".github/skills",
	".goose/skills",
	".grok/skills",
	".iflow/skills",
	".junie/skills",
	".kilocode/skills",
	".kimchi/skills",
	".kiro/skills",
	".mux/skills",
	".neovate/skills",
	".opencode/skills",
	".openhands/skills",
	".pi/skills",
	".qoder/skills",
	".roo/skills",
	".trae/skills",
	".windsurf/skills",
	".zcode/skills",
	".zencoder/skills",
}

// 插件清单可能所在的目录名（不同 AI 代理使用不同前缀）。
var pluginManifestDirs = []string{".claude-plugin", ".codex-plugin", ".agents-plugin", ".gemini-plugin"}

// isSkipDir 判断目录名是否应在技能发现时跳过：版本控制、依赖目录与各类
// 构建产物（对齐 npx skills 的 SKIP_DIRS：node_modules, .git, dist, build,
// __pycache__）。
func isSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "dist", "build", "__pycache__":
		return true
	}
	return false
}

// DiscoverOptions 控制 DiscoverSkillsWithOptions 的发现行为。
type DiscoverOptions struct {
	// FullDepth 为 true 时，除标准容器扫描外，无条件递归遍历整个仓库，
	// 把容器目录之外（如 examples/、tests/ 下）任何含 SKILL.md 的目录
	// 也收录进来。对齐 npx skills 的 --full-depth。已发现的同名技能
	// 不被覆盖（shallower shadows deeper）。
	FullDepth bool

	// AutoFullDepth 为 true 时，若标准容器扫描 + 根 SKILL.md 兜底均未
	// 发现任何技能，则自动回退到一次全仓库递归（对齐 npx skills 的
	// "If no skills found in standard locations, a recursive search is
	// performed"）。与 FullDepth 的区别：仅在标准位置为空时才触发，
	// 因此对正常仓库（有标准技能）无额外开销。add/install 的发现路径
	// 默认启用它。
	AutoFullDepth bool

	// IncludeInternal 为 true 时，不过滤 metadata.internal 为真的技能。
	// 对齐 npx skills 的 includeInternal 选项：当用户通过 --skill 明确
	// 选择技能时（或相应工作流），内部技能应可见以便被选中。
	IncludeInternal bool
}

// DiscoverSkills 在 dir 下查找所有 SKILL.md，返回已发现的技能列表。
// 等价于 DiscoverSkillsWithOptions(dir, DiscoverOptions{})，供不需要
// full-depth 的调用方使用。
// 内部技能（metadata.internal: true）默认会被过滤掉，除非环境变量
// INSTALL_INTERNAL_SKILLS 被设为真值。
func DiscoverSkills(dir string) ([]DiscoveredSkill, error) {
	return DiscoverSkillsWithOptions(dir, DiscoverOptions{})
}

// DiscoverSkillsWithOptions 是发现的完整入口：先按标准容器布局扫描，
// 再按 opts.FullDepth 决定是否递归全仓库补收容器外的 SKILL.md。
func DiscoverSkillsWithOptions(dir string, opts DiscoverOptions) ([]DiscoveredSkill, error) {
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
			// 仅处理目录；跳过版本控制、依赖与构建产物目录。
			if !entry.IsDir() || isSkipDir(entry.Name()) {
				continue
			}

			skillDir := filepath.Join(containerPath, entry.Name())

			// 扁平布局：container/<name>/SKILL.md
			tryAddSkill(skillDir, entry.Name(), "", seen, &skills)

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
				tryAddSkill(subSkillDir, subEntry.Name(), "", seen, &skills)
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
			name := usableSkillName(fm.Name, filepath.Base(dir))
			if !seen[name] {
				seen["."] = true
				seen[name] = true
				skills = append(skills, DiscoveredSkill{
					Name:        SanitizeMetadata(name),
					Description: SanitizeMetadata(fm.Description),
					Path:        dir,
					SkillMDPath: rootMD,
					Internal:    fm.Internal,
				})
			}
		}
	}

	// ── full-depth 递归补收 ──
	// --full-depth：在标准容器扫描之外，递归遍历整个仓库，把容器目录外
	// 任何含 SKILL.md 的目录（如 examples/<x>/SKILL.md、tests/<x>/SKILL.md）
	// 也收录。已发现的同名技能优先（shallower shadows deeper），故仅在
	// tryAddSkill 内部按 seen 去重即可保证不覆盖。
	if opts.FullDepth {
		discoverFullDepth(dir, seen, &skills)
	} else if opts.AutoFullDepth && len(skills) == 0 {
		// 标准位置一无所获：回退到全仓库递归，避免漏掉完全放在非标准
		// 布局（如 examples/<x>/<y>/SKILL.md）里的技能。
		discoverFullDepth(dir, seen, &skills)
	}

	// 过滤内部技能（除非 INSTALL_INTERNAL_SKILLS 为真值）。
	if !opts.IncludeInternal && !internalSkillsVisible() {
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

// discoverFullDepth 从 root 递归遍历整个目录树，把每个含 SKILL.md 的目录
// 经 tryAddSkill 尝试收录。跳过 .git / node_modules 这类噪音目录。
// 不重复收录标准容器已发现的同名技能（tryAddSkill 内部按 seen 去重）。
func discoverFullDepth(root string, seen map[string]bool, skills *[]DiscoveredSkill) {
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // 某个子树不可读就跳过，继续遍历其它
		}
		if d.IsDir() {
			// 跳过版本控制、依赖与构建产物目录，避免无意义遍历与误收录。
			if isSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		// 命中 SKILL.md：以其父目录作为技能目录尝试收录。
		if d.Name() == "SKILL.md" {
			skillDir := filepath.Dir(path)
			tryAddSkill(skillDir, filepath.Base(skillDir), "", seen, skills)
		}
		return nil
	})
}

// tryAddSkill 尝试把 skillDir 作为技能目录加入列表。
// 若该目录无 SKILL.md，或者同名技能已收录，则什么都不做。
//
// 采用 os.Stat 做存在性预检，绝大多数候选目录没有 SKILL.md，
// 廉价的 Stat 让我们避免昂贵的 ReadFile 打开/关闭开销。
func tryAddSkill(skillDir, name, pluginName string, seen map[string]bool, skills *[]DiscoveredSkill) {
	skillMD := filepath.Join(skillDir, "SKILL.md")
	if _, err := os.Stat(skillMD); err != nil {
		return
	}
	fm := parseSkillFrontmatter(skillMD)
	name = usableSkillName(fm.Name, name)
	if seen[name] {
		return
	}
	seen[name] = true
	*skills = append(*skills, DiscoveredSkill{
		Name:        SanitizeMetadata(name),
		Description: SanitizeMetadata(fm.Description),
		Path:        skillDir,
		SkillMDPath: skillMD,
		PluginName:  pluginName,
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
			tryAddSkill(skillPath, filepath.Base(skillPath), plugin.Name, seen, skills)
			// 标准容器扫描可能已先发现同一技能（pluginName 为空）。
			// 按解析后的绝对路径回填 pluginName，对齐 npx 的 enhanceSkill 行为。
			backfillPluginName(skillPath, plugin.Name, *skills)
		}
	}
}

// backfillPluginName 把已发现技能（按解析路径匹配）的 PluginName 设为 pluginName。
// 仅当现有 PluginName 为空时才覆盖，避免优先级更高的来源被改写。
func backfillPluginName(skillPath, pluginName string, skills []DiscoveredSkill) {
	abs, err := filepath.Abs(skillPath)
	if err != nil {
		abs = skillPath
	}
	for i := range skills {
		if skills[i].PluginName != "" {
			continue
		}
		existing, err := filepath.Abs(skills[i].Path)
		if err != nil {
			existing = skills[i].Path
		}
		if existing == abs {
			skills[i].PluginName = pluginName
		}
	}
}

// Installer 把 profile + 临时附加项落为具体的符号链接安装，
// 并把 MCP 定义合并进项目的 .mcp.json。
//
// 典型流程（见 Install）：
//  1. 载入 profile（可选），合并临时 skills/MCP；
//  2. 去重；
//  3. 为每个 skill 在对应代理目录创建符号链接（或拷贝）；
//  4. 把每个 MCP 合并进项目 .mcp.json；
//  5. 把结果写入 .sm.json。
//
// Input: encoding/json, fmt, io, os, path/filepath, strings, github.com/woyin/skills-manager/internal/fsutil, github.com/woyin/skills-manager/internal/home, github.com/woyin/skills-manager/internal/profile, github.com/woyin/skills-manager/internal/project, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/internal/symlink, github.com/woyin/skills-manager/internal/tool
// Output: type Installer, type InstallResult, func New
// Pos: 业务层-技能安装器
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package installer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/woyin/skills-manager/internal/fsutil"
	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/profile"
	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
)

// Installer 把 profile + 临时附加项落为具体的符号链接安装与 MCP 合并。
type Installer struct {
	registry    *registry.Registry
	profiles    *profile.Loader
	tools       []tool.Tool
	input       io.Reader // 用于交互式确认（默认 os.Stdin）
	output      io.Writer // 用于告警输出（默认 os.Stderr）
	projectDir  string    // 项目根（项目 scope 时安装落地根）
	globalScope bool      // true=全局 ~/agent；false=项目 ./agent
	copyMode    bool      // true=拷贝文件而非 symlink（Copy Install）
}

// InstallResult 汇总一次 Install 调用链接了哪些 skill、合并了哪些 MCP。
type InstallResult struct {
	Skills []string
	MCP    []string
}

// New 构造一个 Installer，操作指定的注册表、profiles 目录与目标工具集合。
// 默认项目 scope（globalScope=false）、symlink 模式（copyMode=false）；
// 用 SetScope / SetCopyMode 覆盖。
func New(registryDir, profilesDir string, tools []tool.Tool) (*Installer, error) {
	return &Installer{
		registry: registry.New(registryDir),
		profiles: profile.NewLoader(profilesDir),
		tools:    tools,
		input:    os.Stdin,
		output:   os.Stderr,
	}, nil
}

// SetScope 设定安装范围：globalScope=true 装到 ~/<agent>，
// 否则装到 projectDir 下的项目级目录。返回 inst 便于链式调用。
func (inst *Installer) SetScope(projectDir string, globalScope bool) *Installer {
	inst.projectDir = projectDir
	inst.globalScope = globalScope
	return inst
}

// SetCopyMode 设定是否以文件拷贝（Copy Install）代替 symlink。
// 拷贝会把 registry 原件目录整体复制（含其 .sm-origin.json，若存在），
// 使 Copy Install 实体可被 sm update --in-place 就地刷新。
func (inst *Installer) SetCopyMode(copyMode bool) *Installer {
	inst.copyMode = copyMode
	return inst
}

// Install 把 skills 与 MCP 安装到 projectDir。
//   - profileName：可选的 profile 名称；
//   - extraSkills / extraMCP：临时附加项。
//
// 任何环节失败都打印告警并跳过（不中断整体），最后写回 .sm.json。
func (inst *Installer) Install(projectDir, profileName string, extraSkills, extraMCP []string) (*InstallResult, error) {
	if profileName == "" && len(extraSkills) == 0 && len(extraMCP) == 0 {
		return nil, fmt.Errorf("nothing to install: specify --profile, or add skills/mcp to .sm.json")
	}

	var allSkills []string
	var allMCP []string

	// 载入 profile 作为基础。
	if profileName != "" {
		p, err := inst.profiles.Load(profileName)
		if err != nil {
			return nil, fmt.Errorf("loading profile %q: %w", profileName, err)
		}
		allSkills = append(allSkills, p.Skills...)
		allMCP = append(allMCP, p.MCP...)
	}

	// 合并临时附加项。
	allSkills = append(allSkills, extraSkills...)
	allMCP = append(allMCP, extraMCP...)

	// 去重（保留首次出现的顺序）。
	allSkills = deduplicate(allSkills)
	allMCP = deduplicate(allMCP)

	result := &InstallResult{}

	// 安装 skills：每个变为指向注册表原始目录的符号链接。
	for _, skillName := range allSkills {
		links, err := inst.installSkill(skillName)
		if err != nil {
			fmt.Fprintf(inst.output, "warning: skipping skill %q: %v\n", skillName, err)
			continue
		}
		result.Skills = append(result.Skills, links...)
	}

	// 安装 MCP：合并进项目 .mcp.json。
	for _, mcpName := range allMCP {
		if err := inst.installMCP(projectDir, mcpName); err != nil {
			fmt.Fprintf(inst.output, "warning: skipping MCP %q: %v\n", mcpName, err)
			continue
		}
		result.MCP = append(result.MCP, mcpName)
	}

	// 持久化 .sm.json。
	pm := project.NewManager(projectDir)
	config := &project.Config{
		Profile: profileName,
		Skills:  extraSkills,
		MCP:     extraMCP,
	}
	if err := pm.Save(config); err != nil {
		return result, fmt.Errorf("writing .sm.json: %w", err)
	}

	return result, nil
}

// InstallFromRegistry 从本地 registry 原件按名安装（Registry Install），
// 不重新克隆任何 source —— 因此安装是 O(1) 的 symlink（或拷贝）。
//
//   - category 为空时：对每个 name 走歧义检测——0 匹配报错（不 fallback
//     到 Direct Install）、1 匹配直接装、>1 匹配报错要求显式 --category。
//   - category 非空时：直接定位 registry/skills/<category>/<name>，不存在则报错。
//
// 落地范围与模式由 SetScope / SetCopyMode 预设。返回安装到的实体路径。
func (inst *Installer) InstallFromRegistry(names []string, category string) (*InstallResult, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("registry install requires at least one skill name")
	}

	result := &InstallResult{}
	for _, name := range names {
		skillPath, cat, err := inst.resolveRegistrySkill(name, category)
		if err != nil {
			fmt.Fprintf(inst.output, "warning: skipping skill %q: %v\n", name, err)
			continue
		}
		links, err := inst.createSymlinks(name, skillPath, cat)
		if err != nil {
			fmt.Fprintf(inst.output, "warning: skipping skill %q: %v\n", name, err)
			continue
		}
		result.Skills = append(result.Skills, links...)
	}
	return result, nil
}

// resolveRegistrySkill 解析某 name 在 registry 中的原件路径与分类。
// category 非空时直接定位；为空时做歧义检测（0/1/多匹配）。
func (inst *Installer) resolveRegistrySkill(name, category string) (path, cat string, err error) {
	if category != "" {
		p := filepath.Join(inst.registry.Dir(), "skills", category, name)
		if _, statErr := os.Stat(p); statErr != nil {
			return "", "", fmt.Errorf("skill %q not found in category %q", name, category)
		}
		return p, category, nil
	}

	matches, mErr := inst.registry.FindSkillCategories(name)
	if mErr != nil {
		return "", "", mErr
	}
	if len(matches) == 0 {
		return "", "", fmt.Errorf("skill %q is not in the registry; run `sm add <source>` first (no fallback to direct install)", name)
	}
	if len(matches) > 1 {
		var cats []string
		for _, m := range matches {
			cats = append(cats, m.Category)
		}
		return "", "", fmt.Errorf("skill %q exists in multiple categories (%s); pass --category <cat> to disambiguate", name, strings.Join(cats, ", "))
	}
	return matches[0].Path, matches[0].Category, nil
}

// installSkill 安装单个 skill。name 可能是一个分类目录（安装其中全部）。
func (inst *Installer) installSkill(name string) ([]string, error) {
	// 先判断 name 是否是分类目录。
	skillsDir := filepath.Join(inst.registry.Dir(), "skills")
	categoryPath := filepath.Join(skillsDir, name)
	if info, err := os.Stat(categoryPath); err == nil && info.IsDir() {
		return inst.installCategory(name)
	}

	// 否则当作单个技能名查找。
	skillPath, category, err := inst.findSkill(name)
	if err != nil {
		return nil, err
	}

	return inst.createSymlinks(name, skillPath, category)
}

// installCategory 安装某分类目录下的全部技能。
func (inst *Installer) installCategory(category string) ([]string, error) {
	skillsDir := filepath.Join(inst.registry.Dir(), "skills", category)
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, err
	}

	var allLinks []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".gitkeep" {
			continue
		}
		skillPath := filepath.Join(skillsDir, entry.Name())
		links, err := inst.createSymlinks(entry.Name(), skillPath, category)
		if err != nil {
			fmt.Fprintf(inst.output, "warning: skipping skill %q: %v\n", entry.Name(), err)
			continue
		}
		allLinks = append(allLinks, links...)
	}
	return allLinks, nil
}

// createSymlinks 为技能创建指向 absSkillPath 的符号链接（或拷贝），
// 目标代理由分类决定（见 getToolsForCategory），落地范围由 scope 决定。
func (inst *Installer) createSymlinks(name, skillPath, category string) ([]string, error) {
	var links []string
	absSkillPath, err := filepath.Abs(skillPath)
	if err != nil {
		return nil, err
	}

	// 按分类决定目标工具集合。
	targetTools := inst.getToolsForCategory(category)

	for _, t := range targetTools {
		// 计算该工具的落地根：全局用 home+SkillDir，项目用 GetProjectSkillDir。
		var skillDir string
		if inst.globalScope {
			skillDir = t.SkillDir
			if !filepath.IsAbs(skillDir) {
				skillDir = filepath.Join(home.Dir(), skillDir)
			}
		} else {
			skillDir = tool.GetProjectSkillDir(t, inst.projectDir)
			if skillDir == "" {
				// 该工具无项目级目录配置，跳过。
				continue
			}
		}
		link := filepath.Join(skillDir, name)

		installed, err := inst.placeSkill(absSkillPath, link)
		if err != nil {
			return nil, err
		}
		if installed {
			links = append(links, link)
		}
	}

	return links, nil
}

// placeSkill 把 absSkillPath 落到 link：copyMode 时整体拷贝（含 origin），
// 否则建立 symlink。
func (inst *Installer) placeSkill(absSkillPath, link string) (bool, error) {
	if inst.copyMode {
		return inst.ensureCopy(absSkillPath, link)
	}
	return inst.ensureSymlink(absSkillPath, link)
}

// ensureCopy 把 absSkillPath 整体拷贝到 link，覆盖已存在的同名实体。
// 拷贝包含原件目录内的 .sm-origin.json（若有），使 Copy Install 实体
// 可被 sm update --in-place 通过自身 origin 就地刷新。
func (inst *Installer) ensureCopy(absSkillPath, link string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		return false, fmt.Errorf("creating parent dir: %w", err)
	}
	// 覆盖式：已存在（旧 symlink 或旧拷贝）则先清掉再拷贝。
	if _, err := os.Lstat(link); err == nil {
		_ = os.RemoveAll(link)
	}
	return true, fsutil.CopyDir(absSkillPath, link)
}

// getToolsForCategory 按分类决定目标工具。
// 单工具特殊目录（codex-only 等）只对应一个工具；其它分类（global、自定义）
// 目标为全部已配置工具。
func (inst *Installer) getToolsForCategory(category string) []tool.Tool {
	if name := tool.NameForSpecialDir(category); name != "" {
		return inst.findTool(name)
	}
	return inst.tools
}

// findTool 按工具名查找，先在 inst.tools 中找，找不到则回退到全局目录。
func (inst *Installer) findTool(name string) []tool.Tool {
	for _, t := range inst.tools {
		if t.Name == name {
			return []tool.Tool{t}
		}
	}
	if t := tool.ToolByName(name); t != nil {
		return []tool.Tool{*t}
	}
	return nil
}

// ensureSymlink 确保 link 指向 target。
//   - 已是正确目标：no-op；
//   - 指向其它目标：交互式询问是否替换；
//   - 不存在：创建新链接；
//   - 存在但非符号链接：报错。
func (inst *Installer) ensureSymlink(target, link string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		return false, fmt.Errorf("creating parent dir: %w", err)
	}

	existingTarget, err := os.Readlink(link)
	if err == nil {
		// 已有符号链接：规整为绝对路径后比较。
		if !filepath.IsAbs(existingTarget) {
			existingTarget = filepath.Join(filepath.Dir(link), existingTarget)
		}
		if existingTarget == target {
			return true, nil
		}
		if !inst.confirmReplace(link, existingTarget, target) {
			return false, nil
		}
		if err := os.Remove(link); err != nil {
			return false, err
		}
		return true, os.Symlink(target, link)
	}

	// 已存在但非符号链接：拒绝覆盖。
	if _, statErr := os.Lstat(link); statErr == nil {
		return false, fmt.Errorf("%s already exists and is not a symlink", link)
	}

	return true, symlink.Create(target, link)
}

// confirmReplace 询问用户是否把 link 从 existingTarget 改指向 target。
// 读输入失败或非肯定回答都视为拒绝。
func (inst *Installer) confirmReplace(link, existingTarget, target string) bool {
	fmt.Fprintf(inst.output, "warning: %s already points to %s (want %s)\n", link, existingTarget, target)
	fmt.Fprint(inst.output, "Replace it? [y/N]: ")

	var answer string
	if _, err := fmt.Fscan(inst.input, &answer); err != nil {
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

// findSkill 在注册表中查找技能，返回路径与所在分类。
func (inst *Installer) findSkill(name string) (string, string, error) {
	path, category, err := inst.registry.FindSkillWithCategory(name)
	if err != nil {
		return "", "", err
	}
	if path == "" {
		return "", "", fmt.Errorf("skill %q not found in registry", name)
	}
	return path, category, nil
}

// mcpConfig 是 .mcp.json 文件的形状：顶层对象，唯一有意义的键是
// mcpServers。用类型化结构（替代 map[string]any）让合并类型安全，
// 并消除大量重复的类型断言。
type mcpConfig struct {
	MCPServers map[string]any `json:"mcpServers"`
}

// installMCP 把注册表中的某个 MCP 合并进项目的 .mcp.json。
// 同名 server 会被覆盖并打印告警。
func (inst *Installer) installMCP(projectDir, mcpName string) error {
	mcpPath := inst.registry.GetMCPPath(mcpName)
	mcpData, err := os.ReadFile(mcpPath)
	if err != nil {
		return fmt.Errorf("MCP %q not found in registry", mcpName)
	}

	var incoming mcpConfig
	if err := json.Unmarshal(mcpData, &incoming); err != nil {
		return fmt.Errorf("invalid MCP JSON: %w", err)
	}

	// 读取已有 .mcp.json；不存在则从空 servers map 开始。
	mcpFilePath := filepath.Join(projectDir, ".mcp.json")
	existing := mcpConfig{MCPServers: map[string]any{}}
	if data, err := os.ReadFile(mcpFilePath); err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			fmt.Fprintf(inst.output, "warning: parsing existing .mcp.json: %v\n", err)
		}
	}
	if existing.MCPServers == nil {
		existing.MCPServers = map[string]any{}
	}

	// 合并：后入者覆盖先入者。
	for name, def := range incoming.MCPServers {
		if _, exists := existing.MCPServers[name]; exists {
			fmt.Fprintf(inst.output, "warning: MCP server %q already exists in %s; overwriting\n", name, mcpFilePath)
		}
		existing.MCPServers[name] = def
	}

	merged, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(mcpFilePath, merged, 0644)
}

// deduplicate 去重，保留首次出现的顺序。
func deduplicate(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

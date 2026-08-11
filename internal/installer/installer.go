// Installer 把 profile + 临时附加项落为具体的符号链接安装，
// 并把 MCP 定义合并进项目的 .mcp.json。
//
// 典型流程（见 Install）：
//  1. 载入 profile（可选），合并临时 skills/MCP；
//  2. gatherAndPreflight：在任何写入前校验所有 skills/MCP 存在且唯一（ADR 0012）；
//  3. 为每个 skill 在对应代理目录创建符号链接（或拷贝）；
//  4. 把每个 MCP 合并进项目 .mcp.json；
//  5. 把结果写入 .sm.json；
//  6. 写入阶段任一失败时 rollbackLinks 回滚本次已创建的链接。
//
// Input: encoding/json, fmt, io, os, path/filepath, strings, github.com/woyin/skills-manager/internal/profile, github.com/woyin/skills-manager/internal/project, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/internal/tool
// Output: type Installer, type InstallResult, func New, func (Installer) Install, func (Installer) GatherAndPreflight
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

	"github.com/woyin/skills-manager/internal/profile"
	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/registry"
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
	// ADR 0012: preflight — 任一引用缺失/无效则零副作用。
	allSkills, allMCP, err := inst.gatherAndPreflight(projectDir, profileName, extraSkills, extraMCP)
	if err != nil {
		return nil, err
	}

	result := &InstallResult{}
	// Keep only placements that changed the filesystem.  An idempotent
	// placement is still reported in result.Skills for compatibility, but must
	// not be removed if a later MCP/config write fails.
	var placements []*PlacementResult

	// 安装 skills：每个变为指向注册表原始目录的符号链接。
	for _, skillName := range allSkills {
		links, changed, err := inst.installSkillWithPlacements(skillName)
		if err != nil {
			// ADR 0012: 写入阶段失败 → 回滚本次已产生的链接，零副作用。
			rollbackPlacements(placements)
			return nil, fmt.Errorf("profile install aborted: skill %q failed: %w (all changes rolled back)", skillName, err)
		}
		result.Skills = append(result.Skills, links...)
		placements = append(placements, changed...)
	}

	// 安装 MCP：合并进项目 .mcp.json。
	for _, mcpName := range allMCP {
		if err := inst.installMCP(projectDir, mcpName); err != nil {
			// ADR 0012: 回滚已产生的链接。
			rollbackPlacements(placements)
			return nil, fmt.Errorf("profile install aborted: MCP %q failed: %w (all changes rolled back)", mcpName, err)
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
		rollbackPlacements(placements)
		return nil, fmt.Errorf("writing .sm.json: %w (links rolled back)", err)
	}
	commitPlacements(placements)

	return result, nil
}

// gatherAndPreflight 合并 profile + 附加项，去重，并 preflight 每个 skill/MCP
// 是否在 registry 中存在且唯一。任一缺失/无效返回错误（零副作用）。
// 实现 ADR 0012 的 "Profile Install 在任何写操作前完整预检"。
func (inst *Installer) gatherAndPreflight(projectDir, profileName string, extraSkills, extraMCP []string) (allSkills, allMCP []string, err error) {
	if profileName != "" {
		p, lerr := inst.profiles.Load(profileName)
		if lerr != nil {
			return nil, nil, fmt.Errorf("loading profile %q: %w", profileName, lerr)
		}
		allSkills = append(allSkills, p.Skills...)
		allMCP = append(allMCP, p.MCP...)
	}
	allSkills = append(allSkills, extraSkills...)
	allMCP = append(allMCP, extraMCP...)
	allSkills = deduplicate(allSkills)
	allMCP = deduplicate(allMCP)

	// Preflight skills: 必须在 registry 存在且唯一（或为 category 目录）。
	for _, name := range allSkills {
		if _, statErr := os.Stat(filepath.Join(inst.registry.Dir(), "skills", name)); statErr == nil {
			continue // category 目录安装
		}
		matches, mErr := inst.registry.FindSkillCategories(name)
		if mErr != nil {
			return nil, nil, fmt.Errorf("preflight skill %q: %w", name, mErr)
		}
		if len(matches) == 0 {
			return nil, nil, fmt.Errorf("preflight: skill %q is not in the registry", name)
		}
		if len(matches) > 1 {
			return nil, nil, fmt.Errorf("preflight: skill %q exists in multiple categories; global uniqueness required", name)
		}
	}
	// Preflight MCP: 必须在 registry 存在。
	for _, name := range allMCP {
		mcpPath := inst.registry.GetMCPPath(name)
		if _, statErr := os.Stat(mcpPath); statErr != nil {
			return nil, nil, fmt.Errorf("preflight: MCP %q is not in the registry", name)
		}
	}
	return allSkills, allMCP, nil
}

// rollbackLinks 删除本次安装已创建的链接（写入阶段失败的回滚）。
func (inst *Installer) rollbackLinks(created []string) {
	for _, link := range created {
		_ = os.RemoveAll(link)
	}
}

// rollbackPlacements undoes changed placement operations in reverse order so
// repeated destinations are restored in the same order they were applied.
func rollbackPlacements(created []*PlacementResult) {
	for i := len(created) - 1; i >= 0; i-- {
		_ = created[i].Rollback()
	}
}

func commitPlacements(created []*PlacementResult) {
	for _, placement := range created {
		_ = placement.Commit()
	}
}

// GatherAndPreflight is the exported form of gatherAndPreflight, for tests
// and integrations that need to run the same preflight the installer uses.
func (inst *Installer) GatherAndPreflight(projectDir, profileName string, extraSkills, extraMCP []string) (allSkills, allMCP []string, err error) {
	return inst.gatherAndPreflight(projectDir, profileName, extraSkills, extraMCP)
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
		links, changed, err := inst.createSymlinksWithPlacements(name, skillPath, cat)
		if err != nil {
			fmt.Fprintf(inst.output, "warning: skipping skill %q: %v\n", name, err)
			continue
		}
		result.Skills = append(result.Skills, links...)
		commitPlacements(changed)
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
	links, _, err := inst.installSkillWithPlacements(name)
	return links, err
}

// installSkillWithPlacements is the transactional form used by profile
// installs.  The public behavior remains installSkill's []string result while
// changed placement records are retained for rollback until Install commits.
func (inst *Installer) installSkillWithPlacements(name string) ([]string, []*PlacementResult, error) {
	// 先判断 name 是否是分类目录。
	skillsDir := filepath.Join(inst.registry.Dir(), "skills")
	categoryPath := filepath.Join(skillsDir, name)
	if info, err := os.Stat(categoryPath); err == nil && info.IsDir() {
		return inst.installCategoryWithPlacements(name)
	}

	// 否则当作单个技能名查找。
	skillPath, category, err := inst.findSkill(name)
	if err != nil {
		return nil, nil, err
	}

	return inst.createSymlinksWithPlacements(name, skillPath, category)
}

// installCategory 安装某分类目录下的全部技能。
func (inst *Installer) installCategory(category string) ([]string, error) {
	links, _, err := inst.installCategoryWithPlacements(category)
	return links, err
}

func (inst *Installer) installCategoryWithPlacements(category string) ([]string, []*PlacementResult, error) {
	skillsDir := filepath.Join(inst.registry.Dir(), "skills", category)
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, nil, err
	}

	var allLinks []string
	var changed []*PlacementResult
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".gitkeep" {
			continue
		}
		skillPath := filepath.Join(skillsDir, entry.Name())
		links, placements, err := inst.createSymlinksWithPlacements(entry.Name(), skillPath, category)
		if err != nil {
			fmt.Fprintf(inst.output, "warning: skipping skill %q: %v\n", entry.Name(), err)
			continue
		}
		allLinks = append(allLinks, links...)
		changed = append(changed, placements...)
	}
	return allLinks, changed, nil
}

// createSymlinks 为技能创建指向 absSkillPath 的符号链接（或拷贝），
// 目标代理由分类决定（见 getToolsForCategory），落地范围由 scope 决定。
func (inst *Installer) createSymlinks(name, skillPath, category string) ([]string, error) {
	links, _, err := inst.createSymlinksWithPlacements(name, skillPath, category)
	return links, err
}

// createSymlinksWithPlacements is the shared placement path used by profile,
// registry, and (eventually) Direct Install.  It keeps Changed records so the
// caller can roll back only entities changed by this invocation.
func (inst *Installer) createSymlinksWithPlacements(name, skillPath, category string) ([]string, []*PlacementResult, error) {
	absSkillPath, err := filepath.Abs(skillPath)
	if err != nil {
		return nil, nil, err
	}

	targetScope := ProjectScope
	if inst.globalScope {
		targetScope = GlobalScope
	}
	targets := TargetDirectories(inst.getToolsForCategory(category), inst.projectDir, targetScope)
	placer := inst.newPlacement()
	var links []string
	var changed []*PlacementResult
	for _, target := range targets {
		link := filepath.Join(target.Directory, name)
		placement, err := placer.Place(absSkillPath, link)
		if err != nil {
			rollbackPlacements(changed)
			return nil, nil, err
		}
		if placement.Applied {
			links = append(links, link)
		}
		if placement.Changed {
			changed = append(changed, placement)
		}
	}

	return links, changed, nil
}

// newPlacement translates Installer's compatibility settings into the deep
// placement contract.  Existing profile/registry installs intentionally keep
// symlink fallback disabled; Direct Install can construct Placement directly
// with CopyOnSymlinkFailure when it migrates.
func (inst *Installer) newPlacement() *Placement {
	mode := SymlinkMode
	conflict := PromptOnConflict
	if inst.copyMode {
		mode = CopyMode
		conflict = ReplaceOnConflict
	}
	return NewPlacement(PlacementOptions{
		Mode:          mode,
		Fallback:      NoSymlinkFallback,
		Conflict:      conflict,
		Input:         inst.input,
		Output:        inst.output,
		RejectOverlap: true,
	})
}

// placeSkill preserves the old bool API for package-local callers while
// delegating all filesystem behavior to Placement.  The operation is
// committed immediately; profile Install uses createSymlinksWithPlacements
// when it needs a transaction spanning several skills/MCP writes.
func (inst *Installer) placeSkill(absSkillPath, link string) (bool, error) {
	placement, err := inst.newPlacement().Place(absSkillPath, link)
	if err != nil {
		return false, err
	}
	if commitErr := placement.Commit(); commitErr != nil {
		return false, commitErr
	}
	return placement.Applied, nil
}

// ensureCopy 把 absSkillPath 整体拷贝到 link，覆盖已存在的同名实体。
// 拷贝包含原件目录内的 .sm-origin.json（若有），使 Copy Install 实体
// 可被 sm update --in-place 通过自身 origin 就地刷新。
func (inst *Installer) ensureCopy(absSkillPath, link string) (bool, error) {
	placement, err := NewPlacement(PlacementOptions{
		Mode:          CopyMode,
		Conflict:      ReplaceOnConflict,
		Input:         inst.input,
		Output:        inst.output,
		RejectOverlap: true,
	}).Place(absSkillPath, link)
	if err != nil {
		return false, err
	}
	if commitErr := placement.Commit(); commitErr != nil {
		return false, commitErr
	}
	return placement.Applied, nil
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
	placement, err := NewPlacement(PlacementOptions{
		Mode:          SymlinkMode,
		Fallback:      NoSymlinkFallback,
		Conflict:      PromptOnConflict,
		Input:         inst.input,
		Output:        inst.output,
		RejectOverlap: true,
	}).Place(target, link)
	if err != nil {
		return false, err
	}
	if commitErr := placement.Commit(); commitErr != nil {
		return false, commitErr
	}
	return placement.Applied, nil
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

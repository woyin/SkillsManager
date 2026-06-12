// internal/installer/installer.go
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
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
)

type Installer struct {
	registry *registry.Registry
	profiles *profile.Loader
	tools    []tool.Tool
	input    io.Reader
	output   io.Writer
}

type InstallResult struct {
	Skills []string
	MCP    []string
}

func New(registryDir, profilesDir string, tools []tool.Tool) (*Installer, error) {
	return &Installer{
		registry: registry.New(registryDir),
		profiles: profile.NewLoader(profilesDir),
		tools:    tools,
		input:    os.Stdin,
		output:   os.Stderr,
	}, nil
}

// Install installs skills and MCP into a project directory.
// profileName: optional profile to apply.
// extraSkills, extraMCP: ad-hoc additions.
func (inst *Installer) Install(projectDir, profileName string, extraSkills, extraMCP []string) (*InstallResult, error) {
	if profileName == "" && len(extraSkills) == 0 && len(extraMCP) == 0 {
		return nil, fmt.Errorf("nothing to install: specify --profile, or add skills/mcp to .sm.json")
	}

	var allSkills []string
	var allMCP []string

	// Resolve profile
	if profileName != "" {
		p, err := inst.profiles.Load(profileName)
		if err != nil {
			return nil, fmt.Errorf("loading profile %q: %w", profileName, err)
		}
		allSkills = append(allSkills, p.Skills...)
		allMCP = append(allMCP, p.MCP...)
	}

	// Merge ad-hoc
	allSkills = append(allSkills, extraSkills...)
	allMCP = append(allMCP, extraMCP...)

	// Deduplicate
	allSkills = deduplicate(allSkills)
	allMCP = deduplicate(allMCP)

	result := &InstallResult{}

	// Install skills as symlinks
	for _, skillName := range allSkills {
		links, err := inst.installSkill(skillName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping skill %q: %v\n", skillName, err)
			continue
		}
		result.Skills = append(result.Skills, links...)
	}

	// Install MCP
	for _, mcpName := range allMCP {
		if err := inst.installMCP(projectDir, mcpName); err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping MCP %q: %v\n", mcpName, err)
			continue
		}
		result.MCP = append(result.MCP, mcpName)
	}

	// Write .sm.json
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

func (inst *Installer) installSkill(name string) ([]string, error) {
	// First check if name is a category
	skillsDir := filepath.Join(inst.registry.Dir(), "skills")
	categoryPath := filepath.Join(skillsDir, name)
	if info, err := os.Stat(categoryPath); err == nil && info.IsDir() {
		// It's a category, install all skills in it
		return inst.installCategory(name)
	}

	// Otherwise, find the skill in registry
	skillPath, category, err := inst.findSkill(name)
	if err != nil {
		return nil, err
	}

	return inst.createSymlinks(name, skillPath, category)
}

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
			fmt.Fprintf(os.Stderr, "warning: skipping skill %q: %v\n", entry.Name(), err)
			continue
		}
		allLinks = append(allLinks, links...)
	}
	return allLinks, nil
}

func (inst *Installer) createSymlinks(name, skillPath, category string) ([]string, error) {
	var links []string
	absSkillPath, err := filepath.Abs(skillPath)
	if err != nil {
		return nil, err
	}

	// Determine which tools to install to based on category
	targetTools := inst.getToolsForCategory(category)

	for _, t := range targetTools {
		skillDir := t.SkillDir
		// If skillDir is not absolute, make it absolute relative to home
		if !filepath.IsAbs(skillDir) {
			home, _ := os.UserHomeDir()
			skillDir = filepath.Join(home, skillDir)
		}
		link := filepath.Join(skillDir, name)
		if installed, err := inst.ensureSymlink(absSkillPath, link); err != nil {
			return nil, err
		} else if installed {
			links = append(links, link)
		}
	}

	return links, nil
}

func (inst *Installer) getToolsForCategory(category string) []tool.Tool {
	switch category {
	case registry.CodexOnly:
		return inst.findTool("codex")
	case registry.ClaudeOnly:
		return inst.findTool("claude")
	case registry.GeminiOnly:
		return inst.findTool("gemini")
	case registry.OpenCodeOnly:
		return inst.findTool("opencode")
	case registry.HermesOnly:
		return inst.findTool("hermes")
	case registry.OpenClawOnly:
		return inst.findTool("openclaw")
	default:
		// global, or any other category → all tools
		return inst.tools
	}
}

func (inst *Installer) findTool(name string) []tool.Tool {
	for _, t := range inst.tools {
		if t.Name == name {
			return []tool.Tool{t}
		}
	}
	// Fallback to default tool if not found in inst.tools
	switch name {
	case "codex":
		return []tool.Tool{tool.Codex}
	case "claude":
		return []tool.Tool{tool.Claude}
	case "gemini":
		return []tool.Tool{tool.Gemini}
	case "opencode":
		return []tool.Tool{tool.OpenCode}
	case "hermes":
		return []tool.Tool{tool.Hermes}
	case "openclaw":
		return []tool.Tool{tool.OpenClaw}
	default:
		return nil
	}
}

func (inst *Installer) ensureSymlink(target, link string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		return false, fmt.Errorf("creating parent dir: %w", err)
	}

	existingTarget, err := os.Readlink(link)
	if err == nil {
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

	if _, statErr := os.Lstat(link); statErr == nil {
		return false, fmt.Errorf("%s already exists and is not a symlink", link)
	}

	return true, symlink.Create(target, link)
}

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

func (inst *Installer) findSkill(name string) (string, string, error) {
	skillsDir := filepath.Join(inst.registry.Dir(), "skills")
	categories, err := os.ReadDir(skillsDir)
	if err != nil {
		return "", "", err
	}

	for _, cat := range categories {
		if !cat.IsDir() {
			continue
		}
		path := filepath.Join(skillsDir, cat.Name(), name)
		if _, err := os.Stat(path); err == nil {
			return path, cat.Name(), nil
		}
	}
	return "", "", fmt.Errorf("skill %q not found in registry", name)
}

func (inst *Installer) installMCP(projectDir, mcpName string) error {
	mcpPath := inst.registry.GetMCPPath(mcpName)
	mcpData, err := os.ReadFile(mcpPath)
	if err != nil {
		return fmt.Errorf("MCP %q not found in registry", mcpName)
	}

	var newMCP map[string]interface{}
	if err := json.Unmarshal(mcpData, &newMCP); err != nil {
		return fmt.Errorf("invalid MCP JSON: %w", err)
	}

	// Read existing .mcp.json or create new
	mcpFilePath := filepath.Join(projectDir, ".mcp.json")
	var existing map[string]interface{}

	if data, err := os.ReadFile(mcpFilePath); err == nil {
		json.Unmarshal(data, &existing)
	}
	if existing == nil {
		existing = map[string]interface{}{"mcpServers": map[string]interface{}{}}
	}

	// Merge mcpServers
	existingServers, _ := existing["mcpServers"].(map[string]interface{})
	if existingServers == nil {
		existingServers = make(map[string]interface{})
	}

	newServers, _ := newMCP["mcpServers"].(map[string]interface{})
	for k, v := range newServers {
		if _, exists := existingServers[k]; exists {
			fmt.Fprintf(inst.output, "warning: MCP server %q already exists in %s; overwriting\n", k, mcpFilePath)
		}
		existingServers[k] = v
	}
	existing["mcpServers"] = existingServers

	merged, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(mcpFilePath, merged, 0644)
}

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

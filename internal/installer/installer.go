// internal/installer/installer.go
package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/woyin/skills-manager/internal/profile"
	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/symlink"
)

type Installer struct {
	registry  *registry.Registry
	profiles  *profile.Loader
	codexDir  string
	claudeDir string
}

type InstallResult struct {
	Skills []string
	MCP    []string
}

func New(registryDir, profilesDir, codexDir, claudeDir string) (*Installer, error) {
	return &Installer{
		registry:  registry.New(registryDir),
		profiles:  profile.NewLoader(profilesDir),
		codexDir:  codexDir,
		claudeDir: claudeDir,
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

	// Determine install targets based on category
	if category == registry.CodexOnly {
		link := filepath.Join(inst.codexDir, name)
		if err := symlink.Create(skillPath, link); err == nil {
			links = append(links, link)
		}
	} else if category == registry.ClaudeOnly {
		link := filepath.Join(inst.claudeDir, name)
		if err := symlink.Create(skillPath, link); err == nil {
			links = append(links, link)
		}
	} else {
		// global, or any other category → both tools
		linkCodex := filepath.Join(inst.codexDir, name)
		linkClaude := filepath.Join(inst.claudeDir, name)
		if err := symlink.Create(skillPath, linkCodex); err == nil {
			links = append(links, linkCodex)
		}
		if err := symlink.Create(skillPath, linkClaude); err == nil {
			links = append(links, linkClaude)
		}
	}

	return links, nil
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

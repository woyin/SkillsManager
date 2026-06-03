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
)

type Installer struct {
	registry  *registry.Registry
	profiles  *profile.Loader
	codexDir  string
	claudeDir string
	input     io.Reader
	output    io.Writer
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
		input:     os.Stdin,
		output:    os.Stderr,
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

	// Determine install targets based on category
	if category == registry.CodexOnly {
		link := filepath.Join(inst.codexDir, name)
		if installed, err := inst.ensureSymlink(absSkillPath, link); err != nil {
			return nil, err
		} else if installed {
			links = append(links, link)
		}
	} else if category == registry.ClaudeOnly {
		link := filepath.Join(inst.claudeDir, name)
		if installed, err := inst.ensureSymlink(absSkillPath, link); err != nil {
			return nil, err
		} else if installed {
			links = append(links, link)
		}
	} else {
		// global, or any other category → both tools
		linkCodex := filepath.Join(inst.codexDir, name)
		linkClaude := filepath.Join(inst.claudeDir, name)
		if installed, err := inst.ensureSymlink(absSkillPath, linkCodex); err != nil {
			return nil, err
		} else if installed {
			links = append(links, linkCodex)
		}
		if installed, err := inst.ensureSymlink(absSkillPath, linkClaude); err != nil {
			return nil, err
		} else if installed {
			links = append(links, linkClaude)
		}
	}

	return links, nil
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

// internal/installer/installer_test.go
package installer

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/tool"
)

func setupTestEnv(t *testing.T) (registryDir, profilesDir, codexDir, claudeDir, projectDir string) {
	base := t.TempDir()
	registryDir = filepath.Join(base, "registry")
	profilesDir = filepath.Join(base, "profiles")
	codexDir = filepath.Join(base, ".codex", "skills")
	claudeDir = filepath.Join(base, ".claude", "skills")
	projectDir = filepath.Join(base, "project")

	// Create registry with test skills
	os.MkdirAll(filepath.Join(registryDir, "skills", "global", "global-skill"), 0755)
	os.WriteFile(filepath.Join(registryDir, "skills", "global", "global-skill", "SKILL.md"), []byte("# global"), 0644)
	os.MkdirAll(filepath.Join(registryDir, "skills", "cloudflare", "cf-skill"), 0755)
	os.WriteFile(filepath.Join(registryDir, "skills", "cloudflare", "cf-skill", "SKILL.md"), []byte("# cf"), 0644)
	os.MkdirAll(filepath.Join(registryDir, "skills", "codex-only", "codex-skill"), 0755)
	os.WriteFile(filepath.Join(registryDir, "skills", "codex-only", "codex-skill", "SKILL.md"), []byte("# codex"), 0644)
	os.MkdirAll(filepath.Join(registryDir, "skills", "claude-only", "claude-skill"), 0755)
	os.WriteFile(filepath.Join(registryDir, "skills", "claude-only", "claude-skill", "SKILL.md"), []byte("# claude"), 0644)

	// Create MCP
	mcpJSON := `{"mcpServers":{"test":{"type":"http","url":"https://example.com/mcp"}}}`
	os.MkdirAll(filepath.Join(registryDir, "mcp"), 0755)
	os.WriteFile(filepath.Join(registryDir, "mcp", "test.json"), []byte(mcpJSON), 0644)

	// Create profile
	profileData := map[string]any{
		"skills": []string{"global", "cloudflare"},
		"mcp":    []string{"test"},
	}
	pData, _ := json.Marshal(profileData)
	os.MkdirAll(profilesDir, 0755)
	os.WriteFile(filepath.Join(profilesDir, "cloudflare.json"), pData, 0644)

	// Create target dirs
	os.MkdirAll(codexDir, 0755)
	os.MkdirAll(claudeDir, 0755)
	os.MkdirAll(projectDir, 0755)

	return
}

func getTestTools(base string) []tool.Tool {
	return []tool.Tool{
		{Name: "codex", SkillDir: filepath.Join(base, ".codex", "skills")},
		{Name: "claude", SkillDir: filepath.Join(base, ".claude", "skills")},
	}
}

// getAbsoluteSkillDir returns the absolute path for a tool's skill directory
func getAbsoluteSkillDir(base, skillDir string) string {
	if filepath.IsAbs(skillDir) {
		return skillDir
	}
	return filepath.Join(base, skillDir)
}

func TestInstallWithProfile(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	base := filepath.Dir(registryDir)
	tools := getTestTools(base)

	inst, err := New(registryDir, profilesDir, tools)
	if err != nil {
		t.Fatalf("New installer failed: %v", err)
	}
	inst.SetScope(projectDir, true) // 这些测试验证全局落地路径

	result, err := inst.Install(projectDir, "cloudflare", nil, nil)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Check symlinks created
	codexDir := tools[0].SkillDir
	claudeDir := tools[1].SkillDir

	cfSkillCodex := filepath.Join(codexDir, "cf-skill")
	cfSkillClaude := filepath.Join(claudeDir, "cf-skill")
	globalSkillCodex := filepath.Join(codexDir, "global-skill")
	globalSkillClaude := filepath.Join(claudeDir, "global-skill")

	for _, link := range []string{cfSkillCodex, cfSkillClaude, globalSkillCodex, globalSkillClaude} {
		fi, err := os.Lstat(link)
		if err != nil {
			t.Errorf("Symlink not created: %s (%v)", link, err)
			continue
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("Not a symlink: %s", link)
		}
	}

	// Check .sm.json written
	smPath := filepath.Join(projectDir, ".sm.json")
	data, err := os.ReadFile(smPath)
	if err != nil {
		t.Fatalf(".sm.json not created: %v", err)
	}
	var config map[string]any
	json.Unmarshal(data, &config)
	if config["profile"] != "cloudflare" {
		t.Errorf("Expected profile 'cloudflare' in .sm.json")
	}

	// Check MCP merged into .mcp.json
	mcpPath := filepath.Join(projectDir, ".mcp.json")
	mcpData, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf(".mcp.json not created: %v", err)
	}
	if len(mcpData) == 0 {
		t.Error(".mcp.json is empty")
	}

	// Check result
	if len(result.Skills) != 4 { // global-skill (2 links) + cf-skill (2 links)
		t.Errorf("Expected 4 skill links, got %d", len(result.Skills))
	}
}

func TestInstallerCompatibilityHelpersDelegateToPlacement(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	base := filepath.Dir(registryDir)
	inst, err := New(registryDir, profilesDir, getTestTools(base))
	if err != nil {
		t.Fatal(err)
	}
	inst.SetScope(projectDir, true)

	skills, mcp, err := inst.GatherAndPreflight(projectDir, "", []string{"global-skill"}, nil)
	if err != nil || len(skills) != 1 || skills[0] != "global-skill" || len(mcp) != 0 {
		t.Fatalf("GatherAndPreflight = %v, %v, %v", skills, mcp, err)
	}

	links, err := inst.installSkill("global-skill")
	if err != nil || len(links) != 2 {
		t.Fatalf("installSkill = %v, %v", links, err)
	}
	categoryLinks, err := inst.installCategory("cloudflare")
	if err != nil || len(categoryLinks) != 2 {
		t.Fatalf("installCategory = %v, %v", categoryLinks, err)
	}

	source := filepath.Join(registryDir, "skills", "global", "global-skill")
	destination := filepath.Join(t.TempDir(), "direct-link")
	if applied, err := inst.placeSkill(source, destination); err != nil || !applied {
		t.Fatalf("placeSkill = %t, %v", applied, err)
	}

	copyDestination := filepath.Join(t.TempDir(), "direct-copy")
	if applied, err := inst.ensureCopy(source, copyDestination); err != nil || !applied {
		t.Fatalf("ensureCopy = %t, %v", applied, err)
	}
	if _, err := os.Stat(filepath.Join(copyDestination, "SKILL.md")); err != nil {
		t.Fatalf("copy contents missing: %v", err)
	}
}

func TestInstallWithRelativeRegistryCreatesValidSymlinks(t *testing.T) {
	registryDir, _, _, _, projectDir := setupTestEnv(t)
	base := filepath.Dir(registryDir)
	t.Chdir(base)
	tools := getTestTools(base)

	inst, err := New("registry", "profiles", tools)
	if err != nil {
		t.Fatalf("New installer failed: %v", err)
	}
	inst.SetScope(projectDir, true)

	_, err = inst.Install(projectDir, "", []string{"cf-skill"}, nil)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	for _, tool := range tools {
		link := filepath.Join(tool.SkillDir, "cf-skill")
		target, err := os.Readlink(link)
		if err != nil {
			t.Fatalf("Reading symlink failed: %v", err)
		}
		if !filepath.IsAbs(target) {
			t.Fatalf("Expected absolute symlink target for %s, got %s", link, target)
		}
		if _, err := os.Stat(target); err != nil {
			t.Fatalf("Symlink target should exist: %s (%v)", target, err)
		}
	}
}

func TestInstallWithAdHoc(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	base := filepath.Dir(registryDir)
	tools := getTestTools(base)

	inst, err := New(registryDir, profilesDir, tools)
	if err != nil {
		t.Fatalf("New installer failed: %v", err)
	}
	inst.SetScope(projectDir, true) // 这些测试验证全局落地路径

	result, err := inst.Install(projectDir, "", []string{"cf-skill"}, []string{"test"})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// cf-skill should go to both codex and claude
	for _, tool := range tools {
		link := filepath.Join(tool.SkillDir, "cf-skill")
		if _, err := os.Lstat(link); err != nil {
			t.Errorf("cf-skill not in %s", tool.Name)
		}
	}

	// MCP should be in .mcp.json
	mcpPath := filepath.Join(projectDir, ".mcp.json")
	if _, err := os.Stat(mcpPath); err != nil {
		t.Error(".mcp.json not created")
	}

	if len(result.MCP) != 1 {
		t.Errorf("Expected 1 MCP, got %d", len(result.MCP))
	}
}

func TestInstallCodexOnly(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	base := filepath.Dir(registryDir)
	tools := getTestTools(base)

	inst, err := New(registryDir, profilesDir, tools)
	if err != nil {
		t.Fatalf("New installer failed: %v", err)
	}
	inst.SetScope(projectDir, true) // 这些测试验证全局落地路径
	// Auto-accept replacements for this test
	inst.input = strings.NewReader("y\n")
	inst.output = io.Discard

	// codex-only skill should only appear in codex
	result, err := inst.Install(projectDir, "", []string{"codex-skill"}, nil)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	codexDir := tools[0].SkillDir
	claudeDir := tools[1].SkillDir

	t.Logf("codexDir: %s", codexDir)
	t.Logf("claudeDir: %s", claudeDir)
	t.Logf("result.Skills: %v", result.Skills)

	if _, err := os.Lstat(filepath.Join(codexDir, "codex-skill")); err != nil {
		t.Errorf("codex-skill not in codex: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(claudeDir, "codex-skill")); err == nil {
		t.Error("codex-skill should NOT be in claude")
	}

	if len(result.Skills) != 1 {
		t.Errorf("Expected 1 skill link, got %d", len(result.Skills))
	}
}

func TestInstallConflictDeclinedKeepsExistingSymlink(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	base := filepath.Dir(registryDir)
	tools := getTestTools(base)
	codexDir := tools[0].SkillDir

	existingTarget := filepath.Join(base, "external", "codex-skill")
	if err := os.MkdirAll(existingTarget, 0755); err != nil {
		t.Fatalf("creating existing target: %v", err)
	}
	linkPath := filepath.Join(codexDir, "codex-skill")
	if err := os.Symlink(existingTarget, linkPath); err != nil {
		t.Fatalf("creating existing symlink: %v", err)
	}

	inst, err := New(registryDir, profilesDir, tools)
	if err != nil {
		t.Fatalf("New installer failed: %v", err)
	}
	inst.SetScope(projectDir, true) // 这些测试验证全局落地路径
	inst.input = strings.NewReader("n\n")
	inst.output = io.Discard

	result, err := inst.Install(projectDir, "", []string{"codex-skill"}, nil)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if len(result.Skills) != 0 {
		t.Fatalf("Expected declined conflict to install no links, got %v", result.Skills)
	}
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink failed: %v", err)
	}
	if target != existingTarget {
		t.Fatalf("Expected existing symlink to remain, got %s", target)
	}
}

func TestInstallConflictAcceptedReplacesExistingSymlink(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	base := filepath.Dir(registryDir)
	tools := getTestTools(base)
	codexDir := tools[0].SkillDir

	existingTarget := filepath.Join(base, "external", "codex-skill")
	if err := os.MkdirAll(existingTarget, 0755); err != nil {
		t.Fatalf("creating existing target: %v", err)
	}
	linkPath := filepath.Join(codexDir, "codex-skill")
	if err := os.Symlink(existingTarget, linkPath); err != nil {
		t.Fatalf("creating existing symlink: %v", err)
	}

	inst, err := New(registryDir, profilesDir, tools)
	if err != nil {
		t.Fatalf("New installer failed: %v", err)
	}
	inst.SetScope(projectDir, true) // 这些测试验证全局落地路径
	inst.input = strings.NewReader("y\n")
	inst.output = io.Discard

	result, err := inst.Install(projectDir, "", []string{"codex-skill"}, nil)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if len(result.Skills) != 1 {
		t.Fatalf("Expected accepted conflict to install 1 link, got %v", result.Skills)
	}
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink failed: %v", err)
	}
	// The target should be the registry skill, not the external one
	expectedTarget := filepath.Join(registryDir, "skills", "codex-only", "codex-skill")
	if target != expectedTarget {
		t.Fatalf("Expected symlink to be replaced with %s, got %s", expectedTarget, target)
	}
}

func TestInstallMCPWarnsWhenOverwritingExistingServer(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	base := filepath.Dir(registryDir)
	tools := getTestTools(base)

	existingMCP := `{"mcpServers":{"test":{"type":"stdio","command":"old"}}}`
	if err := os.WriteFile(filepath.Join(projectDir, ".mcp.json"), []byte(existingMCP), 0644); err != nil {
		t.Fatalf("writing existing MCP: %v", err)
	}

	inst, err := New(registryDir, profilesDir, tools)
	if err != nil {
		t.Fatalf("New installer failed: %v", err)
	}
	inst.SetScope(projectDir, true) // 这些测试验证全局落地路径
	var output bytes.Buffer
	inst.output = &output

	_, err = inst.Install(projectDir, "", nil, []string{"test"})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if !strings.Contains(output.String(), `warning: MCP server "test" already exists`) {
		t.Fatalf("Expected overwrite warning, got %q", output.String())
	}
	data, err := os.ReadFile(filepath.Join(projectDir, ".mcp.json"))
	if err != nil {
		t.Fatalf("reading merged MCP: %v", err)
	}
	if !strings.Contains(string(data), "https://example.com/mcp") {
		t.Fatalf("Expected new MCP server to win, got %s", string(data))
	}
}

func TestInstallNoConfig(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	base := filepath.Dir(registryDir)
	tools := getTestTools(base)

	inst, err := New(registryDir, profilesDir, tools)
	if err != nil {
		t.Fatalf("New installer failed: %v", err)
	}
	inst.SetScope(projectDir, true) // 这些测试验证全局落地路径

	_, err = inst.Install(projectDir, "", nil, nil)
	if err == nil {
		t.Error("Expected error when no profile and no ad-hoc items")
	}
}

func TestGetToolsForCategory(t *testing.T) {
	base := t.TempDir()
	allTools := []tool.Tool{
		{Name: "codex", SkillDir: filepath.Join(base, ".codex", "skills")},
		{Name: "claude", SkillDir: filepath.Join(base, ".claude", "skills")},
		{Name: "gemini", SkillDir: filepath.Join(base, ".gemini", "skills")},
	}
	inst, _ := New(filepath.Join(base, "registry"), filepath.Join(base, "profiles"), allTools)

	tests := []struct {
		category string
		expected int
	}{
		{"codex-only", 1},
		{"claude-only", 1},
		{"gemini-only", 1},
		{"global", 3},
		{"cloudflare", 3},
	}

	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			tools := inst.getToolsForCategory(tt.category)
			if len(tools) != tt.expected {
				t.Errorf("getToolsForCategory(%q) returned %d tools, want %d", tt.category, len(tools), tt.expected)
			}
		})
	}
}

func TestFindTool(t *testing.T) {
	base := t.TempDir()
	allTools := []tool.Tool{
		{Name: "codex", SkillDir: filepath.Join(base, ".codex", "skills")},
		{Name: "claude", SkillDir: filepath.Join(base, ".claude", "skills")},
	}
	inst, _ := New(filepath.Join(base, "registry"), filepath.Join(base, "profiles"), allTools)

	// Found in inst.tools
	tools := inst.findTool("codex")
	if len(tools) != 1 || tools[0].Name != "codex" {
		t.Errorf("findTool('codex') = %v, want [codex]", tools)
	}

	// Not in inst.tools but in fallback
	tools = inst.findTool("gemini")
	if len(tools) != 1 || tools[0].Name != "gemini" {
		t.Errorf("findTool('gemini') = %v, want [gemini]", tools)
	}

	// Not found at all
	tools = inst.findTool("nonexistent")
	if tools != nil {
		t.Errorf("findTool('nonexistent') = %v, want nil", tools)
	}
}

func TestEnsureSymlinkSameTarget(t *testing.T) {
	base := t.TempDir()
	allTools := []tool.Tool{
		{Name: "codex", SkillDir: filepath.Join(base, ".codex", "skills")},
	}
	inst, _ := New(filepath.Join(base, "registry"), filepath.Join(base, "profiles"), allTools)

	target := filepath.Join(base, "target")
	link := filepath.Join(base, "links", "test")
	os.MkdirAll(target, 0755)

	// First create
	ok, err := inst.ensureSymlink(target, link)
	if err != nil || !ok {
		t.Fatalf("First ensureSymlink failed: ok=%v err=%v", ok, err)
	}

	// Same target — should be idempotent
	ok, err = inst.ensureSymlink(target, link)
	if err != nil {
		t.Fatalf("Second ensureSymlink with same target failed: %v", err)
	}
	if !ok {
		t.Error("Should return true for same target")
	}
}

func TestEnsureSymlinkNonSymlinkConflict(t *testing.T) {
	base := t.TempDir()
	allTools := []tool.Tool{
		{Name: "codex", SkillDir: filepath.Join(base, ".codex", "skills")},
	}
	inst, _ := New(filepath.Join(base, "registry"), filepath.Join(base, "profiles"), allTools)

	target := filepath.Join(base, "target")
	link := filepath.Join(base, "links", "test")
	os.MkdirAll(target, 0755)
	os.MkdirAll(link, 0755) // link is a directory, not a symlink

	ok, err := inst.ensureSymlink(target, link)
	if err == nil {
		t.Error("Expected error when link is a non-symlink file/dir")
	}
	if ok {
		t.Error("Should return false for non-symlink conflict")
	}
}

func TestEnsureSymlinkConflictReplaceNo(t *testing.T) {
	base := t.TempDir()
	allTools := []tool.Tool{
		{Name: "codex", SkillDir: filepath.Join(base, ".codex", "skills")},
	}

	input := bytes.NewBufferString("n\n")
	output := &bytes.Buffer{}
	inst := &Installer{
		registry: nil,
		profiles: nil,
		tools:    allTools,
		input:    input,
		output:   output,
	}

	target1 := filepath.Join(base, "target1")
	target2 := filepath.Join(base, "target2")
	link := filepath.Join(base, "links", "test")
	os.MkdirAll(target1, 0755)
	os.MkdirAll(target2, 0755)
	os.MkdirAll(filepath.Dir(link), 0755)
	os.Symlink(target1, link)

	ok, err := inst.ensureSymlink(target2, link)
	if err != nil {
		t.Fatalf("ensureSymlink with conflict failed: %v", err)
	}
	if ok {
		t.Error("Should return false when user declines replacement")
	}
}

func TestEnsureSymlinkConflictReplaceYes(t *testing.T) {
	base := t.TempDir()
	allTools := []tool.Tool{
		{Name: "codex", SkillDir: filepath.Join(base, ".codex", "skills")},
	}

	input := bytes.NewBufferString("y\n")
	output := &bytes.Buffer{}
	inst := &Installer{
		registry: nil,
		profiles: nil,
		tools:    allTools,
		input:    input,
		output:   output,
	}

	target1 := filepath.Join(base, "target1")
	target2 := filepath.Join(base, "target2")
	link := filepath.Join(base, "links", "test")
	os.MkdirAll(target1, 0755)
	os.MkdirAll(target2, 0755)
	os.MkdirAll(filepath.Dir(link), 0755)
	os.Symlink(target1, link)

	ok, err := inst.ensureSymlink(target2, link)
	if err != nil {
		t.Fatalf("ensureSymlink with replace failed: %v", err)
	}
	if !ok {
		t.Error("Should return true when user accepts replacement")
	}
}

// getTestToolsWithProjectDir 返回带 ProjectSkillDir 的工具集，用于验证
// 项目 scope（D5 默认）落地。
func getTestToolsWithProjectDir(base string) []tool.Tool {
	return []tool.Tool{
		{Name: "codex", SkillDir: filepath.Join(base, ".codex", "skills"), ProjectSkillDir: filepath.Join(base, ".agents", "skills")},
		{Name: "claude", SkillDir: filepath.Join(base, ".claude", "skills"), ProjectSkillDir: filepath.Join(base, ".claude", "skills")},
	}
}

// TestInstallFromRegistrySingleMatch 验证 Registry Install 单匹配直接 symlink
// 到项目 scope 目录。
func TestInstallFromRegistrySingleMatch(t *testing.T) {
	registryDir, profilesDir, _, _, _ := setupTestEnv(t)
	base := filepath.Dir(registryDir)
	tools := getTestToolsWithProjectDir(base)

	inst, err := New(registryDir, profilesDir, tools)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// 默认项目 scope（不调 SetScope）。
	result, err := inst.InstallFromRegistry([]string{"cf-skill"}, "")
	if err != nil {
		t.Fatalf("InstallFromRegistry: %v", err)
	}
	if len(result.Skills) == 0 {
		t.Fatal("expected at least one install, got 0")
	}
	// 应落到项目级 .claude/skills/cf-skill（claude 的 ProjectSkillDir）。
	claudeProj := filepath.Join(base, ".claude", "skills", "cf-skill")
	if _, err := os.Lstat(claudeProj); err != nil {
		t.Errorf("project-scope symlink missing at %s: %v", claudeProj, err)
	}
}

// TestInstallFromRegistryZeroMatchNoFallback 验证未注册的名报错且不 fallback。
func TestInstallFromRegistryZeroMatchNoFallback(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	base := filepath.Dir(registryDir)
	tools := getTestToolsWithProjectDir(base)

	inst, err := New(registryDir, profilesDir, tools)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inst.SetScope(projectDir, true)

	result, err := inst.InstallFromRegistry([]string{"does-not-exist"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Skills) != 0 {
		t.Errorf("zero match should install nothing, got %d", len(result.Skills))
	}
}

// TestInstallFromRegistryMultiMatchAmbiguous 验证同名跨多 category 时
// 报错要求 --category（不静默选首个）。
func TestInstallFromRegistryMultiMatchAmbiguous(t *testing.T) {
	registryDir, profilesDir, _, _, projectDir := setupTestEnv(t)
	base := filepath.Dir(registryDir)
	tools := getTestToolsWithProjectDir(base)

	// 造一个同名跨 global + codex-only 的歧义。
	os.MkdirAll(filepath.Join(registryDir, "skills", "global", "dup"), 0755)
	os.WriteFile(filepath.Join(registryDir, "skills", "global", "dup", "SKILL.md"), []byte("# dup-g"), 0644)
	os.MkdirAll(filepath.Join(registryDir, "skills", "codex-only", "dup"), 0755)
	os.WriteFile(filepath.Join(registryDir, "skills", "codex-only", "dup", "SKILL.md"), []byte("# dup-c"), 0644)

	inst, err := New(registryDir, profilesDir, tools)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inst.SetScope(projectDir, true)

	result, err := inst.InstallFromRegistry([]string{"dup"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Skills) != 0 {
		t.Errorf("ambiguous match should install nothing, got %d", len(result.Skills))
	}

	// 带 --category 选定后应成功装 codex-only 那份。
	result2, err := inst.InstallFromRegistry([]string{"dup"}, "codex-only")
	if err != nil {
		t.Fatalf("InstallFromRegistry with category: %v", err)
	}
	if len(result2.Skills) == 0 {
		t.Error("disambiguated install should land at least one link")
	}
}

// TestInstallFromRegistryCopyCarriesOrigin 验证 --copy 产出的 Copy Install
// 实体自带 .sm-origin.json（D4 前置）：从 registry 原件整体拷贝时带上。
func TestInstallFromRegistryCopyCarriesOrigin(t *testing.T) {
	registryDir, profilesDir, _, _, _ := setupTestEnv(t)
	base := filepath.Dir(registryDir)
	tools := getTestToolsWithProjectDir(base)

	// 给 registry 原件写一个 .sm-origin.json。
	originPath := filepath.Join(registryDir, "skills", "global", "global-skill", ".sm-origin.json")
	os.WriteFile(originPath, []byte(`{"source":"https://github.com/x/y","rel_path":"."}`), 0644)

	inst, err := New(registryDir, profilesDir, tools)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inst.SetCopyMode(true) // 默认项目 scope

	if _, err := inst.InstallFromRegistry([]string{"global-skill"}, ""); err != nil {
		t.Fatalf("InstallFromRegistry copy: %v", err)
	}
	// 拷贝实体应落到项目级 .claude/skills/global-skill 且带 origin。
	cp := filepath.Join(base, ".claude", "skills", "global-skill", ".sm-origin.json")
	if _, err := os.Stat(cp); err != nil {
		t.Errorf("copy install entity should carry .sm-origin.json at %s: %v", cp, err)
	}
}

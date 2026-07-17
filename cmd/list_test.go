package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

func TestWriteRegistryListCanFilterSkillsOnly(t *testing.T) {
	reg := setupListRegistry(t)
	var out bytes.Buffer

	if err := writeRegistryList(&out, reg, true, false); err != nil {
		t.Fatalf("writeRegistryList failed: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "SKILLS:") {
		t.Fatalf("Expected skills section, got:\n%s", text)
	}
	if strings.Contains(text, "MCP:") {
		t.Fatalf("Did not expect MCP section, got:\n%s", text)
	}
	if !strings.Contains(text, "cloudflare") || !strings.Contains(text, "global") {
		t.Fatalf("Expected skill categories, got:\n%s", text)
	}
}

func TestWriteRegistryListCanFilterMCPOnly(t *testing.T) {
	reg := setupListRegistry(t)
	var out bytes.Buffer

	if err := writeRegistryList(&out, reg, false, true); err != nil {
		t.Fatalf("writeRegistryList failed: %v", err)
	}

	text := out.String()
	if strings.Contains(text, "SKILLS:") {
		t.Fatalf("Did not expect skills section, got:\n%s", text)
	}
	if !strings.Contains(text, "MCP:") {
		t.Fatalf("Expected MCP section, got:\n%s", text)
	}
	if !strings.Contains(text, "github") {
		t.Fatalf("Expected MCP entry, got:\n%s", text)
	}
	// 新增 transport 列：github server 走 stdio（command 字段存在）。
	if !strings.Contains(text, "stdio") {
		t.Fatalf("Expected stdio transport, got:\n%s", text)
	}
}

func setupListRegistry(t *testing.T) *registry.Registry {
	t.Helper()

	dir := t.TempDir()
	for _, path := range []string{
		filepath.Join(dir, "skills", "global", "base-skill"),
		filepath.Join(dir, "skills", "cloudflare", "worker-skill"),
		filepath.Join(dir, "mcp"),
	} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatalf("creating registry path: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp", "github.json"),
		[]byte(`{"mcpServers":{"github":{"command":"github-mcp","url":"https://api.github.com/mcp"}}}`), 0644); err != nil {
		t.Fatalf("writing mcp: %v", err)
	}

	return registry.New(dir)
}

// setupListGlobals 还原 list 的包级 flag，避免测试间污染。
func setupListGlobals(t *testing.T) {
	t.Helper()
	oldAgents, oldProject, oldDir := listAgents, listProject, listDir
	t.Cleanup(func() {
		listAgents, listProject, listDir = oldAgents, oldProject, oldDir
	})
}

// TestListByAgentGlobalScansHomeDir 验证默认（--project=false）扫全局目录。
func TestListByAgentGlobalScansHomeDir(t *testing.T) {
	setupListGlobals(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()

	registry := filepath.Join(t.TempDir(), "registry")
	oldRegistry := RegistryDir
	RegistryDir = registry
	t.Cleanup(func() { RegistryDir = oldRegistry })

	skill := makeRegistrySkill(t, registry, "global", "gskill")
	makeLink(t, filepath.Join(tmpHome, tool.Claude.SkillDir, "gskill"), skill)

	listAgents = []string{"claude"}
	listProject = false
	var out bytes.Buffer
	if err := listByAgent(&out); err != nil {
		t.Fatalf("listByAgent: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("gskill")) {
		t.Fatalf("expected gskill in output, got: %s", out.String())
	}
}

// TestListByAgentProjectScansProjectDir 验证 --project 扫项目级目录。
func TestListByAgentProjectScansProjectDir(t *testing.T) {
	setupListGlobals(t)
	t.Setenv("HOME", t.TempDir())
	home.ResetForTest()
	projectDir := t.TempDir()

	registry := filepath.Join(t.TempDir(), "registry")
	oldRegistry := RegistryDir
	RegistryDir = registry
	t.Cleanup(func() { RegistryDir = oldRegistry })

	skill := makeRegistrySkill(t, registry, "global", "pskill")
	// 仅在项目级放链接，全局不放
	makeLink(t, filepath.Join(projectDir, tool.Claude.ProjectSkillDir, "pskill"), skill)

	listAgents = []string{"claude"}
	listProject = true
	listDir = projectDir
	var out bytes.Buffer
	if err := listByAgent(&out); err != nil {
		t.Fatalf("listByAgent: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("pskill")) {
		t.Fatalf("expected pskill in project output, got: %s", out.String())
	}
}

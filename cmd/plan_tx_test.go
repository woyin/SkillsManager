package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/project"
)

func withFakeBinary(t *testing.T, name string) {
	t.Helper()
	bin := t.TempDir()
	p := filepath.Join(bin, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func runPlanApply(t *testing.T) (string, error) {
	t.Helper()
	planJSON = false
	planCheck = false
	planApply = true
	planProfile = ""
	planSkills = nil
	var buf bytes.Buffer
	oldOut := planOut
	planOut = &buf
	t.Cleanup(func() { planOut = oldOut })
	err := runPlanCommand()
	return buf.String(), err
}

func writeSmJSON(t *testing.T, projectDir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(projectDir, ".sm.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestPlanApplyFailsWhenRegistrySkillMissing(t *testing.T) {
	withFakeBinary(t, "claude")
	projectDir := setupPlanCommand(t)
	t.Cleanup(resetPlanFlags())
	writeSmJSON(t, projectDir, `{"skills":["ghost"]}`)
	planDir = projectDir

	out, err := runPlanApply(t)
	if err == nil {
		t.Fatalf("registry 不存在 ghost 时 apply 不得成功，输出:\n%s", out)
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("错误应指明缺失技能, got %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(projectDir, ".claude", "skills", "ghost")); statErr == nil {
		t.Errorf("不存在的技能不应产生实体")
	}
}

func TestPlanApplyStrictAdditionsZeroSideEffect(t *testing.T) {
	withFakeBinary(t, "claude")
	projectDir := setupPlanCommand(t)
	t.Cleanup(resetPlanFlags())
	mkRegistrySkill(t, "foo")
	writeSmJSON(t, projectDir, `{"skills":["foo","ghost"]}`)
	planDir = projectDir

	if _, err := runPlanApply(t); err == nil {
		t.Fatal("部分技能缺失必须返回错误")
	}
	link := filepath.Join(projectDir, ".claude", "skills", "foo")
	if _, statErr := os.Lstat(link); statErr == nil {
		t.Errorf("严格模式失败时不得残留 %s", link)
	}
	cfg, err := project.NewManager(projectDir).Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Curation != nil && cfg.Curation.IsOwned("claude", "foo") {
		t.Errorf("应用失败却登记 ownership: %+v", cfg.Curation.Managed)
	}
}

func TestPlanApplyBootstrapRecordsOwnership(t *testing.T) {
	withFakeBinary(t, "claude")
	projectDir := setupPlanCommand(t)
	t.Cleanup(resetPlanFlags())
	mkRegistrySkill(t, "foo")
	planDir = projectDir
	planJSON = false
	planCheck = false
	planApply = true
	planProfile = ""
	planSkills = []string{"foo"}

	var buf bytes.Buffer
	oldOut := planOut
	planOut = &buf
	defer func() { planOut = oldOut }()
	if err := runPlanCommand(); err != nil {
		t.Fatalf("apply bootstrap: %v (%s)", err, buf.String())
	}

	link := filepath.Join(projectDir, ".claude", "skills", "foo")
	if _, statErr := os.Lstat(link); statErr != nil {
		t.Errorf("期望 foo 安装为符号链接: %v", statErr)
	}
	cfg, err := project.NewManager(projectDir).Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Curation == nil || cfg.Curation.Baseline == nil || len(cfg.Curation.Baseline.Skills) != 1 || cfg.Curation.Baseline.Skills[0] != "foo" {
		t.Errorf("期望 baseline 记录 foo, got %+v", cfg.Curation)
	}
	if !cfg.Curation.IsOwned("claude", "foo") {
		t.Errorf("期望 foo 登记 owned, got %+v", cfg.Curation.Managed)
	}
}

func TestPlanApplyOwnedRemovalLeavesNoDanglingOwnership(t *testing.T) {
	withFakeBinary(t, "claude")
	projectDir := setupPlanCommand(t)
	t.Cleanup(resetPlanFlags())
	mkRegistrySkill(t, "foo")
	linkHost := filepath.Join(projectDir, ".claude", "skills")
	if err := os.MkdirAll(linkHost, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(RegistryDir, "skills", "global", "foo"), filepath.Join(linkHost, "foo")); err != nil {
		t.Fatal(err)
	}
	writeSmJSON(t, projectDir, `{"curation":{"managed":{"claude":["foo"]}}}`)
	planDir = projectDir

	if _, err := runPlanApply(t); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(linkHost, "foo")); !os.IsNotExist(statErr) {
		t.Errorf("owned 链接应被移除")
	}
	cfg, err := project.NewManager(projectDir).Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Curation != nil && cfg.Curation.IsOwned("claude", "foo") {
		t.Errorf("移除后 managed 应清理, got %+v", cfg.Curation.Managed)
	}
}

// TestPlanApplyBootstrapProfileMCPOnly：cmd 级真实路径——profile 只含 MCP 无技能时，
// bootstrap --apply 必须写入 .mcp.json（验证 MCP-only 不会被漏装）。
func TestPlanApplyBootstrapProfileMCPOnly(t *testing.T) {
	withFakeBinary(t, "claude")
	projectDir := setupPlanCommand(t)
	t.Cleanup(resetPlanFlags())
	if err := os.MkdirAll(ProfilesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(RegistryDir, "mcp"), 0755); err != nil {
		t.Fatal(err)
	}
	mcpBody := []byte("{\"mcpServers\":{\"ctx7\":{\"command\":\"npx\",\"args\":[\"-y\",\"ctx7\"]}}}")
	if err := os.WriteFile(filepath.Join(RegistryDir, "mcp", "ctx7.json"), mcpBody, 0644); err != nil {
		t.Fatal(err)
	}
	profileBody := []byte("{\"skills\":[],\"mcp\":[\"ctx7\"]}")
	if err := os.WriteFile(filepath.Join(ProfilesDir, "mcp-only.json"), profileBody, 0644); err != nil {
		t.Fatal(err)
	}
	planDir = projectDir
	planJSON = false
	planCheck = false
	planApply = true
	planProfile = "mcp-only"
	planSkills = nil

	var buf bytes.Buffer
	oldOut := planOut
	planOut = &buf
	defer func() { planOut = oldOut }()
	if err := runPlanCommand(); err != nil {
		t.Fatalf("bootstrap MCP-only apply: %v (%s)", err, buf.String())
	}

	mcpPath := filepath.Join(projectDir, ".mcp.json")
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("期望 .mcp.json 被写入: %v", err)
	}
	if !strings.Contains(string(data), "ctx7") {
		t.Errorf(".mcp.json 应含 ctx7，得到 %s", data)
	}
	cfg, err := project.NewManager(projectDir).Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Profile != "mcp-only" {
		t.Errorf("顶层 profile 应为 mcp-only，得到 %q", cfg.Profile)
	}
	if cfg.Curation == nil || cfg.Curation.Baseline == nil || cfg.Curation.Baseline.Profile != "mcp-only" {
		t.Errorf("baseline 应记录 profile=mcp-only，得到 %+v", cfg.Curation)
	}
	if cfg.Curation != nil && cfg.Curation.Baseline != nil && len(cfg.Curation.Baseline.Skills) != 0 {
		t.Errorf("baseline.Skills 不应含 profile 展开成员，得到 %v", cfg.Curation.Baseline.Skills)
	}
}

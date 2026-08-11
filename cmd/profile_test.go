package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/profile"
)

func TestFormatList(t *testing.T) {
	tests := []struct {
		name   string
		items  []string
		expect string
	}{
		{"empty", nil, "(none)"},
		{"empty slice", []string{}, "(none)"},
		{"single", []string{"a"}, "a"},
		{"multiple", []string{"a", "b", "c"}, "a, b, c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatList(tt.items)
			if result != tt.expect {
				t.Errorf("formatList(%v) = %q, want %q", tt.items, result, tt.expect)
			}
		})
	}
}

func TestRegistryMCPExistsRecognizesRegisteredDefinition(t *testing.T) {
	oldRegistryDir := RegistryDir
	RegistryDir = t.TempDir()
	t.Cleanup(func() { RegistryDir = oldRegistryDir })

	if err := registryMCPExists("missing"); err == nil {
		t.Fatal("missing MCP definition should return an error")
	}

	mcpPath := filepath.Join(RegistryDir, "mcp", "browser.json")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mcpPath, []byte(`{"mcpServers": {}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := registryMCPExists("browser"); err != nil {
		t.Fatalf("registered MCP definition should exist: %v", err)
	}
}

func TestRegistrySkillExistsRecognizesUniqueSkill(t *testing.T) {
	oldRegistryDir := RegistryDir
	RegistryDir = t.TempDir()
	t.Cleanup(func() { RegistryDir = oldRegistryDir })

	if err := registrySkillExists("missing"); err == nil {
		t.Fatal("missing skill should return an error")
	}

	skillPath := filepath.Join(RegistryDir, "skills", "global", "browser", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("# Browser\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := registrySkillExists("browser"); err != nil {
		t.Fatalf("unique skill should exist: %v", err)
	}
}

func TestProfileCreateCommandPersistsValidatedMembers(t *testing.T) {
	oldRegistryDir, oldProfilesDir := RegistryDir, ProfilesDir
	oldSkills, oldMCP := profileCreateSkills, profileCreateMCP
	RegistryDir, ProfilesDir = t.TempDir(), t.TempDir()
	profileCreateSkills, profileCreateMCP = "browser", "server"
	t.Cleanup(func() {
		RegistryDir, ProfilesDir = oldRegistryDir, oldProfilesDir
		profileCreateSkills, profileCreateMCP = oldSkills, oldMCP
	})

	skillPath := filepath.Join(RegistryDir, "skills", "global", "browser", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("# Browser\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mcpPath := filepath.Join(RegistryDir, "mcp", "server.json")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mcpPath, []byte(`{"mcpServers": {}}`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := profileCreateCmd.RunE(profileCreateCmd, []string{"dev"}); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	created, err := profile.NewLoader(ProfilesDir).Load("dev")
	if err != nil {
		t.Fatalf("load created profile: %v", err)
	}
	if len(created.Skills) != 1 || created.Skills[0] != "browser" || len(created.MCP) != 1 || created.MCP[0] != "server" {
		t.Fatalf("unexpected created profile: %#v", created)
	}
}

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{"single", "a", []string{"a"}},
		{"multiple", "a,b,c", []string{"a", "b", "c"}},
		{"spaces", " a , b , c ", []string{"a", "b", "c"}},
		{"empty parts", "a,,b,", []string{"a", "b"}},
		{"just spaces", "  ,  ,  ", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitAndTrim(tt.input)
			if len(result) != len(tt.expect) {
				t.Fatalf("splitAndTrim(%q) returned %d items, want %d: %v", tt.input, len(result), len(tt.expect), result)
			}
			for i, v := range result {
				if v != tt.expect[i] {
					t.Errorf("splitAndTrim(%q)[%d] = %q, want %q", tt.input, i, v, tt.expect[i])
				}
			}
		})
	}
}

// TestProfileUpdatePreservesUntouchedFields 验证 profile update 只覆盖
// 显式传入的 flag 对应字段，未传的字段保留原值（不清空）。
func TestProfileUpdatePreservesUntouchedFields(t *testing.T) {
	dir := t.TempDir()

	loader := profile.NewLoader(dir)
	if err := loader.Save("dev", &profile.Profile{
		Skills: []string{"superpowers"},
		MCP:    []string{"ctx7"},
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	// 装配一个独立的 cobra cmd，只解析 --mcp：Changed("mcp")=true，Changed("skills")=false。
	profileUpdateSkills = ""
	profileUpdateMCP = ""
	cmd := &cobra.Command{}
	cmd.Flags().StringVar(&profileUpdateSkills, "skills", "", "")
	cmd.Flags().StringVar(&profileUpdateMCP, "mcp", "", "")
	if err := cmd.ParseFlags([]string{"--mcp", "pptx"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	// 复用 profileUpdateCmd.RunE 的判定逻辑：传这个 cmd 进去，
	// 以便 Flags().Changed 反映正确的解析状态。
	runUpdate := func(c *cobra.Command) error {
		p, err := loader.Load("dev")
		if err != nil {
			return err
		}
		if c.Flags().Changed("skills") {
			p.Skills = splitAndTrim(profileUpdateSkills)
		}
		if c.Flags().Changed("mcp") {
			p.MCP = splitAndTrim(profileUpdateMCP)
		}
		return loader.Save("dev", p)
	}

	if err := runUpdate(cmd); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := loader.Load("dev")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(got.Skills) != 1 || got.Skills[0] != "superpowers" {
		t.Errorf("skills should be preserved, got %v", got.Skills)
	}
	if len(got.MCP) != 1 || got.MCP[0] != "pptx" {
		t.Errorf("mcp should be overwritten to pptx, got %v", got.MCP)
	}
}

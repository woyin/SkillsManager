// cmd/use_test.go
package cmd

import (
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUseCmdRegistered(t *testing.T) {
	// Verify the use command is registered
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "use <source>" {
			found = true
			break
		}
	}
	if !found {
		t.Error("use command not registered")
	}
}

func TestUseCmdFlags(t *testing.T) {
	// Verify flags exist
	var cmd *cobra.Command
	for _, c := range rootCmd.Commands() {
		if c.Name() == "use" {
			cmd = c
			break
		}
	}
	if cmd == nil {
		t.Fatal("use command not found")
	}

	flags := []string{"skill", "agent"}
	for _, f := range flags {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("expected flag %q on use command", f)
		}
	}
}
func TestUseCmdFlagsFullDepth(t *testing.T) {
	var cmd *cobra.Command
	for _, c := range rootCmd.Commands() {
		if c.Name() == "use" {
			cmd = c
			break
		}
	}
	if cmd == nil {
		t.Fatal("use command not found")
	}
	if cmd.Flags().Lookup("full-depth") == nil {
		t.Error("expected --full-depth flag on use command")
	}
}

func TestBuildUsePromptWrapsSkillMD(t *testing.T) {
	got := buildUsePrompt("# Title\nbody", "", false)
	if !strings.Contains(got, "<SKILL.md>\n# Title\nbody\n</SKILL.md>") {
		t.Errorf("prompt missing <SKILL.md> wrapper: %s", got)
	}
	if !strings.HasPrefix(got, "You are being given a Skill") {
		t.Errorf("prompt missing header: %s", got)
	}
	if strings.Contains(got, "Supporting files") {
		t.Errorf("prompt should not mention supporting files when none: %s", got)
	}
}

func TestBuildUsePromptWithSupportingFiles(t *testing.T) {
	got := buildUsePrompt("# x", "/tmp/skills-use-xx", true)
	if !strings.Contains(got, "Supporting files for this skill were downloaded to:\n/tmp/skills-use-xx") {
		t.Errorf("prompt missing supporting-files notice: %s", got)
	}
}

func TestDirHasSupportingFiles(t *testing.T) {
	dir := t.TempDir()
	// only SKILL.md → no supporting files
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# x"), 0644)
	if dirHasSupportingFiles(dir) {
		t.Error("expected false with only SKILL.md")
	}
	// add a supporting file → true
	os.WriteFile(filepath.Join(dir, "refs.txt"), []byte("data"), 0644)
	if !dirHasSupportingFiles(dir) {
		t.Error("expected true with a supporting file")
	}
	// case-insensitive SKILL.md alone → false
	dir2 := t.TempDir()
	os.WriteFile(filepath.Join(dir2, "skill.md"), []byte("# x"), 0644)
	if dirHasSupportingFiles(dir2) {
		t.Error("expected false with only skill.md (case-insensitive)")
	}
}

func TestRunUseLocalSkill(t *testing.T) {
	// Create a temp skill
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "my-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: my-skill
description: Test skill
---
# Test Skill
Hello world
`), 0644)

	// Test that use can read the local skill
	err := runUse(skillDir)
	if err != nil {
		t.Errorf("use local skill failed: %v", err)
	}
}

func TestCheckOpenClawRisk(t *testing.T) {
	saved := useAcceptOpenClawRisks
	t.Cleanup(func() { useAcceptOpenClawRisks = saved })

	tests := []struct {
		name    string
		source  string
		accept  bool
		wantErr bool
	}{
		{"shorthand blocked", "openclaw/some-skill", false, true},
		{"shorthand accepted", "openclaw/some-skill", true, false},
		{"url blocked", "https://github.com/openclaw/repo", false, true},
		{"url accepted", "https://github.com/openclaw/repo", true, false},
		{"non-openclaw ok", "vercel-labs/agent-skills", false, false},
		{"case-insensitive", "OpenClaw/some-skill", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useAcceptOpenClawRisks = tt.accept
			err := checkOpenClawRisk(tt.source)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkOpenClawRisk(%q) accept=%v: err=%v, wantErr=%v", tt.source, tt.accept, err, tt.wantErr)
			}
		})
	}
}

// cmd/use_test.go
package cmd

import (
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
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

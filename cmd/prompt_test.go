// cmd/prompt_test.go
package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestPromptCmdRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "prompt" {
			found = true
			break
		}
	}
	if !found {
		t.Error("prompt command not registered on root command")
	}
}

func TestPromptSubcommandsRegistered(t *testing.T) {
	// Find the prompt command
	var promptCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "prompt" {
			promptCmd = cmd
			break
		}
	}
	if promptCmd == nil {
		t.Fatal("prompt command not found")
	}

	expectedSubcommands := []string{"list", "show", "apply", "create", "delete"}
	for _, name := range expectedSubcommands {
		found := false
		for _, cmd := range promptCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("prompt subcommand %q not registered", name)
		}
	}
}

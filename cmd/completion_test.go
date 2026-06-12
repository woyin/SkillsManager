package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCompletionBash(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	buf := new(bytes.Buffer)

	err := cmd.GenBashCompletionV2(buf, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "complete -o default -F") {
		t.Errorf("bash completion should contain 'complete -o default -F', got: %s", output[:min(200, len(output))])
	}
}

func TestCompletionZsh(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	buf := new(bytes.Buffer)

	err := cmd.GenZshCompletion(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "#compdef") {
		t.Errorf("zsh completion should contain '#compdef', got: %s", output[:min(200, len(output))])
	}
}

func TestCompletionFish(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	buf := new(bytes.Buffer)

	err := cmd.GenFishCompletion(buf, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "complete -c test") {
		t.Errorf("fish completion should contain 'complete -c test', got: %s", output[:min(200, len(output))])
	}
}

func TestCompletionPowerShell(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	buf := new(bytes.Buffer)

	err := cmd.GenPowerShellCompletionWithDesc(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Register-ArgumentCompleter") {
		t.Errorf("powershell completion should contain 'Register-ArgumentCompleter', got: %s", output[:min(200, len(output))])
	}
}

func TestCompletionCmdRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "completion" {
			found = true
			break
		}
	}
	if !found {
		t.Error("completion command not registered on root command")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

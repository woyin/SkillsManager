// cmd/backup_test.go
package cmd

import (
	"testing"
)

func TestBackupCmdRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "backup" {
			found = true
			break
		}
	}
	if !found {
		t.Error("backup command not registered on root command")
	}
}

func TestRestoreCmdRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "restore" {
			found = true
			break
		}
	}
	if !found {
		t.Error("restore command not registered on root command")
	}
}

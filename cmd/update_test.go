package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateGitReposWalksSkillsAndMCP(t *testing.T) {
	registryDir := t.TempDir()
	skillRepo := filepath.Join(registryDir, "skills", "cloudflare", "workers")
	mcpRepo := filepath.Join(registryDir, "mcp", "browser")
	for _, repo := range []string{skillRepo, mcpRepo} {
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0755); err != nil {
			t.Fatalf("creating fake repo: %v", err)
		}
	}

	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "git.log")
	fakeGit := filepath.Join(binDir, "git")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + logPath + "\nexit 0\n"
	if err := os.WriteFile(fakeGit, []byte(script), 0755); err != nil {
		t.Fatalf("writing fake git: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	summary, err := updateGitRepos(registryDir)
	if err != nil {
		t.Fatalf("updateGitRepos failed: %v", err)
	}
	if summary.Updated != 2 {
		t.Fatalf("Expected 2 updated repos, got %+v", summary)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading fake git log: %v", err)
	}
	log := string(logData)
	for _, repo := range []string{skillRepo, mcpRepo} {
		if !strings.Contains(log, "-C "+repo+" pull --ff-only") {
			t.Fatalf("Expected git pull for %s, log was:\n%s", repo, log)
		}
	}
}

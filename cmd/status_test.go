package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/tool"
)

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		name   string
		input  int64
		expect string
	}{
		{"zero", 0, "0"},
		{"small", 42, "42"},
		{"thousands", 1500, "1.5K"},
		{"millions", 2500000, "2.5M"},
		{"billions", 3000000000, "3.0B"},
		{"exact thousand", 1000, "1.0K"},
		{"exact million", 1000000, "1.0M"},
		{"exact billion", 1000000000, "1.0B"},
		{"under thousand", 999, "999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTokenCount(tt.input)
			if result != tt.expect {
				t.Errorf("formatTokenCount(%d) = %q, want %q", tt.input, result, tt.expect)
			}
		})
	}
}

func TestWriteProjectStatusShowsProjectInstallAndOrphan(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()

	oldReg := RegistryDir
	RegistryDir = filepath.Join(t.TempDir(), "registry")
	t.Cleanup(func() { RegistryDir = oldReg })
	if err := os.MkdirAll(filepath.Join(RegistryDir, "skills", "global", "orphan"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(RegistryDir, "skills", "global", "orphan", "SKILL.md"),
		[]byte("---\nname: orphan\ndescription: long enough description here\n---\n# o\n"), 0644); err != nil {
		t.Fatal(err)
	}

	projectDir := t.TempDir()
	linkDir := filepath.Join(projectDir, tool.Claude.ProjectSkillDir)
	if err := os.MkdirAll(linkDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(RegistryDir, "skills", "global", "orphan"), filepath.Join(linkDir, "orphan")); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := writeProjectStatus(&buf, projectDir); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "orphan") {
		t.Fatalf("expected orphan in status:\n%s", out)
	}
	if !strings.Contains(out, "Issues:") {
		t.Fatalf("expected Issues section:\n%s", out)
	}
	if !strings.Contains(out, "INSTALLED (project)") {
		t.Fatalf("expected project section:\n%s", out)
	}
}

func TestWriteProjectStatusReportsBrokenSymlink(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()

	projectDir := t.TempDir()
	linkDir := filepath.Join(projectDir, tool.Claude.ProjectSkillDir)
	if err := os.MkdirAll(linkDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing-target"), filepath.Join(linkDir, "gone")); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := writeProjectStatus(&buf, projectDir); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "broken") || !strings.Contains(out, "gone") {
		t.Fatalf("expected broken issue:\n%s", out)
	}
}

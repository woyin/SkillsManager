package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/registry"
)

func TestIsBareName(t *testing.T) {
	cases := []struct {
		arg string
		ok  bool
	}{
		{"my-skill", true},
		{"foo", true},
		{"owner/repo", false},
		{"./local", false},
		{"/abs/path", false},
		{"https://x.com/r", false},
		{"git@h:o/r.git", false},
		{"foo/bar/baz", false},
		{"", false},
	}
	for _, c := range cases {
		got := registry.IsBareName(c.arg)
		if got != c.ok {
			t.Errorf("IsBareName(%q) = %v, want %v", c.arg, got, c.ok)
		}
	}
}

func withInstallDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := installDir
	installDir = dir
	t.Cleanup(func() { installDir = old })
	return dir
}

func TestInstallBareNameFromRegistry(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	regDir := withTestRegistry(t)
	projectDir := withInstallDir(t)

	src := t.TempDir()
	makeValidSkillDir(t, src, "deployable")
	reg := registry.New(regDir)
	if _, err := reg.Register(filepath.Join(src, "deployable"), "",
		registry.SkillOrigin{SourceKind: registry.SourceLocalSnapshot, Source: src}, false); err != nil {
		t.Fatal(err)
	}

	if err := installFromRegistry("deployable"); err != nil {
		t.Fatalf("installFromRegistry: %v", err)
	}

	link := filepath.Join(projectDir, ".claude/skills/deployable")
	target, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatalf("expected symlink at %s: %v", link, err)
	}
	regOriginal, _ := filepath.EvalSymlinks(filepath.Join(regDir, "skills", "global", "deployable"))
	if target != regOriginal {
		t.Errorf("symlink target = %s, want registry %s", target, regOriginal)
	}
}

func TestInstallBareNameHonorsExplicitAgents(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	withTestRegistry(t)
	projectDir := withInstallDir(t)

	src := t.TempDir()
	makeValidSkillDir(t, src, "demo-skill")
	reg := registry.New(RegistryDir)
	if _, err := reg.Register(filepath.Join(src, "demo-skill"), "",
		registry.SkillOrigin{SourceKind: registry.SourceLocalSnapshot, Source: src}, false); err != nil {
		t.Fatal(err)
	}

	oldAgents := installAgents
	installAgents = []string{"codex", "gemini"}
	t.Cleanup(func() { installAgents = oldAgents })

	if err := installFromRegistry("demo-skill"); err != nil {
		t.Fatalf("installFromRegistry: %v", err)
	}

	shared := filepath.Join(projectDir, ".agents", "skills", "demo-skill")
	if _, err := os.Lstat(shared); err != nil {
		t.Fatalf("expected shared project symlink at %s: %v", shared, err)
	}
	for _, unexpected := range []string{
		filepath.Join(projectDir, ".claude", "skills", "demo-skill"),
		filepath.Join(projectDir, ".codex", "skills", "demo-skill"),
		filepath.Join(projectDir, ".gemini", "skills", "demo-skill"),
	} {
		if _, err := os.Lstat(unexpected); !os.IsNotExist(err) {
			t.Fatalf("explicit agent selection leaked to %s (stat error=%v)", unexpected, err)
		}
	}
}

func TestInstallUnknownBareNameFailsWithHint(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	withTestRegistry(t)
	withInstallDir(t)

	err := installFromRegistry("does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown bare name")
	}
	if !containsSubstr(err.Error(), "sm add") {
		t.Errorf("error should hint sm add, got: %v", err)
	}
}

func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

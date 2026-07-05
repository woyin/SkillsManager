package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLintCommandOutput(t *testing.T) {
	regDir := t.TempDir()
	mkSkill := func(category, name, content string) {
		dir := filepath.Join(regDir, "skills", category, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// good：合规 + 充实 → 高分，✓
	mkSkill("global", "good",
		"---\nname: good\ndescription: A sufficiently long and clear description here.\n---\n"+
			"## Usage\n"+strings.Repeat("step. ", 40)+"\n## Examples\n"+strings.Repeat("ex. ", 40)+"\n## Notes\nok")
	// shorty：description 过短 → 低分，⚠
	mkSkill("codex-only", "shorty",
		"---\nname: shorty\ndescription: meh\n---\n# X\n")
	// bad：缺 description → error，✗
	mkSkill("global", "bad",
		"---\nname: bad\n---\n# X\n")

	// 临时接管 rootCmd 的输出与全局 flag。
	RegistryDir = regDir
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"lint"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("lint failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"good", "shorty", "bad", "SCORE", "STATUS"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q, got:\n%s", want, got)
		}
	}
	// good 高分应有 ✓，bad 缺 description 应有 ✗。
	if !strings.Contains(got, "✓") {
		t.Errorf("expected ✓ for healthy skill, got:\n%s", got)
	}
	if !strings.Contains(got, "✗") {
		t.Errorf("expected ✗ for error skill, got:\n%s", got)
	}
	if !strings.Contains(got, "3 skills") {
		t.Errorf("expected 3 skills summary, got:\n%s", got)
	}
}

func TestLintStrictExitsNonZero(t *testing.T) {
	regDir := t.TempDir()
	dir := filepath.Join(regDir, "skills", "global", "broken")
	os.MkdirAll(dir, 0755)
	// 缺 name + description → errors。
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\n---\n# X\n"), 0644)

	RegistryDir = regDir
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"lint", "--strict"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected non-zero exit with --strict and error skills")
	}
}

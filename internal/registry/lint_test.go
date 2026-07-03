package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSkillMD 在 reg 的 category 下建 name 目录并写入 SKILL.md。
func writeSkillMDAt(t *testing.T, reg *Registry, category, name, content string) {
	t.Helper()
	dir := filepath.Join(reg.skillsDir(), category, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLintSkillCompliant(t *testing.T) {
	reg := New(t.TempDir())
	writeSkillMDAt(t, reg, "global", "good",
		"---\nname: good\ndescription: This is a sufficiently long description for the skill.\n---\n# Good\n")
	res := reg.LintSkill(filepath.Join("global", "good"))
	if len(res.Findings) != 0 {
		t.Errorf("expected no findings, got %+v", res.Findings)
	}
}

func TestLintSkillMissingFile(t *testing.T) {
	reg := New(t.TempDir())
	res := reg.LintSkill(filepath.Join("global", "ghost"))
	if !res.HasErrors() {
		t.Errorf("expected error for missing SKILL.md")
	}
}

func TestLintSkillMissingName(t *testing.T) {
	reg := New(t.TempDir())
	writeSkillMDAt(t, reg, "global", "noname",
		"---\ndescription: some description that is long enough for the rule.\n---\n")
	res := reg.LintSkill(filepath.Join("global", "noname"))
	if !containsMsg(res.Findings, "missing required field: name") {
		t.Errorf("expected missing-name error, got %+v", res.Findings)
	}
}

func TestLintSkillMissingDescription(t *testing.T) {
	reg := New(t.TempDir())
	writeSkillMDAt(t, reg, "global", "nodesc",
		"---\nname: nodesc\n---\n")
	res := reg.LintSkill(filepath.Join("global", "nodesc"))
	if !containsMsg(res.Findings, "missing required field: description") {
		t.Errorf("expected missing-description error, got %+v", res.Findings)
	}
}

func TestLintSkillShortDescription(t *testing.T) {
	reg := New(t.TempDir())
	writeSkillMDAt(t, reg, "global", "short",
		"---\nname: short\ndescription: too short\n---\n")
	res := reg.LintSkill(filepath.Join("global", "short"))
	if !containsMsg(res.Findings, "description too short") {
		t.Errorf("expected short-description warning, got %+v", res.Findings)
	}
}

func TestLintSkillInvalidName(t *testing.T) {
	reg := New(t.TempDir())
	writeSkillMDAt(t, reg, "global", "bad",
		"---\nname: Bad Name!\ndescription: some description that is long enough for the rule.\n---\n")
	res := reg.LintSkill(filepath.Join("global", "bad"))
	if !containsMsg(res.Findings, "outside [a-z0-9-]") {
		t.Errorf("expected invalid-name warning, got %+v", res.Findings)
	}
}

func TestIsValidSkillName(t *testing.T) {
	cases := map[string]bool{
		"good-name":   true,
		"name123":     true,
		"":            false,
		"Bad":         false,
		"with space":  false,
		"under_score": false,
	}
	for name, want := range cases {
		if got := isValidSkillName(name); got != want {
			t.Errorf("isValidSkillName(%q) = %v, want %v", name, got, want)
		}
	}
}

func containsMsg(findings []LintFinding, substr string) bool {
	for _, f := range findings {
		if strings.Contains(f.Message, substr) {
			return true
		}
	}
	return false
}

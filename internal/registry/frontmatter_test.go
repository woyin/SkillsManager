// internal/registry/frontmatter_test.go
package registry

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSkillMD writes a SKILL.md with the given frontmatter body.
func writeSkillMD(t *testing.T, dir string, frontmatter string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := "---\n" + frontmatter + "\n---\n# Skill\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestParseFrontmatterDescription(t *testing.T) {
	dir := t.TempDir()
	writeSkillMD(t, filepath.Join(dir, "s"), "name: s\ndescription: \"A test skill\"")
	if got := ParseFrontmatterDescription(filepath.Join(dir, "s", "SKILL.md")); got != "A test skill" {
		t.Errorf("description = %q, want %q", got, "A test skill")
	}
}

func TestParseFrontmatterName(t *testing.T) {
	dir := t.TempDir()
	writeSkillMD(t, filepath.Join(dir, "repo-name"), "name: actual-skill\ndescription: A test skill")

	fm := parseSkillFrontmatter(filepath.Join(dir, "repo-name", "SKILL.md"))
	if fm.Name != "actual-skill" {
		t.Errorf("name = %q, want %q", fm.Name, "actual-skill")
	}
}

func TestParseFrontmatterInternal(t *testing.T) {
	dir := t.TempDir()
	writeSkillMD(t, filepath.Join(dir, "s"), "name: s\ndescription: d\nmetadata:\n  internal: true")

	fm := parseSkillFrontmatter(filepath.Join(dir, "s", "SKILL.md"))
	if !fm.Internal {
		t.Error("expected Internal=true for metadata.internal: true")
	}
	if fm.Description != "d" {
		t.Errorf("description = %q, want %q", fm.Description, "d")
	}
}

func TestParseFrontmatterNotInternal(t *testing.T) {
	dir := t.TempDir()
	// A top-level `internal:` (not under metadata:) must not be treated as internal.
	writeSkillMD(t, filepath.Join(dir, "s"), "name: s\ndescription: d\ninternal: true")
	fm := parseSkillFrontmatter(filepath.Join(dir, "s", "SKILL.md"))
	if fm.Internal {
		t.Error("top-level internal: should not set Internal flag")
	}
}

// TestDiscoverSkillsHidesInternal verifies internal skills are filtered out
// unless INSTALL_INTERNAL_SKILLS is set.
func TestDiscoverSkillsHidesInternal(t *testing.T) {
	src := t.TempDir()
	// Normal skill under skills/
	writeSkillMD(t, filepath.Join(src, "skills", "public-skill"),
		"name: public-skill\ndescription: visible")
	// Internal skill
	writeSkillMD(t, filepath.Join(src, "skills", "hidden-skill"),
		"name: hidden-skill\ndescription: secret\nmetadata:\n  internal: true")

	// Default: internal hidden
	t.Setenv("INSTALL_INTERNAL_SKILLS", "")
	got, err := DiscoverSkills(src)
	if err != nil {
		t.Fatal(err)
	}
	names := skillNames(got)
	if contains(names, "hidden-skill") {
		t.Errorf("internal skill should be hidden by default, got %v", names)
	}
	if !contains(names, "public-skill") {
		t.Errorf("public skill should be visible, got %v", names)
	}

	// With env var set: internal visible
	t.Setenv("INSTALL_INTERNAL_SKILLS", "1")
	got, err = DiscoverSkills(src)
	if err != nil {
		t.Fatal(err)
	}
	names = skillNames(got)
	if !contains(names, "hidden-skill") {
		t.Errorf("internal skill should be visible with INSTALL_INTERNAL_SKILLS=1, got %v", names)
	}
}

// TestDiscoverSkillsIncludeInternalOption verifies that the IncludeInternal
// discovery option surfaces internal skills even without the env var, matching
// npx skills' includeInternal selector semantics.
func TestDiscoverSkillsIncludeInternalOption(t *testing.T) {
	src := t.TempDir()
	writeSkillMD(t, filepath.Join(src, "skills", "public-skill"),
		"name: public-skill\ndescription: visible")
	writeSkillMD(t, filepath.Join(src, "skills", "hidden-skill"),
		"name: hidden-skill\ndescription: secret\nmetadata:\n  internal: true")

	// Env var unset; IncludeInternal option overrides the filter.
	t.Setenv("INSTALL_INTERNAL_SKILLS", "")
	got, err := DiscoverSkillsWithOptions(src, DiscoverOptions{IncludeInternal: true})
	if err != nil {
		t.Fatal(err)
	}
	names := skillNames(got)
	if !contains(names, "hidden-skill") {
		t.Errorf("IncludeInternal=true should surface internal skill, got %v", names)
	}
	if !contains(names, "public-skill") {
		t.Errorf("public skill should remain visible, got %v", names)
	}

	// Without the option, internal stays hidden (regression guard).
	got, err = DiscoverSkillsWithOptions(src, DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	names = skillNames(got)
	if contains(names, "hidden-skill") {
		t.Errorf("IncludeInternal=false (no env) should hide internal skill, got %v", names)
	}
}

func TestInternalSkillsVisibleEnvVariants(t *testing.T) {
	cases := map[string]bool{
		"":        false,
		"0":       false,
		"false":   false,
		"1":       true,
		"true":    true,
		"True":    true,
		"yes":     true,
		"garbage": false,
	}
	for v, want := range cases {
		t.Setenv("INSTALL_INTERNAL_SKILLS", v)
		if got := internalSkillsVisible(); got != want {
			t.Errorf("INSTALL_INTERNAL_SKILLS=%q -> visible=%v, want %v", v, got, want)
		}
	}
}

func skillNames(skills []DiscoveredSkill) []string {
	var out []string
	for _, s := range skills {
		out = append(out, s.Name)
	}
	return out
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// TestDiscoverSkillsAgentSpecificDir verifies skills in agent-specific
// container dirs (e.g. .grok/skills, .windsurf/skills) are discovered,
// aligning with npx skills AGENT_PROJECT_SKILL_DIRS.
func TestDiscoverSkillsAgentSpecificDir(t *testing.T) {
	for _, dir := range []string{".grok/skills", ".windsurf/skills", ".zcode/skills"} {
		t.Run(dir, func(t *testing.T) {
			src := t.TempDir()
			skillDir := filepath.Join(src, dir, "my-skill")
			writeSkillMD(t, skillDir, `---
name: my-skill
description: test
`)
			got, err := DiscoverSkills(src)
			if err != nil {
				t.Fatalf("DiscoverSkills: %v", err)
			}
			if !contains(skillNames(got), "my-skill") {
				t.Errorf("skill in %s not discovered; got %v", dir, skillNames(got))
			}
		})
	}
}

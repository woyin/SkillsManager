// internal/registry/source_test.go
package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsGitURLExtended(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		// GitHub shorthand
		{"owner/repo", true},
		{"vercel-labs/agent-skills", true},
		// Full GitHub URL
		{"https://github.com/owner/repo", true},
		{"https://github.com/vercel-labs/agent-skills", true},
		// Tree URLs
		{"https://github.com/owner/repo/tree/main/skills/my-skill", true},
		// GitLab
		{"https://gitlab.com/org/repo", true},
		// SSH
		{"git@github.com:owner/repo.git", true},
		// .git suffix
		{"https://example.com/repo.git", true},
		// Local paths (not git)
		{"./my-skill", false},
		{"/absolute/path", false},
		{"my-local-dir", false},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			got := IsGitURL(tt.source)
			if got != tt.want {
				t.Errorf("IsGitURL(%q) = %v, want %v", tt.source, got, tt.want)
			}
		})
	}
}

func TestParseTreeURL(t *testing.T) {
	tests := []struct {
		source      string
		wantRepo    string
		wantBranch  string
		wantSubPath string
		wantOK      bool
	}{
		{
			"https://github.com/owner/repo/tree/main/skills/my-skill",
			"https://github.com/owner/repo", "main", "skills/my-skill", true,
		},
		{
			"https://github.com/owner/repo/tree/main",
			"https://github.com/owner/repo", "main", "", true,
		},
		{
			"https://github.com/owner/repo",
			"https://github.com/owner/repo", "", "", true,
		},
		{
			"https://gitlab.com/org/repo/tree/dev/src/skills",
			"https://gitlab.com/org/repo", "dev", "src/skills", true,
		},
		{
			"owner/repo/tree/main/skills/web-design",
			"https://github.com/owner/repo", "main", "skills/web-design", true,
		},
		{
			"./local-path",
			"", "", "", false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			repo, branch, subPath, ok := ParseTreeURL(tt.source)
			if ok != tt.wantOK {
				t.Errorf("ParseTreeURL(%q) ok = %v, want %v", tt.source, ok, tt.wantOK)
				return
			}
			if ok {
				if repo != tt.wantRepo {
					t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
				}
				if branch != tt.wantBranch {
					t.Errorf("branch = %q, want %q", branch, tt.wantBranch)
				}
				if subPath != tt.wantSubPath {
					t.Errorf("subPath = %q, want %q", subPath, tt.wantSubPath)
				}
			}
		})
	}
}

func TestNormalizeGitURLEnhanced(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{"owner/repo", "https://github.com/owner/repo"},
		{"https://github.com/owner/repo", "https://github.com/owner/repo"},
		{"git@github.com:owner/repo.git", "git@github.com:owner/repo.git"},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			got := NormalizeGitURL(tt.source)
			if got != tt.want {
				t.Errorf("NormalizeGitURL(%q) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}

func TestSkillNameFromPathEnhanced(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{"https://github.com/owner/repo", "repo"},
		{"https://github.com/owner/repo/tree/main/skills/my-skill", "my-skill"},
		{"owner/repo", "repo"},
		{"./my-skill", "my-skill"},
		{"git@github.com:owner/my-skill.git", "my-skill"},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			got := SkillNameFromPath(tt.source)
			if got != tt.want {
				t.Errorf("SkillNameFromPath(%q) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}

func TestDiscoverSkills(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a skill in skills/ subdirectory
	skillDir := filepath.Join(tmpDir, "skills", "web-design")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: web-design
description: Web design guidelines
---
# Web Design
`), 0644)

	// Create another skill in skills/ subdirectory
	skill2Dir := filepath.Join(tmpDir, "skills", "typescript-best-practices")
	os.MkdirAll(skill2Dir, 0755)
	os.WriteFile(filepath.Join(skill2Dir, "SKILL.md"), []byte(`---
name: typescript-best-practices
description: TypeScript coding standards
---
# TypeScript
`), 0644)

	skills, err := DiscoverSkills(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverSkills error: %v", err)
	}

	if len(skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(skills))
		for _, s := range skills {
			t.Logf("  found: %s (%s)", s.Name, s.Description)
		}
	}

	names := make(map[string]bool)
	for _, s := range skills {
		names[s.Name] = true
	}

	if !names["web-design"] {
		t.Error("web-design skill not found")
	}
	if !names["typescript-best-practices"] {
		t.Error("typescript-best-practices skill not found")
	}
}

func TestDiscoverSkillsRootLevel(t *testing.T) {
	tmpDir := t.TempDir()

	// Create SKILL.md at root
	os.WriteFile(filepath.Join(tmpDir, "SKILL.md"), []byte(`---
name: root-skill
description: A root-level skill
---
# Root Skill
`), 0644)

	skills, err := DiscoverSkills(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverSkills error: %v", err)
	}

	if len(skills) < 1 {
		t.Error("expected at least 1 skill from root SKILL.md")
	}
}

func TestDiscoverSkillsCatalogLayout(t *testing.T) {
	tmpDir := t.TempDir()

	// Create catalog layout: skills/category/name/SKILL.md
	skillDir := filepath.Join(tmpDir, "skills", "frontend", "css-patterns")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: css-patterns
description: CSS best practices
---
# CSS Patterns
`), 0644)

	skills, err := DiscoverSkills(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverSkills error: %v", err)
	}

	found := false
	for _, s := range skills {
		if s.Name == "css-patterns" {
			found = true
			break
		}
	}
	if !found {
		t.Error("catalog layout skill 'css-patterns' not found")
	}
}

func TestDiscoverSkillsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	skills, err := DiscoverSkills(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverSkills error: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills from empty dir, got %d", len(skills))
	}
}

func TestParseSourceShorthandWithSkillFilter(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantURL   string
		wantRef   string
		wantSkill string
		wantLocal bool
	}{
		{"plain shorthand", "owner/repo", "https://github.com/owner/repo", "", "", false},
		{"at-skill", "owner/repo@my-skill", "https://github.com/owner/repo", "", "my-skill", false},
		{"hash-branch", "owner/repo#main", "https://github.com/owner/repo", "main", "", false},
		{"hash-branch-at-skill", "owner/repo#main@my-skill", "https://github.com/owner/repo", "main", "my-skill", false},
		{"github-prefix", "github:owner/repo", "https://github.com/owner/repo", "", "", false},
		{"gitlab-prefix", "gitlab:org/repo", "https://gitlab.com/org/repo", "", "", false},
		{"tree-url", "owner/repo/tree/main/skills/foo", "https://github.com/owner/repo", "main", "", false},
		{"local", "./skills", "", "", "", true},
		{"full-url", "https://github.com/owner/repo.git", "https://github.com/owner/repo", "", "", false},
		// URL-encoded fragment ref (e.g. branch with slash encoded as %2F).
		{"encoded-branch", "owner/repo#feat%2Fthing", "https://github.com/owner/repo", "feat/thing", "", false},
		{"encoded-branch-skill", "owner/repo#release%2Fv2@deploy", "https://github.com/owner/repo", "release/v2", "deploy", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := ParseSource(tt.input)
			if tt.wantLocal {
				if !ps.IsLocal {
					t.Errorf("expected IsLocal=true, got URL=%s", ps.URL)
				}
				return
			}
			if ps.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", ps.URL, tt.wantURL)
			}
			if ps.Ref != tt.wantRef {
				t.Errorf("Ref = %q, want %q", ps.Ref, tt.wantRef)
			}
			if ps.SkillFilter != tt.wantSkill {
				t.Errorf("SkillFilter = %q, want %q", ps.SkillFilter, tt.wantSkill)
			}
		})
	}
}

func TestSanitizeSubpath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"skills/my-skill", "skills/my-skill"},
		{"skills\\my-skill", "skills\\my-skill"},   // backslash not normalized in return, but checked
		{"../etc/passwd", ""},                      // traversal rejected
		{"skills/../etc", ""},                      // traversal in middle
		{"skills/./my-skill", "skills/./my-skill"}, // single dot OK
		{"a/b/c", "a/b/c"},
	}
	for _, tt := range tests {
		got := SanitizeSubpath(tt.input)
		if got != tt.want {
			t.Errorf("SanitizeSubpath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSanitizeMetadata(t *testing.T) {
	// ANSI escape sequences should be stripped
	evil := "\x1b[31mred\x1b[0m text"
	got := SanitizeMetadata(evil)
	if got != "red text" {
		t.Errorf("SanitizeMetadata(ANSI) = %q, want %q", got, "red text")
	}
	// Control characters stripped
	got = SanitizeMetadata("hello\x07bell")
	if got != "hellobell" {
		t.Errorf("SanitizeMetadata(bell) = %q, want %q", got, "hellobell")
	}
	// Newlines collapsed
	got = SanitizeMetadata("line1\nline2\r\nline3")
	if got != "line1 line2 line3" {
		t.Errorf("SanitizeMetadata(newlines) = %q, want %q", got, "line1 line2 line3")
	}
	// Normal text preserved
	got = SanitizeMetadata("normal skill name")
	if got != "normal skill name" {
		t.Errorf("SanitizeMetadata(normal) = %q, want %q", got, "normal skill name")
	}
}

// TestLooksLikeGitSource verifies the fragment-parsing gate so that a '#' is
// only treated as a ref delimiter for git-like sources.
func TestLooksLikeGitSource(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"github:owner/repo", true},
		{"gitlab:org/repo", true},
		{"git@github.com:owner/repo.git", true},
		{"https://github.com/owner/repo", true},
		{"https://github.com/owner/repo.git", true},
		{"owner/repo", true},
		{"owner/repo@skill", true},
		{"./local/path", false},
		{"/abs/path", false},
		{"plain string", false},
		{"hello#world", false},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			if got := looksLikeGitSource(c.input); got != c.want {
				t.Errorf("looksLikeGitSource(%q) = %v, want %v", c.input, got, c.want)
			}
		})
	}
}

// TestDecodeFragmentValue verifies URL-decoding of #fragment ref/skill values.
func TestDecodeFragmentValue(t *testing.T) {
	cases := map[string]string{
		"feat%2Fthing": "feat/thing",
		"main":         "main",
		"":             "",
		"100%25-done":  "100%-done",
		// Invalid percent-encoding falls back to raw value.
		"%ZZ": "%ZZ",
	}
	for in, want := range cases {
		if got := decodeFragmentValue(in); got != want {
			t.Errorf("decodeFragmentValue(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestParseSourceNonGitHashLiteral verifies that a '#' in a non-git source is
// kept literal (not parsed as a ref delimiter).
func TestParseSourceNonGitHashLiteral(t *testing.T) {
	// "hello#world" is not a git source and not a local path, so it falls
	// through to the raw-URL fallback with the '#' intact.
	ps := ParseSource("hello#world")
	if ps.IsLocal {
		t.Fatal("expected non-local")
	}
	// The fragment gate should have left the input intact.
	if ps.Ref != "" {
		t.Errorf("non-git '#' should not produce a ref, got Ref=%q", ps.Ref)
	}
}

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
			got := normalizeGitURL(tt.source)
			if got != tt.want {
				t.Errorf("normalizeGitURL(%q) = %q, want %q", tt.source, got, tt.want)
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

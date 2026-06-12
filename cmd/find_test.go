// cmd/find_test.go
package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindCmdRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "find" {
			found = true
			break
		}
	}
	if !found {
		t.Error("find command not registered")
	}
}

func TestMatchesQuery(t *testing.T) {
	tests := []struct {
		name    string
		desc    string
		query   string
		want    bool
	}{
		{"frontend-design", "Web design guidelines", "frontend", true},
		{"frontend-design", "Web design guidelines", "web", true},
		{"frontend-design", "Web design guidelines", "python", false},
		{"skill-a", "", "skill", true},
		{"skill-a", "", "SKILL", true}, // case insensitive
		{"my-skill", "A helpful skill", "", true},        // empty query matches all
		{"my-skill", "A helpful and useful skill", "help useful", true}, // multi-term
		{"my-skill", "A helpful skill", "help missing", false},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_"+tt.query, func(t *testing.T) {
			got := matchesQuery(tt.name, tt.desc, tt.query)
			if got != tt.want {
				t.Errorf("matchesQuery(%q, %q, %q) = %v, want %v", tt.name, tt.desc, tt.query, got, tt.want)
			}
		})
	}
}

func TestExtractDescription(t *testing.T) {
	tests := []struct {
		content string
		want    string
	}{
		{
			"---\nname: test\ndescription: Hello world\n---\n# Test",
			"Hello world",
		},
		{
			"---\nname: test\ndescription: \"Quoted description\"\n---\n",
			"Quoted description",
		},
		{
			"# No frontmatter",
			"",
		},
		{
			"",
			"",
		},
	}

	for i, tt := range tests {
		got := extractDescription(tt.content)
		if got != tt.want {
			t.Errorf("test %d: extractDescription() = %q, want %q", i, got, tt.want)
		}
	}
}

func TestCollectFindMatchesEmpty(t *testing.T) {
	// Set up a temporary registry dir
	tmpDir := t.TempDir()
	origDir := RegistryDir
	RegistryDir = tmpDir
	defer func() { RegistryDir = origDir }()

	matches, err := collectFindMatches("")
	if err != nil {
		t.Fatalf("collectFindMatches failed: %v", err)
	}
	// May find skills from home directory (~/.agents/skills etc.),
	// but should not error
	_ = matches
}

func TestCollectFindMatchesWithSkills(t *testing.T) {
	tmpDir := t.TempDir()
	origDir := RegistryDir
	RegistryDir = tmpDir
	defer func() { RegistryDir = origDir }()

	// Create some skills in the registry
	skillDir := filepath.Join(tmpDir, "skills", "global", "test-skill-unique-zzz")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: A unique test skill\n---\n# Test"), 0644); err != nil {
		t.Fatal(err)
	}

	skillDir2 := filepath.Join(tmpDir, "skills", "global", "another-unique-zzz")
	if err := os.MkdirAll(skillDir2, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir2, "SKILL.md"), []byte("---\ndescription: Another unique skill for Python zzz\n---\n# Python"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test without query - should find at least our skills
	matches, err := collectFindMatches("")
	if err != nil {
		t.Fatalf("collectFindMatches failed: %v", err)
	}
	foundTest := false
	foundAnother := false
	for _, m := range matches {
		if m.Name == "test-skill-unique-zzz" {
			foundTest = true
		}
		if m.Name == "another-unique-zzz" {
			foundAnother = true
		}
	}
	if !foundTest {
		t.Error("expected to find 'test-skill-unique-zzz' in matches")
	}
	if !foundAnother {
		t.Error("expected to find 'another-unique-zzz' in matches")
	}

	// Test with unique query - should find only the unique skills
	matches, err = collectFindMatches("unique-zzz")
	if err != nil {
		t.Fatalf("collectFindMatches failed: %v", err)
	}
	if len(matches) != 2 {
		names := []string{}
		for _, m := range matches {
			names = append(names, m.Name)
		}
		t.Errorf("expected 2 matches for 'unique-zzz', got %d: %v", len(matches), names)
	}
}

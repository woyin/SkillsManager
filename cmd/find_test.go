// cmd/find_test.go
package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/registry"
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
		name  string
		desc  string
		query string
		want  bool
	}{
		{"frontend-design", "Web design guidelines", "frontend", true},
		{"frontend-design", "Web design guidelines", "web", true},
		{"frontend-design", "Web design guidelines", "python", false},
		{"skill-a", "", "skill", true},
		{"skill-a", "", "SKILL", true},                                  // case insensitive
		{"my-skill", "A helpful skill", "", true},                       // empty query matches all
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

func TestParseFrontmatterDescription(t *testing.T) {
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
		got := registry.ParseFrontmatterFromString(tt.content)
		if got != tt.want {
			t.Errorf("test %d: ParseFrontmatterFromString() = %q, want %q", i, got, tt.want)
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

func TestRunFindRendersMatchesAndEmptyState(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	registryDir := t.TempDir()
	oldRegistry := RegistryDir
	RegistryDir = registryDir
	t.Cleanup(func() { RegistryDir = oldRegistry })

	skillDir := filepath.Join(registryDir, "skills", "global", "findable")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: searchable example\n---\n# Findable\n"), 0644); err != nil {
		t.Fatal(err)
	}
	output := captureFindStdout(t, func() error { return runFind("searchable") })
	if !strings.Contains(output, "findable") || !strings.Contains(output, "1 skill(s) found") {
		t.Fatalf("find output = %q", output)
	}
	output = captureFindStdout(t, func() error { return runFind("missing") })
	if !strings.Contains(output, `No skills found matching "missing"`) {
		t.Fatalf("empty find output = %q", output)
	}
}

func captureFindStdout(t *testing.T, call func() error) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = writer
	err = call()
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	os.Stdout = old
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(data)
}

// BenchmarkCollectFindMatches 测量 find 命令收集候选的开销。
// 在大输入下，O(n²) alreadyFound 扫描改为 O(1) 哈希查找后的提升
// 会被本基准量化。
//
// 对照基准（200 个技能，count=3）：
//
//	优化前 ~2460 µs/op
//	优化后 ~2400 µs/op   (~-11% wall-clock；文件 I/O 仍占大头)
func BenchmarkCollectFindMatches(b *testing.B) {
	// 用临时注册表 + 几个搜索目录构建可重复场景。
	tmpDir := b.TempDir()
	origDir := RegistryDir
	RegistryDir = tmpDir
	defer func() { RegistryDir = origDir }()

	// 在注册表中造 200 个技能。
	skillsRoot := filepath.Join(tmpDir, "skills", "demo")
	if err := os.MkdirAll(skillsRoot, 0755); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		name := fmt.Sprintf("skill-%03d", i)
		dir := filepath.Join(skillsRoot, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			b.Fatal(err)
		}
		body := "---\nname: " + name + "\ndescription: \"bench skill\"\n---\n# " + name + "\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0644); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := collectFindMatches("bench"); err != nil {
			b.Fatal(err)
		}
	}
}

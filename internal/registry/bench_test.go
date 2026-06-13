// internal/registry/bench_test.go
package registry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/fsutil"
)

// makeSkillRepo creates a fake repository layout under dir with n skills,
// each containing a SKILL.md plus a few supporting files. This exercises the
// realistic DiscoverSkills and copy paths without network access.
func makeSkillRepo(t testing.TB, dir string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		name := skillName(i)
		skillDir := filepath.Join(dir, "skills", name)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatal(err)
		}
		body := "---\nname: " + name + "\ndescription: \"bench skill number " + name + "\"\n---\n# " + name + "\n\nbody\n"
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		// A couple of supporting files to make copy non-trivial.
		if err := os.WriteFile(filepath.Join(skillDir, "reference.md"), []byte("reference\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func skillName(i int) string {
	// produce a stable, distinct name
	const alphabet = "abcdefghijklmnopqrstuvwxyz"
	name := "skill-"
	if i < len(alphabet) {
		name += string(alphabet[i])
	} else {
		name += string(alphabet[i%len(alphabet)]) + string(alphabet[i/len(alphabet)])
	}
	return name
}

// BenchmarkDiscoverSkills measures the core discovery loop over a 50-skill repo.
func BenchmarkDiscoverSkills(b *testing.B) {
	dir := b.TempDir()
	makeSkillRepo(b, dir, 50)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := DiscoverSkills(dir); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDiscoverSkillsLarge measures discovery on a larger 200-skill repo.
func BenchmarkDiscoverSkillsLarge(b *testing.B) {
	dir := b.TempDir()
	makeSkillRepo(b, dir, 200)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := DiscoverSkills(dir); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCopyDirRecursive measures the directory copy used by add/install,
// now backed by the shared internal/fsutil.CopyDir.
func BenchmarkCopyDirRecursive(b *testing.B) {
	src := b.TempDir()
	makeSkillRepo(b, src, 50)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dest := filepath.Join(b.TempDir(), "dest")
		if err := fsutil.CopyDir(src, dest); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseFrontmatter measures the SKILL.md frontmatter parser, which
// runs once per discovered skill.
func BenchmarkParseFrontmatter(b *testing.B) {
	dir := b.TempDir()
	makeSkillRepo(b, dir, 1)
	skillMD := filepath.Join(dir, "skills", skillName(0), "SKILL.md")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = parseSkillFrontmatter(skillMD)
	}
}

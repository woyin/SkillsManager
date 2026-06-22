// internal/registry/bench_test.go 提供 registry 包的性能基准测试。
//
// 本文件中的基准用于回归对比：每次重构热点路径（frontmatter 解析、
// 目录拷贝、技能发现、远端 URL 解析、技能详情列举）后都应运行
// ?   	github.com/woyin/skills-manager	[no test files] 验证分配与时间未劣化。
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

// BenchmarkGitRemoteURL 测量 gitRemoteURL 的解析开销。
// 优化前使用 strings.Split 产生整段字符串数组；优化后使用字节级行扫描。
//
// 对照基准（count=3）：
//   优化前 ~9970 ns/op   9 allocs   1448 B/op
//   优化后 ~9850 ns/op   7 allocs   1080 B/op   (-22% allocs, -25% mem)
func BenchmarkGitRemoteURL(b *testing.B) {
	dir := b.TempDir()
	// 构造一个典型的 .git/config 文件。
	configDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		b.Fatal(err)
	}
	config := `[core]
	repositoryformatversion = 0
	filemode = true
[remote "origin"]
	url = https://github.com/owner/repo.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "main"]
	remote = origin
	merge = refs/heads/main
`
	if err := os.WriteFile(filepath.Join(configDir, "config"), []byte(config), 0644); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if got := gitRemoteURL(dir); got != "https://github.com/owner/repo.git" {
			b.Fatalf("got %q", got)
		}
	}
}

// BenchmarkListSkillDetails 测量 ListSkillDetails 的开销。
// 该路径同时受益于 stat 消除（复用 DirEntry.Info）与 gitRemoteURL
// 的零拷贝行扫描，是大注册表下 web/list/export 等命令的关键热点。
func BenchmarkListSkillDetails(b *testing.B) {
	dir := b.TempDir()
	// 造 50 个技能目录，每个带 .git/config 让 gitRemoteURL 有活干。
	for i := 0; i < 50; i++ {
		name := skillName(i)
		skillDir := filepath.Join(dir, "skills", "demo", name)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			b.Fatal(err)
		}
		body := "---\nname: " + name + "\ndescription: \"bench\"\n---\n# " + name + "\n"
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0644); err != nil {
			b.Fatal(err)
		}
		// 写一份带 origin URL 的 .git/config。
		gitDir := filepath.Join(skillDir, ".git")
		if err := os.MkdirAll(gitDir, 0755); err != nil {
			b.Fatal(err)
		}
		cfg := `[remote "origin"]
	url = https://github.com/owner/` + name + `.git
`
		if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(cfg), 0644); err != nil {
			b.Fatal(err)
		}
	}

	reg := New(dir)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := reg.ListSkillDetails(); err != nil {
			b.Fatal(err)
		}
	}
}

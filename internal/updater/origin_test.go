package updater

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/sourcecache"
)

// gitIn 运行 git -C dir args，并固定作者信息，避免依赖本机 git 配置。
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// writeSkillFile 在 dir 下写 SKILL.md。
func writeSkillFile(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func readSkillFile(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

const validSkillV1 = "---\nname: skill-a\ndescription: fixture v1\n---\nv1 body\n"
const validSkillV2 = "---\nname: skill-a\ndescription: fixture v2\n---\nv2 body\n"

// newSourceRepo 创建默认分支含 skill-a=v1 的仓库并打 tag v1，随后把默认分支推进到 v2。
func newSourceRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	gitIn(t, repo, "checkout", "-q", "-b", "main")
	writeSkillFile(t, filepath.Join(repo, "skill-a"), validSkillV1)
	gitIn(t, repo, "add", "-A")
	gitIn(t, repo, "commit", "-qm", "v1")
	gitIn(t, repo, "tag", "v1")
	writeSkillFile(t, filepath.Join(repo, "skill-a"), validSkillV2)
	gitIn(t, repo, "add", "-A")
	gitIn(t, repo, "commit", "-qm", "v2")
	return repo
}

// newRegistry 建 Registry 目录并写入旧版技能内容。
func newRegistry(t *testing.T, skills map[string]string) string {
	t.Helper()
	regDir := t.TempDir()
	for name, body := range skills {
		writeSkillFile(t, filepath.Join(regDir, "skills", "global", name), body)
	}
	return regDir
}

func TestGroupOriginTargetsKeepsSourceRefIsolation(t *testing.T) {
	targets := []OriginTarget{
		{Name: "a", SkillDir: "/r/a", Origin: registry.SkillOrigin{Source: "s1"}},
		{Name: "b", SkillDir: "/r/b", Origin: registry.SkillOrigin{Source: "s2", Ref: "v1"}},
		{Name: "c", SkillDir: "/r/c", Origin: registry.SkillOrigin{Source: "s1"}},
		{Name: "d", SkillDir: "/r/d", Origin: registry.SkillOrigin{Source: "s1", Ref: "v2"}},
	}
	groups := GroupOriginTargets(targets)
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3: %+v", len(groups), groups)
	}
	if groups[0].Source != "s1" || groups[0].Ref != "" || len(groups[0].Targets) != 2 {
		t.Fatalf("group0 = %+v", groups[0])
	}
	if groups[1].Source != "s2" || groups[1].Ref != "v1" || len(groups[1].Targets) != 1 {
		t.Fatalf("group1 = %+v", groups[1])
	}
	if groups[2].Source != "s1" || groups[2].Ref != "v2" || len(groups[2].Targets) != 1 {
		t.Fatalf("group2 = %+v", groups[2])
	}

	if !(OriginGroup{Ref: "v1", RefKind: registry.RefUnknown}).IsPinned() {
		t.Fatal("unknown ref kind with non-empty ref must stay pinned (legacy behavior)")
	}
	if (OriginGroup{RefKind: registry.RefDefaultBranch}).IsPinned() {
		t.Fatal("default branch must be tracking")
	}
	if !(OriginGroup{Ref: "v1", RefKind: registry.RefTag}).IsPinned() {
		t.Fatal("tag must be pinned")
	}
}

func TestRefreshOriginGroupPinnedRewritesWithoutAdvancing(t *testing.T) {
	src := newSourceRepo(t)
	dataDir := t.TempDir()
	cache, err := sourcecache.Acquire(dataDir, src, "v1", false)
	if err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	before := HeadHash(cache.Path)

	regDir := newRegistry(t, map[string]string{"skill-a": "---\nname: skill-a\ndescription: old\n---\nold body\n"})
	skillDir := filepath.Join(regDir, "skills", "global", "skill-a")
	targets := []OriginTarget{{
		Name:     "skill-a",
		SkillDir: skillDir,
		Origin: registry.SkillOrigin{
			Source: src, Ref: "v1", SourceKind: registry.SourceGit, RefKind: registry.RefTag, SubPath: "skill-a",
		},
	}}

	res, err := RefreshOriginGroup(OriginGroup{Source: src, Ref: "v1", RefKind: registry.RefTag, Targets: targets}, Options{RegistryDir: regDir, DataDir: dataDir})
	if err != nil {
		t.Fatalf("RefreshOriginGroup: %v", err)
	}
	if !res.Pinned {
		t.Fatal("tag group must be reported pinned")
	}
	if res.CommitBefore != before || res.CommitAfter != before {
		t.Fatalf("pinned must not advance cache: before=%s after=%s want=%s", res.CommitBefore, res.CommitAfter, before)
	}
	if res.Written != 1 || len(res.Failures) != 0 {
		t.Fatalf("result = %+v", res)
	}
	if got := readSkillFile(t, skillDir); got != validSkillV1 {
		t.Fatalf("content = %q, want v1 content", got)
	}
	read := registry.New(regDir).ReadOrigin(skillDir)
	if !read.Valid || read.Origin.Commit != before {
		t.Fatalf("origin = %+v", read)
	}
	if read.Origin.SourceKind != registry.SourceGit || read.Origin.RefKind != registry.RefTag {
		t.Fatalf("origin schema not preserved: %+v", read.Origin)
	}
}

func TestRefreshOriginGroupDetachedIsPinned(t *testing.T) {
	src := newSourceRepo(t)
	dataDir := t.TempDir()
	cache, err := sourcecache.Acquire(dataDir, src, "", false)
	if err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	gitIn(t, cache.Path, "checkout", "-q", "--detach", "HEAD")

	regDir := newRegistry(t, map[string]string{"skill-a": "---\nname: skill-a\ndescription: old\n---\nold body\n"})
	skillDir := filepath.Join(regDir, "skills", "global", "skill-a")
	targets := []OriginTarget{{
		Name:     "skill-a",
		SkillDir: skillDir,
		Origin:   registry.SkillOrigin{Source: src, SourceKind: registry.SourceGit, RefKind: registry.RefDefaultBranch, SubPath: "skill-a"},
	}}

	res, err := RefreshOriginGroup(OriginGroup{Source: src, RefKind: registry.RefDefaultBranch, Targets: targets}, Options{RegistryDir: regDir, DataDir: dataDir})
	if err != nil {
		t.Fatalf("RefreshOriginGroup: %v", err)
	}
	if !res.Pinned || res.CommitBefore != res.CommitAfter || res.Written != 1 {
		t.Fatalf("detached cache must be treated as pinned: %+v", res)
	}
	if got := readSkillFile(t, skillDir); got != validSkillV2 {
		t.Fatalf("content = %q, want cache HEAD content", got)
	}
}

func TestRefreshOriginGroupDirtyCacheReportsGroupError(t *testing.T) {
	src := newSourceRepo(t)
	dataDir := t.TempDir()
	cache, err := sourcecache.Acquire(dataDir, src, "", false)
	if err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cache.Path, "skill-a", "untracked.txt"), []byte("dirty"), 0644); err != nil {
		t.Fatal(err)
	}

	regDir := newRegistry(t, map[string]string{"skill-a": "---\nname: skill-a\ndescription: old\n---\nold body\n"})
	skillDir := filepath.Join(regDir, "skills", "global", "skill-a")
	targets := []OriginTarget{{
		Name:     "skill-a",
		SkillDir: skillDir,
		Origin:   registry.SkillOrigin{Source: src, SourceKind: registry.SourceGit, RefKind: registry.RefDefaultBranch, SubPath: "skill-a"},
	}}

	if _, err := RefreshOriginGroup(OriginGroup{Source: src, RefKind: registry.RefDefaultBranch, Targets: targets}, Options{RegistryDir: regDir, DataDir: dataDir}); err == nil {
		t.Fatal("dirty source cache must return a group error")
	}
	if got := readSkillFile(t, skillDir); !strings.Contains(got, "old body") {
		t.Fatalf("registry must stay untouched on dirty cache, got %q", got)
	}
}

func TestRewriteOriginTargetsAtomicAcrossSource(t *testing.T) {
	src := t.TempDir()
	gitIn(t, src, "init", "-q")
	gitIn(t, src, "checkout", "-q", "-b", "main")
	writeSkillFile(t, filepath.Join(src, "good-skill"), "---\nname: good-skill\ndescription: new good\n---\nnew good\n")
	// 缺少 description → LintError，触发整组回滚。
	writeSkillFile(t, filepath.Join(src, "bad-skill"), "---\nname: bad-skill\n---\nnew bad\n")
	gitIn(t, src, "add", "-A")
	gitIn(t, src, "commit", "-qm", "skills")

	dataDir := t.TempDir()
	cache, err := sourcecache.Acquire(dataDir, src, "", false)
	if err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	oldGood := "---\nname: good-skill\ndescription: old good\n---\nold good\n"
	oldBad := "---\nname: bad-skill\ndescription: old bad\n---\nold bad\n"
	regDir := newRegistry(t, map[string]string{"good-skill": oldGood, "bad-skill": oldBad})
	goodDir := filepath.Join(regDir, "skills", "global", "good-skill")
	badDir := filepath.Join(regDir, "skills", "global", "bad-skill")

	targets := []OriginTarget{
		{Name: "good-skill", SkillDir: goodDir, Origin: registry.SkillOrigin{Source: src, SourceKind: registry.SourceGit, RefKind: registry.RefDefaultBranch, SubPath: "good-skill"}},
		{Name: "bad-skill", SkillDir: badDir, Origin: registry.SkillOrigin{Source: src, SourceKind: registry.SourceGit, RefKind: registry.RefDefaultBranch, SubPath: "bad-skill"}},
	}
	written, failures := RewriteOriginTargets(cache.Path, targets, Options{RegistryDir: regDir, DataDir: dataDir})
	if written != 0 || len(failures) == 0 {
		t.Fatalf("written=%d failures=%+v, want 0 and failures", written, failures)
	}
	if got := readSkillFile(t, goodDir); got != oldGood {
		t.Fatalf("good-skill must be rolled back, got %q", got)
	}
	if got := readSkillFile(t, badDir); got != oldBad {
		t.Fatalf("bad-skill must be untouched, got %q", got)
	}
}

func TestRefreshOriginGroupPreservesRegistryOriginSchema(t *testing.T) {
	src := newSourceRepo(t)
	dataDir := t.TempDir()
	cache, err := sourcecache.Acquire(dataDir, src, "v1", false)
	if err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	regDir := newRegistry(t, map[string]string{"skill-a": "---\nname: skill-a\ndescription: old\n---\nold body\n"})
	skillDir := filepath.Join(regDir, "skills", "global", "skill-a")
	reg := registry.New(regDir)
	if err := reg.WriteOrigin(skillDir, registry.SkillOrigin{
		Source: src, Ref: "v1", SourceKind: registry.SourceGit, RefKind: registry.RefTag, SubPath: "skill-a", Commit: "deadbeef",
	}); err != nil {
		t.Fatalf("seed origin: %v", err)
	}

	targets := []OriginTarget{{
		Name:     "skill-a",
		SkillDir: skillDir,
		Origin:   reg.ReadOrigin(skillDir).Origin,
	}}
	res, err := RefreshOriginGroup(OriginGroup{Source: src, Ref: "v1", RefKind: registry.RefTag, Targets: targets}, Options{RegistryDir: regDir, DataDir: dataDir})
	if err != nil || res.Written != 1 {
		t.Fatalf("refresh: res=%+v err=%v", res, err)
	}

	read := reg.ReadOrigin(skillDir)
	if !read.Valid || read.Origin.SourceKind != registry.SourceGit || read.Origin.RefKind != registry.RefTag || read.Origin.Ref != "v1" {
		t.Fatalf("origin schema lost: %+v", read.Origin)
	}
	if read.Origin.Commit != HeadHash(cache.Path) {
		t.Fatalf("origin commit = %s, want %s", read.Origin.Commit, HeadHash(cache.Path))
	}
	raw, err := os.ReadFile(filepath.Join(skillDir, registry.OriginFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "\"schema_version\"") || !strings.Contains(string(raw), "\"source_kind\"") {
		t.Fatalf("raw origin missing schema fields: %s", raw)
	}
}

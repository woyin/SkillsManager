package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

// makeLocalSkillSource 在 dir 下造一个含 SKILL.md 的本地技能源目录，
// 返回源根目录（可直接传给 installSkillsToAgents 的 source 参数）。
func makeLocalSkillSource(t *testing.T, dir, name string) string {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("creating local skill source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: test\n---\n# "+name+"\n"), 0644); err != nil {
		t.Fatalf("writing SKILL.md: %v", err)
	}
	return dir
}

// withTestRegistry 把 RegistryDir 指到临时目录，测试结束恢复。
func withTestRegistry(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "registry")
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0755); err != nil {
		t.Fatalf("mkdir registry: %v", err)
	}
	old := RegistryDir
	RegistryDir = dir
	t.Cleanup(func() { RegistryDir = old })
	return dir
}

// TestInstallProjectScopeWritesProjectDir 验证项目级把技能装到
// projectDir/<ProjectSkillDir>/<name>，并写入 registry 原件。
func TestInstallProjectScopeWritesProjectDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	regDir := withTestRegistry(t)

	projectDir := t.TempDir()
	source := t.TempDir()
	makeLocalSkillSource(t, source, "alpha")

	err := installSkillsToAgents(source, []string{"claude"}, []string{"*"}, false, true, projectDir)
	if err != nil {
		t.Fatalf("installSkillsToAgents project: %v", err)
	}

	want := filepath.Join(projectDir, tool.Claude.ProjectSkillDir, "alpha")
	assertExists(t, want)

	// symlink 应指向 registry 原件
	target, err := os.Readlink(want)
	if err != nil {
		t.Fatalf("expected symlink at %s: %v", want, err)
	}
	regSkill := filepath.Join(regDir, "skills", "global", "alpha")
	if target != regSkill {
		absTarget, _ := filepath.EvalSymlinks(want)
		absReg, _ := filepath.EvalSymlinks(regSkill)
		if absTarget != absReg {
			t.Fatalf("symlink target = %q, want registry %q", target, regSkill)
		}
	}

	globalLink := filepath.Join(tmpHome, tool.Claude.SkillDir, "alpha")
	if _, err := os.Lstat(globalLink); !os.IsNotExist(err) {
		t.Fatalf("global link should not exist, got %v", err)
	}
}

func TestInstallSubagentEveRedirectsDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	withTestRegistry(t)

	projectDir := t.TempDir()
	source := t.TempDir()
	makeLocalSkillSource(t, source, "beta")

	// --subagent 仅对 eve 生效：重定向到 agent/subagents/<name>/skills
	saved := installSubagent
	t.Cleanup(func() { installSubagent = saved })
	installSubagent = "researcher"

	if err := installSkillsToAgents(source, []string{"eve"}, []string{"*"}, false, true, projectDir); err != nil {
		t.Fatalf("installSkillsToAgents eve subagent: %v", err)
	}

	want := filepath.Join(projectDir, "agent", "subagents", "researcher", "skills", "beta")
	assertExists(t, want)
}

func TestInstallGlobalScopeWritesHomeDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	withTestRegistry(t)

	source := t.TempDir()
	makeLocalSkillSource(t, source, "beta")

	err := installSkillsToAgents(source, []string{"claude"}, []string{"*"}, false, false, "")
	if err != nil {
		t.Fatalf("installSkillsToAgents global: %v", err)
	}

	want := filepath.Join(tmpHome, tool.Claude.SkillDir, "beta")
	assertExists(t, want)
}

func TestResolveInstallAgentsRequiresDetectedOrExplicit(t *testing.T) {
	tools, err := resolveInstallAgents([]string{"claude"})
	if err != nil || len(tools) != 1 {
		t.Fatalf("explicit agent: tools=%v err=%v", tools, err)
	}

	_, err = resolveInstallAgents([]string{"no-such-agent-xyz"})
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
	if !strings.Contains(err.Error(), "no matching agents") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSelectSkillsForInstallYesInstallsAll(t *testing.T) {
	oldYes := installYes
	installYes = true
	t.Cleanup(func() { installYes = oldYes })

	skills := []registry.DiscoveredSkill{
		{Name: "a", Path: "/tmp/a"},
		{Name: "b", Path: "/tmp/b"},
	}
	got, err := selectSkillsForInstall(skills, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 skills, got %d", len(got))
	}
}

func TestSelectSkillsExplicitFilter(t *testing.T) {
	skills := []registry.DiscoveredSkill{
		{Name: "a", Path: "/tmp/a"},
		{Name: "b", Path: "/tmp/b"},
	}
	got, err := selectSkillsForInstall(skills, []string{"b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "b" {
		t.Fatalf("want [b], got %#v", got)
	}
}

func TestCachedGitSourceKeepsSymlinkTarget(t *testing.T) {
	oldDataDir := DataDir
	DataDir = t.TempDir()
	t.Cleanup(func() { DataDir = oldDataDir })

	repo := filepath.Join(t.TempDir(), "source.git")
	if err := os.Mkdir(repo, 0755); err != nil {
		t.Fatal(err)
	}
	makeLocalSkillSource(t, repo, "cached")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q")
	run("add", ".")
	run("-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-qm", "init")

	cached, err := cachedGitSource("file://"+repo, "")
	if err != nil {
		t.Fatalf("cachedGitSource: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cached, "cached", "SKILL.md")); err != nil {
		t.Fatalf("cached source missing skill: %v", err)
	}
	again, err := cachedGitSource("file://"+repo, "")
	if err != nil || again != cached {
		t.Fatalf("cache reuse = %q, %v; want %q", again, err, cached)
	}
}

func TestCachedGitSourcePinsRefAndSeparatesCache(t *testing.T) {
	oldDataDir := DataDir
	DataDir = t.TempDir()
	t.Cleanup(func() { DataDir = oldDataDir })

	repo := filepath.Join(t.TempDir(), "source.git")
	if err := os.Mkdir(repo, 0755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, "", "init", "-q", repo)
	gitRun(t, repo, "config", "user.name", "test")
	gitRun(t, repo, "config", "user.email", "test@example.com")
	writeUpdateSkill(t, repo, "one\n")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-qm", "one")
	first := gitHeadHash(repo)
	writeUpdateSkill(t, repo, "two\n")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-qm", "two")

	pinned, err := cachedGitSource("file://"+repo, first)
	if err != nil {
		t.Fatal(err)
	}
	floating, err := cachedGitSource("file://"+repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if pinned == floating {
		t.Fatal("pinned and floating sources shared cache path")
	}
	if got := gitHeadHash(pinned); got != first {
		t.Fatalf("pinned HEAD = %s, want %s", got, first)
	}
	if detached, err := gitDetached(pinned); err != nil || !detached {
		t.Fatalf("detached = %v, %v", detached, err)
	}
}

func TestCachedGitSourceOfflineUsesExactCache(t *testing.T) {
	oldDataDir := DataDir
	DataDir = t.TempDir()
	t.Cleanup(func() { DataDir = oldDataDir })

	repo := filepath.Join(t.TempDir(), "source.git")
	if err := os.Mkdir(repo, 0755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, "", "init", "-q", repo)
	gitRun(t, repo, "config", "user.name", "test")
	gitRun(t, repo, "config", "user.email", "test@example.com")
	writeUpdateSkill(t, repo, "offline\n")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-qm", "initial")
	ref := gitHeadHash(repo)

	cached, err := cachedGitSource("file://"+repo, ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cachedGitSource("file://"+repo, ref, true); err != nil {
		t.Fatalf("offline exact cache: %v", err)
	}
	if _, err := cachedGitSource("file://"+repo, "missing", true); err == nil {
		t.Fatal("offline cache miss should fail")
	}
	_, metaPath := sourceCachePaths("file://"+repo, ref)
	meta := readSourceCacheMetadata(metaPath)
	if meta.Source != "file://"+repo || meta.Ref != ref || meta.Commit != gitHeadHash(cached) {
		t.Fatalf("metadata = %+v", meta)
	}
}

func TestListSkillsFromSourceOfflineUsesPinnedCache(t *testing.T) {
	oldData, oldRef, oldOffline := DataDir, installRef, installOffline
	DataDir, installOffline = t.TempDir(), true
	t.Cleanup(func() { DataDir, installRef, installOffline = oldData, oldRef, oldOffline })

	repo := filepath.Join(t.TempDir(), "source.git")
	if err := os.Mkdir(repo, 0755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, "", "init", "-q", repo)
	gitRun(t, repo, "config", "user.name", "test")
	gitRun(t, repo, "config", "user.email", "test@example.com")
	makeLocalSkillSource(t, repo, "alpha")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-qm", "alpha")
	installRef = gitHeadHash(repo)
	if _, err := cachedGitSource("file://"+repo, installRef); err != nil {
		t.Fatal(err)
	}
	if err := listSkillsFromSource("file://"+repo, false); err != nil {
		t.Fatalf("offline list: %v", err)
	}

	installRef = "missing"
	if err := listSkillsFromSource("file://"+repo, false); err == nil {
		t.Fatal("offline list cache miss should fail")
	}
}

func TestInstallGitSourceWritesOriginAndSymlinksRegistry(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	regDir := withTestRegistry(t)
	oldData := DataDir
	DataDir = t.TempDir()
	t.Cleanup(func() { DataDir = oldData })

	// bare-ish file:// repo with one skill
	repo := filepath.Join(t.TempDir(), "src.git")
	if err := os.Mkdir(repo, 0755); err != nil {
		t.Fatal(err)
	}
	makeLocalSkillSource(t, repo, "gamma")
	gitRun(t, "", "init", "-q", repo)
	gitRun(t, repo, "config", "user.name", "test")
	gitRun(t, repo, "config", "user.email", "test@example.com")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-qm", "init")

	projectDir := t.TempDir()
	src := "file://" + repo
	if err := installSkillsToAgents(src, []string{"claude"}, []string{"*"}, false, true, projectDir); err != nil {
		t.Fatalf("install: %v", err)
	}

	regSkill := filepath.Join(regDir, "skills", "global", "gamma")
	assertExists(t, regSkill)
	origin, ok := readSkillOrigin(regSkill)
	if !ok {
		t.Fatal("expected .sm-origin.json on registry skill")
	}
	if origin.Source != src {
		t.Fatalf("origin.Source = %q, want %q", origin.Source, src)
	}
	if origin.RelPath != "gamma" && origin.RelPath != "gamma"+string(filepath.Separator) {
		// RelPath should be relative path of skill inside clone
		if filepath.Base(origin.RelPath) != "gamma" {
			t.Fatalf("origin.RelPath = %q", origin.RelPath)
		}
	}

	link := filepath.Join(projectDir, tool.Claude.ProjectSkillDir, "gamma")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if target != regSkill {
		absT, _ := filepath.EvalSymlinks(link)
		absR, _ := filepath.EvalSymlinks(regSkill)
		if absT != absR {
			t.Fatalf("link target %q want %q", target, regSkill)
		}
	}
}

func TestEnsureSkillsInRegistryWarnsOnOverwrite(t *testing.T) {
	regDir := withTestRegistry(t)
	source := t.TempDir()
	makeLocalSkillSource(t, source, "dup")
	// first install
	skills, err := registry.DiscoverSkills(source)
	if err != nil || len(skills) == 0 {
		t.Fatalf("discover: %v", err)
	}
	if _, err := ensureSkillsInRegistry(skills, "", "", ""); err != nil {
		t.Fatal(err)
	}
	// second should overwrite
	if _, err := ensureSkillsInRegistry(skills, "", "", ""); err != nil {
		t.Fatal(err)
	}
	assertExists(t, filepath.Join(regDir, "skills", "global", "dup"))
}

// TestInstallProjectScopeWritesLockfile verifies that project-scope Direct Install
// writes skills-lock.json with source and hash entries.
func TestInstallProjectScopeWritesLockfile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	withTestRegistry(t)

	projectDir := t.TempDir()
	source := t.TempDir()
	makeLocalSkillSource(t, source, "locktest")

	err := installSkillsToAgents(source, []string{"claude"}, []string{"*"}, false, true, projectDir)
	if err != nil {
		t.Fatalf("installSkillsToAgents project: %v", err)
	}

	// skills-lock.json should exist in projectDir
	lockPath := filepath.Join(projectDir, "skills-lock.json")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("skills-lock.json not written: %v", err)
	}

	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("reading skills-lock.json: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "locktest") {
		t.Errorf("skills-lock.json missing skill name: %s", content)
	}
	if !strings.Contains(content, "computedHash") {
		t.Errorf("skills-lock.json missing computedHash: %s", content)
	}
	if !strings.Contains(content, "local") {
		t.Errorf("skills-lock.json missing sourceType 'local': %s", content)
	}
}

// TestInstallFromLockNoFile verifies --from-lock errors when no lockfile exists.
func TestInstallFromLockNoFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()

	projectDir := t.TempDir()
	oldDir := installDir
	installDir = projectDir
	t.Cleanup(func() { installDir = oldDir })

	err := installFromLockFile(nil)
	if err == nil {
		t.Fatal("expected error when no lockfile exists")
	}
	if !strings.Contains(err.Error(), "no skills-lock.json") {
		t.Errorf("expected 'no skills-lock.json' error, got: %v", err)
	}
}

// TestPathsOverlap verifies the install-time overlap guard used to prevent
// self-referential installs (source at or inside the destination).
func TestPathsOverlap(t *testing.T) {
	base := t.TempDir()
	inner := filepath.Join(base, "child")
	other := t.TempDir()

	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"equal paths", base, base, true},
		{"a contains b", base, inner, true},
		{"b contains a", inner, base, true},
		{"sibling dirs", base, other, false},
		{"prefix no separator", filepath.Join(base, "foo"), filepath.Join(base, "foobar"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pathsOverlap(c.a, c.b); got != c.want {
				t.Errorf("pathsOverlap(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

// TestFilterSkillsCaseInsensitive verifies that --skill matching is
// case-insensitive, matching npx skills filterSkills behavior.
func TestFilterSkillsCaseInsensitive(t *testing.T) {
	discovered := []registry.DiscoveredSkill{
		{Name: "MySkill"},
		{Name: "other-skill"},
		{Name: "third"},
	}
	cases := []struct {
		name      string
		input     []string
		wantN     int
		wantFirst string
	}{
		{"exact", []string{"MySkill"}, 1, "MySkill"},
		{"lowercase query", []string{"myskill"}, 1, "MySkill"},
		{"uppercase query", []string{"OTHER-SKILL"}, 1, "other-skill"},
		{"no match", []string{"nomatch"}, 0, ""},
		{"wildcard returns all", []string{"*"}, 3, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := filterSkills(discovered, c.input)
			if len(got) != c.wantN {
				t.Fatalf("got %d skills, want %d", len(got), c.wantN)
			}
			if c.wantFirst != "" && got[0].Name != c.wantFirst {
				t.Errorf("first match = %q, want %q", got[0].Name, c.wantFirst)
			}
		})
	}
}

package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/home"
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

// TestInstallProjectScopeWritesProjectDir 验证 --project 把技能装到
// projectDir/<ProjectSkillDir>/<name> 而非全局 ~/SkillDir/<name>。
func TestInstallProjectScopeWritesProjectDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()

	projectDir := t.TempDir()
	source := t.TempDir()
	makeLocalSkillSource(t, source, "alpha")

	err := installSkillsToAgents(source, []string{"claude"}, []string{"*"}, false, true, projectDir)
	if err != nil {
		t.Fatalf("installSkillsToAgents project: %v", err)
	}

	want := filepath.Join(projectDir, tool.Claude.ProjectSkillDir, "alpha")
	assertExists(t, want)

	globalLink := filepath.Join(tmpHome, tool.Claude.SkillDir, "alpha")
	if _, err := os.Lstat(globalLink); !os.IsNotExist(err) {
		t.Fatalf("global link should not exist, got %v", err)
	}
}

func TestInstallGlobalScopeWritesHomeDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()

	source := t.TempDir()
	makeLocalSkillSource(t, source, "beta")

	err := installSkillsToAgents(source, []string{"claude"}, []string{"*"}, false, false, "")
	if err != nil {
		t.Fatalf("installSkillsToAgents global: %v", err)
	}

	want := filepath.Join(tmpHome, tool.Claude.SkillDir, "beta")
	assertExists(t, want)
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
	if err := listSkillsFromSource("file://" + repo); err != nil {
		t.Fatalf("offline list: %v", err)
	}

	installRef = "missing"
	if err := listSkillsFromSource("file://" + repo); err == nil {
		t.Fatal("offline list cache miss should fail")
	}
}

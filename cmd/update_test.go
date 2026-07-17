package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/tool"
)

func TestUpdateGitReposWalksSkillsAndMCP(t *testing.T) {
	registryDir := t.TempDir()
	skillRepo := filepath.Join(registryDir, "skills", "cloudflare", "workers")
	mcpRepo := filepath.Join(registryDir, "mcp", "browser")
	for _, repo := range []string{skillRepo, mcpRepo} {
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0755); err != nil {
			t.Fatalf("creating fake repo: %v", err)
		}
	}

	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "git.log")
	fakeGit := filepath.Join(binDir, "git")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + logPath + "\nexit 0\n"
	if err := os.WriteFile(fakeGit, []byte(script), 0755); err != nil {
		t.Fatalf("writing fake git: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	summary, err := updateGitRepos(registryDir)
	if err != nil {
		t.Fatalf("updateGitRepos failed: %v", err)
	}
	if summary.Updated != 2 {
		t.Fatalf("Expected 2 updated repos, got %+v", summary)
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading fake git log: %v", err)
	}
	log := string(logData)
	for _, repo := range []string{skillRepo, mcpRepo} {
		if !strings.Contains(log, "-C "+repo+" pull --ff-only") {
			t.Fatalf("Expected git pull for %s, log was:\n%s", repo, log)
		}
	}
}

// TestProjectInstalledSourcesReverseLooksUpRegistry 验证 --dir 反查：
// 项目只装了 A，反查结果应只含 A 的 git 源，不含 B。
func TestProjectInstalledSourcesReverseLooksUpRegistry(t *testing.T) {
	registryDir := t.TempDir()
	repoA := filepath.Join(registryDir, "skills", "global", "alpha")
	repoB := filepath.Join(registryDir, "skills", "global", "beta")
	for _, repo := range []string{repoA, repoB} {
		if err := os.MkdirAll(repo, 0755); err != nil {
			t.Fatalf("creating repo: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0755); err != nil {
			t.Fatalf("creating fake .git: %v", err)
		}
	}
	// macOS 下 /var 是 /private/var 的软链接。PointInside 对 target 与 root
	// 都只用 filepath.Abs（不解析软链），二者须在同一字面路径空间；因此
	// RegistryDir 与 symlink target 都用字面 registryDir 派生路径。
	// nearestGitRepo 内部用 EvalSymlinks 解析，返回规整后的路径，
	// 断言时同样规整期望值。
	wantRepoA, _ := filepath.EvalSymlinks(repoA)

	oldRegistry := RegistryDir
	RegistryDir = registryDir
	t.Cleanup(func() { RegistryDir = oldRegistry })

	// 项目目录下只建指向 A 的 symlink（字面 repoA，未规整）
	projectDir := t.TempDir()
	toolInstance := tool.AllTools()[0] // 任取一个有 ProjectSkillDir 的工具
	linkDir := filepath.Join(projectDir, toolInstance.ProjectSkillDir)
	if err := os.MkdirAll(linkDir, 0755); err != nil {
		t.Fatalf("creating project skill dir: %v", err)
	}
	if err := os.Symlink(repoA, filepath.Join(linkDir, "alpha")); err != nil {
		t.Fatalf("creating symlink: %v", err)
	}

	sources := projectInstalledSources(projectDir)
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d: %v", len(sources), sources)
	}
	if sources[0] != wantRepoA {
		t.Fatalf("expected %s, got %s", wantRepoA, sources[0])
	}
}

// TestUpdateProjectSourcesOnlyPullsInstalledSource 验证 --dir 只 pull
// 项目实际安装的源（端到端，用 fake git 记录调用）。
func TestUpdateProjectSourcesOnlyPullsInstalledSource(t *testing.T) {
	registryDir := t.TempDir()
	repoA := filepath.Join(registryDir, "skills", "global", "alpha")
	repoB := filepath.Join(registryDir, "skills", "global", "beta")
	for _, repo := range []string{repoA, repoB} {
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0755); err != nil {
			t.Fatalf("creating fake repo: %v", err)
		}
	}
	// macOS 下 /var 是 /private/var 的软链接；建 symlink 用字面路径，
	// 断言比较时规整（nearestGitRepo 返回规整路径）。
	wantA, _ := filepath.EvalSymlinks(repoA)
	wantB, _ := filepath.EvalSymlinks(repoB)

	oldRegistry := RegistryDir
	RegistryDir = registryDir
	t.Cleanup(func() { RegistryDir = oldRegistry })

	// fake git：把调用参数写入日志
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "git.log")
	fakeGit := filepath.Join(binDir, "git")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + logPath + "\nexit 0\n"
	if err := os.WriteFile(fakeGit, []byte(script), 0755); err != nil {
		t.Fatalf("writing fake git: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// 项目只装 A（字面 repoA）
	projectDir := t.TempDir()
	toolInstance := tool.AllTools()[0]
	linkDir := filepath.Join(projectDir, toolInstance.ProjectSkillDir)
	if err := os.MkdirAll(linkDir, 0755); err != nil {
		t.Fatalf("creating project skill dir: %v", err)
	}
	if err := os.Symlink(repoA, filepath.Join(linkDir, "alpha")); err != nil {
		t.Fatalf("creating symlink: %v", err)
	}

	updateDir = projectDir
	t.Cleanup(func() { updateDir = "" })
	if err := updateProjectSources(nil); err != nil {
		t.Fatalf("updateProjectSources: %v", err)
	}

	logData, _ := os.ReadFile(logPath)
	log := string(logData)
	if !strings.Contains(log, "-C "+wantA+" pull --ff-only") {
		t.Fatalf("expected pull for A (%s), log was:\n%s", wantA, log)
	}
	if strings.Contains(log, "-C "+wantB+" pull --ff-only") {
		t.Fatalf("should NOT pull B (not installed), log was:\n%s", log)
	}
}

func TestManagedGitRepoDirsIncludesSourceCache(t *testing.T) {
	registryDir, dataDir := t.TempDir(), t.TempDir()
	registryRepo := filepath.Join(registryDir, "skills", "global", "alpha")
	cachedRepo := filepath.Join(dataDir, "sources", "hash")
	for _, repo := range []string{registryRepo, cachedRepo} {
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	got := managedGitRepoDirs(registryDir, dataDir)
	if len(got) != 2 || !containsString(got, registryRepo) || !containsString(got, cachedRepo) {
		t.Fatalf("managed repos = %v", got)
	}
}

func TestProjectInstalledSourcesFindsCachedRemoteSource(t *testing.T) {
	oldRegistry, oldData := RegistryDir, DataDir
	RegistryDir, DataDir = t.TempDir(), t.TempDir()
	t.Cleanup(func() { RegistryDir, DataDir = oldRegistry, oldData })

	cachedRepo := filepath.Join(DataDir, "sources", "hash")
	skillDir := filepath.Join(cachedRepo, "skills", "alpha")
	if err := os.MkdirAll(filepath.Join(cachedRepo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	projectDir := t.TempDir()
	linkDir := filepath.Join(projectDir, tool.AllTools()[0].ProjectSkillDir)
	if err := os.MkdirAll(linkDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(skillDir, filepath.Join(linkDir, "alpha")); err != nil {
		t.Fatal(err)
	}

	got := projectInstalledSources(projectDir)
	want, _ := filepath.EvalSymlinks(cachedRepo)
	if len(got) != 1 || got[0] != want {
		t.Fatalf("sources = %v, want [%s]", got, want)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestPullReposRollsBackInvalidSkillUpdate(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "remote.git")
	work := filepath.Join(t.TempDir(), "work")
	registryDir := t.TempDir()
	skillRepo := filepath.Join(registryDir, "skills", "global", "safe")

	gitRun(t, "", "init", "--bare", "-q", remote)
	gitRun(t, "", "clone", "-q", remote, work)
	gitRun(t, work, "config", "user.name", "test")
	gitRun(t, work, "config", "user.email", "test@example.com")
	writeUpdateSkill(t, work, "---\nname: safe\ndescription: this valid description triggers safely\n---\n# Safe\n")
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-qm", "valid")
	gitRun(t, work, "push", "-q", "origin", "HEAD")
	gitRun(t, "", "clone", "-q", remote, skillRepo)
	before := gitHeadHash(skillRepo)

	writeUpdateSkill(t, work, "---\nname: safe\n---\n# Broken\n")
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-qm", "invalid")
	gitRun(t, work, "push", "-q", "origin", "HEAD")

	oldRegistry := RegistryDir
	RegistryDir = registryDir
	t.Cleanup(func() { RegistryDir = oldRegistry })
	results := pullReposConcurrently([]namedRepo{{path: skillRepo, label: "safe", skillRel: "global/safe"}})
	if len(results) != 1 || results[0].ok || !results[0].rolledBack {
		t.Fatalf("result = %+v", results)
	}
	if got := gitHeadHash(skillRepo); got != before {
		t.Fatalf("HEAD = %s, want rollback to %s", got, before)
	}
}

func TestPullReposRefusesDirtyRepository(t *testing.T) {
	repo := t.TempDir()
	gitRun(t, "", "init", "-q", repo)
	gitRun(t, repo, "config", "user.name", "test")
	gitRun(t, repo, "config", "user.email", "test@example.com")
	writeUpdateSkill(t, repo, "initial\n")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-qm", "initial")
	writeUpdateSkill(t, repo, "dirty\n")

	results := pullReposConcurrently([]namedRepo{{path: repo, label: "dirty"}})
	if len(results) != 1 || results[0].ok {
		t.Fatalf("result = %+v", results)
	}
	data, err := os.ReadFile(filepath.Join(repo, "SKILL.md"))
	if err != nil || string(data) != "dirty\n" {
		t.Fatalf("local change lost: %q, %v", data, err)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func writeUpdateSkill(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestPullReposSkipsPinnedRepository(t *testing.T) {
	repo := t.TempDir()
	gitRun(t, "", "init", "-q", repo)
	gitRun(t, repo, "config", "user.name", "test")
	gitRun(t, repo, "config", "user.email", "test@example.com")
	writeUpdateSkill(t, repo, "pinned\n")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-qm", "initial")
	head := gitHeadHash(repo)
	gitRun(t, repo, "checkout", "-q", "--detach", head)

	results := pullReposConcurrently([]namedRepo{{path: repo, label: "pinned"}})
	if len(results) != 1 || !results[0].skipped || results[0].ok {
		t.Fatalf("result = %+v", results)
	}
	if got := gitHeadHash(repo); got != head {
		t.Fatalf("HEAD changed: %s", got)
	}
}

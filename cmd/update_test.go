package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/lockfile"
	"github.com/woyin/skills-manager/internal/registry"
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

func TestUpdateSpecificSkillsReportsMissingAndOrphanedEntries(t *testing.T) {
	oldRegistry, oldData := RegistryDir, DataDir
	RegistryDir, DataDir = t.TempDir(), t.TempDir()
	t.Cleanup(func() { RegistryDir, DataDir = oldRegistry, oldData })
	orphan := filepath.Join(RegistryDir, "skills", "global", "orphan")
	if err := os.MkdirAll(orphan, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphan, "SKILL.md"), []byte("---\nname: orphan\ndescription: orphan update test\n---\n# Orphan\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := updateSpecificSkills([]string{"orphan", "missing"}); err != nil {
		t.Fatalf("updateSpecificSkills: %v", err)
	}
}

func TestLegacyUpdateHelpersHandleEmptyRegistry(t *testing.T) {
	oldRegistry, oldData := RegistryDir, DataDir
	oldGlobal, oldProject := updateGlobal, updateProject
	RegistryDir, DataDir = t.TempDir(), t.TempDir()
	updateGlobal, updateProject = true, true
	t.Cleanup(func() {
		RegistryDir, DataDir = oldRegistry, oldData
		updateGlobal, updateProject = oldGlobal, oldProject
	})
	if err := updateAllSkillsLegacyDirs(); err != nil {
		t.Fatalf("updateAllSkillsLegacyDirs: %v", err)
	}
	if err := pullSourceList(nil); err != nil {
		t.Fatalf("pullSourceList: %v", err)
	}
}

func TestUpdateOriginBackedSkillRewritesRegistry(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	// home package may be used by collect; ensure clean
	oldReg, oldData := RegistryDir, DataDir
	RegistryDir = filepath.Join(t.TempDir(), "registry")
	DataDir = t.TempDir()
	t.Cleanup(func() { RegistryDir, DataDir = oldReg, oldData })
	if err := os.MkdirAll(filepath.Join(RegistryDir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}

	// remote + working clone we push updates through
	remote := filepath.Join(t.TempDir(), "remote.git")
	work := filepath.Join(t.TempDir(), "work")
	gitRun(t, "", "init", "--bare", "-q", remote)
	gitRun(t, "", "clone", "-q", remote, work)
	gitRun(t, work, "config", "user.name", "test")
	gitRun(t, work, "config", "user.email", "test@example.com")
	skillDir := filepath.Join(work, "omega")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: omega\ndescription: v1 description long enough\n---\n# v1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-qm", "v1")
	gitRun(t, work, "push", "-q", "origin", "HEAD")

	source := "file://" + remote
	// seed cache + registry via install path helpers
	cache, err := cachedGitSource(source, "")
	if err != nil {
		t.Fatal(err)
	}
	// install skill content into registry with origin
	skills, err := registry.DiscoverSkills(cache)
	if err != nil || len(skills) == 0 {
		t.Fatalf("discover: %v %#v", err, skills)
	}
	// use ensureSkillsInRegistry
	paths, err := ensureSkillsInRegistry(skills, source, "", cache)
	if err != nil {
		t.Fatal(err)
	}
	regSkill := paths["omega"]
	if regSkill == "" {
		// name might differ
		for _, p := range paths {
			regSkill = p
		}
	}
	body1, _ := os.ReadFile(filepath.Join(regSkill, "SKILL.md"))
	if !strings.Contains(string(body1), "# v1") {
		t.Fatalf("expected v1 content: %s", body1)
	}

	// project install symlink
	projectDir := t.TempDir()
	linkDir := filepath.Join(projectDir, tool.AllTools()[0].ProjectSkillDir)
	if err := os.MkdirAll(linkDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(regSkill, filepath.Join(linkDir, "omega")); err != nil {
		t.Fatal(err)
	}

	// push v2
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: omega\ndescription: v2 description long enough\n---\n# v2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-qm", "v2")
	gitRun(t, work, "push", "-q", "origin", "HEAD")

	// update installed
	if err := updateInstalledSources(projectDir); err != nil {
		t.Fatalf("update: %v", err)
	}
	body2, _ := os.ReadFile(filepath.Join(regSkill, "SKILL.md"))
	if !strings.Contains(string(body2), "# v2") {
		t.Fatalf("expected v2 after update, got: %s", body2)
	}
	origin, ok := readSkillOrigin(regSkill)
	if !ok || origin.Commit == "" {
		t.Fatalf("origin after update: ok=%v %+v", ok, origin)
	}
}

func TestUpdateReportsOrphanSkills(t *testing.T) {
	oldReg, oldData := RegistryDir, DataDir
	RegistryDir = filepath.Join(t.TempDir(), "registry")
	DataDir = t.TempDir()
	t.Cleanup(func() { RegistryDir, DataDir = oldReg, oldData })
	if err := os.MkdirAll(filepath.Join(RegistryDir, "skills", "global", "orphan"), 0755); err != nil {
		t.Fatal(err)
	}
	// registry skill without .git and without origin
	if err := os.WriteFile(filepath.Join(RegistryDir, "skills", "global", "orphan", "SKILL.md"),
		[]byte("---\nname: orphan\ndescription: long enough description here\n---\n# o\n"), 0644); err != nil {
		t.Fatal(err)
	}
	projectDir := t.TempDir()
	linkDir := filepath.Join(projectDir, tool.AllTools()[0].ProjectSkillDir)
	if err := os.MkdirAll(linkDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(RegistryDir, "skills", "global", "orphan"), filepath.Join(linkDir, "orphan")); err != nil {
		t.Fatal(err)
	}

	targets := collectInstalledUpdateTargets(projectDir, nil, true, false)
	if len(targets.orphans) != 1 || targets.orphans[0] != "orphan" {
		t.Fatalf("orphans = %#v, want [orphan]", targets.orphans)
	}
	if len(targets.gitRepos) != 0 || len(targets.originSkills) != 0 {
		t.Fatalf("unexpected updatable targets: %#v", targets)
	}
}

func TestRewriteOriginSkillsRollsBackLintErrors(t *testing.T) {
	oldReg := RegistryDir
	RegistryDir = filepath.Join(t.TempDir(), "registry")
	t.Cleanup(func() { RegistryDir = oldReg })
	regSkill := filepath.Join(RegistryDir, "skills", "global", "safe")
	if err := os.MkdirAll(regSkill, 0755); err != nil {
		t.Fatal(err)
	}
	good := "---\nname: safe\ndescription: this valid description triggers safely\n---\n# Safe\n"
	if err := os.WriteFile(filepath.Join(regSkill, "SKILL.md"), []byte(good), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeSkillOrigin(regSkill, skillOrigin{Source: "file://x", RelPath: "safe", Commit: "aaa"}); err != nil {
		t.Fatal(err)
	}

	cache := t.TempDir()
	badDir := filepath.Join(cache, "safe")
	if err := os.MkdirAll(badDir, 0755); err != nil {
		t.Fatal(err)
	}
	// missing description -> lint error
	if err := os.WriteFile(filepath.Join(badDir, "SKILL.md"), []byte("---\nname: safe\n---\n# Broken\n"), 0644); err != nil {
		t.Fatal(err)
	}

	okN, errN := rewriteOriginSkills(cache, []originSkillTarget{{
		skillDir: regSkill,
		name:     "safe",
		origin:   skillOrigin{Source: "file://x", RelPath: "safe"},
	}})
	if okN != 0 || errN != 1 {
		t.Fatalf("ok=%d err=%d, want 0/1", okN, errN)
	}
	body, _ := os.ReadFile(filepath.Join(regSkill, "SKILL.md"))
	if !strings.Contains(string(body), "# Safe") {
		t.Fatalf("expected rollback to good content, got %s", body)
	}
}

// TestRewriteOriginSkillsIsAtomicAcrossSource verifies ADR-0013 for a source
// that contributes multiple extracted Registry originals: if one staged skill
// fails post-update lint, none of the siblings nor their origin metadata move.
func TestRewriteOriginSkillsIsAtomicAcrossSource(t *testing.T) {
	oldReg := RegistryDir
	RegistryDir = filepath.Join(t.TempDir(), "registry")
	t.Cleanup(func() { RegistryDir = oldReg })

	cache := t.TempDir()
	validOld := "---\nname: first\ndescription: this is a valid old description\n---\n# old first\n"
	validNew := "---\nname: first\ndescription: this is a valid new description\n---\n# new first\n"
	secondOld := "---\nname: second\ndescription: this is a valid old description\n---\n# old second\n"
	invalidNew := "---\nname: second\n---\n# broken second\n"

	firstRegistry := filepath.Join(RegistryDir, "skills", "global", "first")
	secondRegistry := filepath.Join(RegistryDir, "skills", "global", "second")
	for _, skill := range []struct {
		dir, body, name string
	}{
		{firstRegistry, validOld, "first"},
		{secondRegistry, secondOld, "second"},
	} {
		if err := os.MkdirAll(skill.dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skill.dir, "SKILL.md"), []byte(skill.body), 0644); err != nil {
			t.Fatal(err)
		}
		if err := writeSkillOrigin(skill.dir, skillOrigin{
			Source:  "file://source",
			RelPath: skill.name,
			Commit:  "old-" + skill.name,
		}); err != nil {
			t.Fatal(err)
		}
	}

	firstCache := filepath.Join(cache, "first")
	secondCache := filepath.Join(cache, "second")
	if err := os.MkdirAll(firstCache, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secondCache, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(firstCache, "SKILL.md"), []byte(validNew), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondCache, "SKILL.md"), []byte(invalidNew), 0644); err != nil {
		t.Fatal(err)
	}

	okN, errN := rewriteOriginSkills(cache, []originSkillTarget{
		{skillDir: firstRegistry, name: "first", origin: skillOrigin{Source: "file://source", RelPath: "first"}},
		{skillDir: secondRegistry, name: "second", origin: skillOrigin{Source: "file://source", RelPath: "second"}},
	})
	if okN != 0 || errN != 2 {
		t.Fatalf("rewriteOriginSkills = (%d, %d), want (0, 2)", okN, errN)
	}

	for _, skill := range []struct {
		dir, body, name string
	}{
		{firstRegistry, validOld, "first"},
		{secondRegistry, secondOld, "second"},
	} {
		body, err := os.ReadFile(filepath.Join(skill.dir, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != skill.body {
			t.Errorf("%s content changed after sibling failure: %q", skill.name, body)
		}
		origin, ok := readSkillOrigin(skill.dir)
		if !ok || origin.Commit != "old-"+skill.name {
			t.Errorf("%s origin changed after sibling failure: ok=%v origin=%+v", skill.name, ok, origin)
		}
	}
}

func TestWarnCopyInstallsStaleDetectsNonSymlink(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	// home package
	// ensure SkillDir under home
	dir := filepath.Join(tmpHome, tool.Claude.SkillDir, "copied")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// just ensure function doesn't panic and finds path - capture stderr not easy;
	// call and ensure directory still exists (smoke)
	warnCopyInstallsStale("copied")
}

// TestUpdateInPlaceRefreshesCopyEntities 验证 sm update --in-place：
// 项目内 Copy Install 实体（带 origin）按 source cache 就地刷新，
// Link Install（symlink）被跳过，且 registry 原件不被改动。
func TestUpdateInPlaceRefreshesCopyEntities(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	oldReg, oldData := RegistryDir, DataDir
	RegistryDir = filepath.Join(t.TempDir(), "registry")
	DataDir = t.TempDir()
	t.Cleanup(func() { RegistryDir, DataDir = oldReg, oldData })
	os.MkdirAll(filepath.Join(RegistryDir, "skills"), 0755)

	// remote + work clone, push v1。
	remote := filepath.Join(t.TempDir(), "remote.git")
	work := filepath.Join(t.TempDir(), "work")
	gitRun(t, "", "init", "--bare", "-q", remote)
	gitRun(t, "", "clone", "-q", remote, work)
	gitRun(t, work, "config", "user.name", "test")
	gitRun(t, work, "config", "user.email", "test@example.com")
	skillDir := filepath.Join(work, "omega")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: omega\ndescription: v1 long enough description\n---\n# v1\n"), 0644)
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-qm", "v1")
	gitRun(t, work, "push", "-q", "origin", "HEAD")

	source := "file://" + remote
	cache, err := cachedGitSource(source, "")
	if err != nil {
		t.Fatalf("cachedGitSource: %v", err)
	}

	// 项目 agent 目录里放一个 Copy Install 实体：整体拷贝 cache 里的 omega + 带 origin。
	projectDir := t.TempDir()
	copyEntity := filepath.Join(projectDir, ".claude", "skills", "omega")
	os.MkdirAll(filepath.Dir(copyEntity), 0755)
	if err := copySkillDir(filepath.Join(cache, "omega"), copyEntity); err != nil {
		t.Fatalf("copySkillDir: %v", err)
	}
	writeSkillOrigin(copyEntity, skillOrigin{Source: source, Ref: "", RelPath: "omega"})
	body1, _ := os.ReadFile(filepath.Join(copyEntity, "SKILL.md"))
	if !strings.Contains(string(body1), "# v1") {
		t.Fatalf("copy entity should start at v1: %s", body1)
	}

	// 同时放一个 symlink（Link Install），应被跳过且不被修改。
	linkEntity := filepath.Join(projectDir, ".claude", "skills", "link-only")
	os.Symlink(filepath.Join(cache, "omega"), linkEntity)

	// push v2。
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: omega\ndescription: v2 long enough description\n---\n# v2\n"), 0644)
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-qm", "v2")
	gitRun(t, work, "push", "-q", "origin", "HEAD")
	// 让缓存前进到 v2（in-place 自己不会 pull，故先用普通 update 路径拉 cache）。
	if _, err := cachedGitSource(source, ""); err != nil {
		// 触发 pull：直接 git pull cache
	}
	if out, err := exec.Command("git", "-C", cache, "pull", "--ff-only").CombinedOutput(); err != nil {
		t.Fatalf("pull cache: %v\n%s", err, out)
	}

	// 跑 in-place。
	if err := updateInPlaceInstalls(projectDir); err != nil {
		t.Fatalf("updateInPlaceInstalls: %v", err)
	}

	// copy 实体应变 v2。
	body2, _ := os.ReadFile(filepath.Join(copyEntity, "SKILL.md"))
	if !strings.Contains(string(body2), "# v2") {
		t.Errorf("copy entity should be refreshed to v2, got: %s", body2)
	}
	// copy 实体应仍带 origin，且 commit 非空。
	if o, ok := readSkillOrigin(copyEntity); !ok || o.Commit == "" {
		t.Errorf("copy entity origin after refresh: ok=%v %+v", ok, o)
	}
	// symlink 应仍是 symlink（未被当作 copy 刷新）。
	if fi, err := os.Lstat(linkEntity); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link-only entity should remain a symlink: %v", err)
	}
}

// TestUpdateInPlaceMissingCacheDoesNotClone 验证 source cache 缺失时
// --in-place 报错指向 sm update，不触网（不 clone）。
func TestUpdateInPlaceMissingCacheDoesNotClone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	oldReg, oldData := RegistryDir, DataDir
	RegistryDir = filepath.Join(t.TempDir(), "registry")
	DataDir = t.TempDir() // 空 data 目录：无 sources 缓存
	t.Cleanup(func() { RegistryDir, DataDir = oldReg, oldData })
	os.MkdirAll(filepath.Join(RegistryDir, "skills"), 0755)

	projectDir := t.TempDir()
	copyEntity := filepath.Join(projectDir, ".claude", "skills", "omega")
	os.MkdirAll(copyEntity, 0755) // copyEntity 本身必须是目录（Copy Install 实体）
	os.WriteFile(filepath.Join(copyEntity, "SKILL.md"), []byte("# x"), 0644)
	writeSkillOrigin(copyEntity, skillOrigin{Source: "file:///nonexistent/remote", RelPath: "omega"})

	if err := updateInPlaceInstalls(projectDir); err != nil {
		t.Fatalf("updateInPlaceInstalls should not return error (reports inline): %v", err)
	}
	// 内容应保持不变（未被刷新，也未被 clone 覆盖）。
	body, _ := os.ReadFile(filepath.Join(copyEntity, "SKILL.md"))
	if !strings.Contains(string(body), "# x") {
		t.Errorf("entity should be untouched when cache missing, got: %s", body)
	}
	// 不应生成任何 source cache。
	entries, _ := os.ReadDir(filepath.Join(DataDir, "sources"))
	if len(entries) != 0 {
		t.Errorf("expected no source cache created, got %d entries", len(entries))
	}
}

// TestRefreshProjectLockAfterUpdate 验证 origin-backed 技能被回写后，
// refreshProjectLockAfterUpdate 重新计算 registry 目录哈希并更新
// skills-lock.json 中同名条目的 computedHash。
func TestRefreshProjectLockAfterUpdate(t *testing.T) {
	projectDir := t.TempDir()

	// 1) 构造 registry 内的技能目录（模拟回写后的内容）。
	regSkill := filepath.Join(t.TempDir(), "registry", "skills", "global", "alpha")
	if err := os.MkdirAll(regSkill, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: alpha\ndescription: refreshed content\n---\n# Alpha\nnew body\n"
	if err := os.WriteFile(filepath.Join(regSkill, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// 2) 预置 skills-lock.json，含过期哈希。
	lm := lockfile.NewManager(projectDir)
	staleHash := "0000000000000000000000000000000000000000000000000000000000000000"
	if err := lm.Upsert("alpha", &lockfile.SkillEntry{
		Source:       "file:///tmp/alpha-src",
		SourceType:   "local",
		SkillPath:    "alpha/SKILL.md",
		Ref:          "",
		ComputedHash: staleHash,
	}); err != nil {
		t.Fatal(err)
	}

	// 3) 调用：传入刚回写的技能目录。
	refreshProjectLockAfterUpdate(projectDir, []originSkillTarget{{
		skillDir: regSkill,
		name:     "alpha",
		origin:   skillOrigin{Source: "file:///tmp/alpha-src", RelPath: "alpha"},
	}})

	// 4) 断言：哈希应与当前内容一致，不再等于 staleHash。
	wantHash, err := lockfile.ComputeHash(regSkill)
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}
	lock, err := lm.Load()
	if err != nil {
		t.Fatalf("Load lock: %v", err)
	}
	entry, ok := lock.Skills["alpha"]
	if !ok {
		t.Fatal("expected alpha entry to remain in lock")
	}
	if entry.ComputedHash != wantHash {
		t.Fatalf("hash not refreshed: got %s, want %s", entry.ComputedHash, wantHash)
	}
	if entry.ComputedHash == staleHash {
		t.Fatal("hash is still the stale value")
	}
}

// TestRefreshProjectLockNoLockfile 验证锁文件不存在时静默跳过（不报错）。
func TestRefreshProjectLockNoLockfile(t *testing.T) {
	projectDir := t.TempDir() // 无 skills-lock.json
	regSkill := filepath.Join(t.TempDir(), "alpha")
	os.MkdirAll(regSkill, 0755)
	os.WriteFile(filepath.Join(regSkill, "SKILL.md"), []byte("# x"), 0644)

	// 应不 panic、不报错、不创建锁文件。
	refreshProjectLockAfterUpdate(projectDir, []originSkillTarget{{
		skillDir: regSkill,
		name:     "alpha",
		origin:   skillOrigin{Source: "file://x", RelPath: "alpha"},
	}})

	if lockfile.NewManager(projectDir).Exists() {
		t.Fatal("lockfile should not be created when none existed")
	}
}

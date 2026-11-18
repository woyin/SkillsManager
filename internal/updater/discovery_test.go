package updater

// discovery_test.go 覆盖已安装技能发现与分类：git 仓库、legacy origin 技能、
// 按名过滤、去重、Well-Known 消除孤儿、扫描失败诊断，以及
// PathInside/NearestGitRepo 的边界语义（Windows CI 运行后两项）。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/woyin/skills-manager/internal/lockfile"
	"github.com/woyin/skills-manager/internal/tool"
)

// discTempRoot 返回规范化（EvalSymlinks 后）的临时根目录：发现路径比较的是
// symlink 解析后的路径，而 macOS 的 t.TempDir() 带 /var 符号链接。
func discTempRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// discWriteSkill 在 dir 下写 SKILL.md。
func discWriteSkill(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: discovery fixture skill\n---\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// discWriteOrigin 写入 legacy .sm-origin.json。
func discWriteOrigin(t *testing.T, dir string, meta OriginMetadata) {
	t.Helper()
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(LegacyOriginPath(dir), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// discProjectDirs 返回 projectDir 下所有工具的已装技能目录（跳过未配置项）。
// 多个工具可能共用同一目录（如 .agents/skills），因此去重。
func discProjectDirs(t *testing.T, projectDir string) []string {
	t.Helper()
	var dirs []string
	seen := map[string]bool{}
	for _, tl := range tool.AllTools() {
		d := tool.GetProjectSkillDir(tl, projectDir)
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		dirs = append(dirs, d)
	}
	if len(dirs) == 0 {
		t.Fatal("no tool defines a project skill dir")
	}
	return dirs
}

// discLinkAll 在所有已装技能目录下建同名条目：target 非空则建 symlink。
func discLinkAll(t *testing.T, projectDir, name, target string) {
	t.Helper()
	for _, dir := range discProjectDirs(t, projectDir) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
}

// TestDiscoverInstalledTargetsClassifiesInstalledEntries 用同一锁定 fixture
// 覆盖四类目标与两类跳过：受管 symlink → git/origin，copy 实体 → 查 registry 原件，
// 无 git 无 origin → 孤儿，不受管 symlink → 不收集。
func TestDiscoverInstalledTargetsClassifiesInstalledEntries(t *testing.T) {
	root := discTempRoot(t)
	registryDir := filepath.Join(root, "registry")
	dataDir := filepath.Join(root, "data")
	projectDir := filepath.Join(root, "project")
	updatedAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	gitSkill := filepath.Join(registryDir, "skills", "global", "git-skill")
	discWriteSkill(t, gitSkill, "git-skill")
	if err := os.MkdirAll(filepath.Join(gitSkill, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	originSkill := filepath.Join(registryDir, "skills", "global", "origin-skill")
	discWriteSkill(t, originSkill, "origin-skill")
	discWriteOrigin(t, originSkill, OriginMetadata{
		Source: "https://example.com/repo", Ref: "main", RelPath: "skills/origin-skill", Commit: "abc123", UpdatedAt: updatedAt,
	})

	copySkill := filepath.Join(registryDir, "skills", "global", "copy-skill")
	discWriteSkill(t, copySkill, "copy-skill")
	discWriteOrigin(t, copySkill, OriginMetadata{Source: "https://example.com/copy", RelPath: "copy-skill", Commit: "def456"})

	orphanSkill := filepath.Join(registryDir, "skills", "global", "orphan-skill")
	discWriteSkill(t, orphanSkill, "orphan-skill")

	outside := filepath.Join(root, "outside", "unmanaged")
	discWriteSkill(t, outside, "unmanaged")

	discLinkAll(t, projectDir, "git-skill", gitSkill)
	discLinkAll(t, projectDir, "origin-skill", originSkill)
	discLinkAll(t, projectDir, "orphan-skill", orphanSkill)
	discLinkAll(t, projectDir, "unmanaged", outside)
	// copy 安装：agent 目录里是实体目录，需回查 registry 同名原件。
	for _, dir := range discProjectDirs(t, projectDir) {
		if err := os.MkdirAll(filepath.Join(dir, "copy-skill"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	targets, diags := DiscoverInstalledTargets(DiscoverOptions{
		Options:        Options{RegistryDir: registryDir, DataDir: dataDir},
		ProjectDir:     projectDir,
		IncludeProject: true,
	})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}

	if len(targets.GitRepos) != 1 || targets.GitRepos[0] != gitSkill {
		t.Fatalf("GitRepos = %#v, want [%s]", targets.GitRepos, gitSkill)
	}
	if len(targets.Origins) != 2 {
		t.Fatalf("Origins = %#v, want 2 entries (origin-skill, copy-skill)", targets.Origins)
	}
	byName := map[string]DiscoveredOrigin{}
	for _, o := range targets.Origins {
		byName[o.Name] = o
	}
	if got := byName["origin-skill"]; got.SkillDir != originSkill || got.Metadata.Commit != "abc123" || !got.Metadata.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("origin-skill = %+v, want SkillDir=%s commit=abc123 updatedAt=%s", got, originSkill, updatedAt)
	}
	if got := byName["copy-skill"]; got.SkillDir != copySkill || got.Metadata.Source != "https://example.com/copy" {
		t.Fatalf("copy-skill = %+v, want registry original %s", got, copySkill)
	}
	if len(targets.Orphans) != 1 || targets.Orphans[0] != "orphan-skill" {
		t.Fatalf("Orphans = %#v, want [orphan-skill]", targets.Orphans)
	}
	if len(targets.WellKnownSkills) != 0 {
		t.Fatalf("WellKnownSkills = %#v, want none", targets.WellKnownSkills)
	}
	if targets.Empty() {
		t.Fatal("Empty() = true, want false")
	}
}

// TestDiscoverInstalledTargetsNamesFilter 只收集按名指定的技能。
func TestDiscoverInstalledTargetsNamesFilter(t *testing.T) {
	root := discTempRoot(t)
	registryDir := filepath.Join(root, "registry")
	projectDir := filepath.Join(root, "project")

	gitSkill := filepath.Join(registryDir, "skills", "global", "git-skill")
	discWriteSkill(t, gitSkill, "git-skill")
	if err := os.MkdirAll(filepath.Join(gitSkill, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	originSkill := filepath.Join(registryDir, "skills", "global", "origin-skill")
	discWriteSkill(t, originSkill, "origin-skill")
	discWriteOrigin(t, originSkill, OriginMetadata{Source: "https://example.com/repo", RelPath: "origin-skill"})

	discLinkAll(t, projectDir, "git-skill", gitSkill)
	discLinkAll(t, projectDir, "origin-skill", originSkill)

	targets, _ := DiscoverInstalledTargets(DiscoverOptions{
		Options:        Options{RegistryDir: registryDir, DataDir: filepath.Join(root, "data")},
		ProjectDir:     projectDir,
		Names:          []string{"origin-skill"},
		IncludeProject: true,
	})
	if len(targets.Origins) != 1 || targets.Origins[0].Name != "origin-skill" {
		t.Fatalf("Origins = %#v, want [origin-skill]", targets.Origins)
	}
	if len(targets.GitRepos) != 0 || len(targets.Orphans) != 0 {
		t.Fatalf("unexpected targets: %+v", targets)
	}
}

// TestDiscoverInstalledTargetsWellKnownEliminatesOrphan 验证锁文件里的
// Well-Known Source 技能进入 WellKnownSkills，并从孤儿列表中移除。
func TestDiscoverInstalledTargetsWellKnownEliminatesOrphan(t *testing.T) {
	root := discTempRoot(t)
	projectDir := filepath.Join(root, "project")
	registryDir := filepath.Join(root, "registry")

	wellKnownSkill := filepath.Join(registryDir, "skills", "global", "wk-skill")
	discWriteSkill(t, wellKnownSkill, "wk-skill")
	discLinkAll(t, projectDir, "wk-skill", wellKnownSkill)

	lock := &lockfile.LocalLock{Version: 1, Skills: map[string]*lockfile.SkillEntry{
		"wk-skill": {Source: "https://example.com/wk", SourceType: "well-known", SourceURL: "https://example.com/wk/skills", ComputedHash: "hash"},
	}}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "skills-lock.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	targets, diags := DiscoverInstalledTargets(DiscoverOptions{
		Options:        Options{RegistryDir: registryDir, DataDir: filepath.Join(root, "data")},
		ProjectDir:     projectDir,
		IncludeProject: true,
	})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if len(targets.WellKnownSkills) != 1 || targets.WellKnownSkills[0].Name != "wk-skill" ||
		targets.WellKnownSkills[0].Source != "https://example.com/wk/skills" {
		t.Fatalf("WellKnownSkills = %#v", targets.WellKnownSkills)
	}
	if len(targets.Orphans) != 0 {
		t.Fatalf("Orphans = %#v, want none (Well-Known entry eliminates orphan)", targets.Orphans)
	}
}

// TestDiscoverInstalledTargetsReportsUnreadableDir 验证扫描失败被记录为诊断、
// 不中断发现（保留既有跳过行为）。
func TestDiscoverInstalledTargetsReportsUnreadableDir(t *testing.T) {
	root := discTempRoot(t)
	projectDir := filepath.Join(root, "project")
	dirs := discProjectDirs(t, projectDir)
	// 把第一个项目级技能目录做成普通文件：ReadDir 返回 ENOTDIR。
	if err := os.MkdirAll(filepath.Dir(dirs[0]), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dirs[0], []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}

	targets, diags := DiscoverInstalledTargets(DiscoverOptions{
		Options:        Options{RegistryDir: filepath.Join(root, "registry"), DataDir: filepath.Join(root, "data")},
		ProjectDir:     projectDir,
		IncludeProject: true,
	})
	found := false
	for _, diag := range diags {
		if diag.Op != "read-dir" || diag.Err == nil {
			t.Fatalf("diagnostic = %+v, want op=read-dir with error", diag)
		}
		if diag.Path == dirs[0] {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %#v, want one for %s", diags, dirs[0])
	}
	if !targets.Empty() {
		t.Fatalf("targets = %+v, want empty", targets)
	}
}

// TestNearestGitRepo 覆盖上界、跨越 root 与无 .git 三种情形。
func TestNearestGitRepo(t *testing.T) {
	root := discTempRoot(t)
	bounded := filepath.Join(root, "bounded")
	repo := filepath.Join(bounded, "skills", "a")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(repo, "nested", "dir")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}

	if got := NearestGitRepo(deep, bounded); got != repo {
		t.Fatalf("NearestGitRepo inside bound = %q, want %q", got, repo)
	}
	// 上界收窄到下界之外的 root：穿越 root 时返回空。
	if got := NearestGitRepo(deep, filepath.Join(bounded, "other")); got != "" {
		t.Fatalf("NearestGitRepo outside bound = %q, want empty", got)
	}
	// 无 .git 的目录树返回空。
	plain := filepath.Join(root, "plain")
	if err := os.MkdirAll(filepath.Join(plain, "x"), 0755); err != nil {
		t.Fatal(err)
	}
	if got := NearestGitRepo(filepath.Join(plain, "x"), plain); got != "" {
		t.Fatalf("NearestGitRepo without .git = %q, want empty", got)
	}
}

// TestPathInside 覆盖包含判定与 "../" 前缀陷阱。
func TestPathInside(t *testing.T) {
	root := discTempRoot(t)
	inside := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(inside, 0755); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(root, "ab")
	if err := os.MkdirAll(sibling, 0755); err != nil {
		t.Fatal(err)
	}

	if !PathInside(inside, filepath.Join(root, "a")) {
		t.Fatal("subdirectory must be inside root")
	}
	if !PathInside(inside, inside) {
		t.Fatal("root must be inside itself")
	}
	if PathInside(sibling, filepath.Join(root, "a")) {
		t.Fatal("sibling sharing a prefix must not be inside root")
	}
	if PathInside(filepath.Join(root, "missing"), filepath.Join(root, "a")) {
		t.Fatal("path outside root must not be inside")
	}
}

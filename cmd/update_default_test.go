package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/registry"
)

func withTestRegistryUpdate2(t *testing.T) (regDir, dataDir string) {
	t.Helper()
	regDir = filepath.Join(t.TempDir(), "registry")
	os.MkdirAll(filepath.Join(regDir, "skills"), 0755)
	dataDir = filepath.Join(t.TempDir(), "data")
	oldReg, oldData := RegistryDir, DataDir
	RegistryDir, DataDir = regDir, dataDir
	t.Cleanup(func() { RegistryDir, DataDir = oldReg, oldData })
	return
}

func TestUpdateBareDefaultsToEntireRegistry(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	regDir, _ := withTestRegistryUpdate2(t)
	reg := registry.New(regDir)

	snapSrc := filepath.Join(t.TempDir(), "snap")
	os.MkdirAll(snapSrc, 0755)
	os.WriteFile(filepath.Join(snapSrc, "SKILL.md"), []byte("---\nname: snap\ndescription: a snapshot skill\n---\n# x"), 0644)
	if _, err := reg.Register(snapSrc, "", registry.SkillOrigin{
		SourceKind: registry.SourceLocalSnapshot, Source: snapSrc,
	}, false); err != nil {
		t.Fatal(err)
	}

	orphanDir := filepath.Join(regDir, "skills", "global", "orphan-one")
	os.MkdirAll(orphanDir, 0755)
	os.WriteFile(filepath.Join(orphanDir, "SKILL.md"), []byte("# orphan"), 0644)

	err := updateAllSkills()
	if err == nil {
		t.Fatal("expected non-zero error because orphan has no provenance")
	}
}

func TestUpdateSnapshotSkippedCleanly(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	regDir, _ := withTestRegistryUpdate2(t)
	reg := registry.New(regDir)

	src := filepath.Join(t.TempDir(), "only-snap")
	os.MkdirAll(src, 0755)
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: only-snap\ndescription: snapshot only\n---\n# x"), 0644)
	if _, err := reg.Register(src, "", registry.SkillOrigin{
		SourceKind: registry.SourceLocalSnapshot, Source: src,
	}, false); err != nil {
		t.Fatal(err)
	}

	if err := updateAllSkills(); err != nil {
		t.Errorf("snapshot-only registry should update cleanly, got: %v", err)
	}
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// makeGitRepo 造一个本地 git 仓库（路径以 .git 结尾，使 IsGitURL 命中 git 来源），
// 内含 skills/<name>/SKILL.md；默认分支 main，打 tag v1.0.0。
func makeGitRepo(t *testing.T, name string) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), name+".git")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, repo, "init", "-q", "-b", "main")
	gitCmd(t, repo, "config", "user.email", "t@example.com")
	gitCmd(t, repo, "config", "user.name", "T")
	os.MkdirAll(filepath.Join(repo, "skills", name), 0755)
	os.WriteFile(filepath.Join(repo, "skills", name, "SKILL.md"),
		[]byte("---\nname: "+name+"\ndescription: git skill\n---\n# v1\n"), 0644)
	gitCmd(t, repo, "add", "-A")
	gitCmd(t, repo, "commit", "-qm", "v1")
	gitCmd(t, repo, "tag", "v1.0.0")
	return repo
}

func readOriginFile(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(path, registry.OriginFile))
	if err != nil {
		t.Fatalf("reading origin: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parsing origin: %v", err)
	}
	return m
}

// TestUpdatePreservesRegistrySchemaOrigin 验证 sm update 回写 Registry 原件时
// 使用 schema v1（source_kind/ref_kind 不丢失）—— 回归：旧实现用 writeSkillOrigin
// 回写成旧格式，导致 pinned/branch 语义在 update 后丢失。
func TestUpdatePreservesRegistrySchemaOrigin(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	regDir, _ := withTestRegistryUpdate2(t)
	repo := makeGitRepo(t, "git-skill")

	if err := registerFromSource(registry.New(regDir), repo, "", nil, false, "", false); err != nil {
		t.Fatalf("registerFromSource: %v", err)
	}
	skillDir := filepath.Join(regDir, "skills", "global", "git-skill")
	before := readOriginFile(t, skillDir)
	if before["schema_version"] != float64(1) || before["source_kind"] != "git" || before["ref_kind"] != "default-branch" {
		t.Fatalf("unexpected origin before update: %v", before)
	}

	if err := updateAllSkills(); err != nil {
		t.Fatalf("updateAllSkills: %v", err)
	}
	after := readOriginFile(t, skillDir)
	if after["schema_version"] != float64(1) || after["source_kind"] != "git" || after["ref_kind"] != "default-branch" {
		t.Errorf("origin lost schema after update: %v", after)
	}
}

// TestUpdateTracksNamedBranch 验证显式 branch 是 tracking（ADR 0014）：
// branch 前进后 update 会拉取新内容，而不是按 pinned 跳过。
func TestUpdateTracksNamedBranch(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	regDir, _ := withTestRegistryUpdate2(t)
	repo := makeGitRepo(t, "branch-skill")
	gitCmd(t, repo, "checkout", "-q", "-b", "feature")

	if err := registerFromSource(registry.New(regDir), repo, "", nil, false, "feature", false); err != nil {
		t.Fatalf("registerFromSource: %v", err)
	}
	skillDir := filepath.Join(regDir, "skills", "global", "branch-skill")
	origin := readOriginFile(t, skillDir)
	if origin["ref_kind"] != "branch" || origin["ref"] != "feature" {
		t.Fatalf("expected branch origin, got: %v", origin)
	}

	// 推进 feature 分支。
	gitCmd(t, repo, "checkout", "-q", "feature")
	gitCmd(t, repo, "config", "user.email", "t@example.com")
	gitCmd(t, repo, "config", "user.name", "T")
	f, err := os.OpenFile(filepath.Join(repo, "skills", "branch-skill", "SKILL.md"), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("# v2\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	gitCmd(t, repo, "add", "-A")
	gitCmd(t, repo, "commit", "-qm", "v2")

	if err := updateAllSkills(); err != nil {
		t.Fatalf("updateAllSkills: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# v2") {
		t.Errorf("branch-tracking skill not advanced after update; content: %s", data)
	}
}

// TestUpdatePinnedTagSkipped 验证 tag/commit 是 pinned：update 健康跳过，
// 且 origin 的 ref_kind=tag 在 update 后保留。
func TestUpdatePinnedTagSkipped(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	regDir, _ := withTestRegistryUpdate2(t)
	repo := makeGitRepo(t, "pin-skill")

	if err := registerFromSource(registry.New(regDir), repo, "", nil, false, "v1.0.0", false); err != nil {
		t.Fatalf("registerFromSource: %v", err)
	}
	skillDir := filepath.Join(regDir, "skills", "global", "pin-skill")
	origin := readOriginFile(t, skillDir)
	if origin["ref_kind"] != "tag" || origin["ref"] != "v1.0.0" {
		t.Fatalf("expected tag origin, got: %v", origin)
	}

	// 推进 main 分支（pinned 不应前进）。
	gitCmd(t, repo, "config", "user.email", "t@example.com")
	gitCmd(t, repo, "config", "user.name", "T")
	f, err := os.OpenFile(filepath.Join(repo, "skills", "pin-skill", "SKILL.md"), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("# v2\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	gitCmd(t, repo, "add", "-A")
	gitCmd(t, repo, "commit", "-qm", "v2")

	if err := updateAllSkills(); err != nil {
		t.Fatalf("updateAllSkills: %v", err)
	}
	after := readOriginFile(t, skillDir)
	if after["ref_kind"] != "tag" {
		t.Errorf("pinned origin ref_kind lost after update: %v", after)
	}
	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "# v2") {
		t.Errorf("pinned skill advanced past its tag: %s", data)
	}
}

// TestUpdateSourceIsolation 验证 Registry 更新按 Source 隔离（ADR 0013）：
// 一个 Source 的 cache 损坏/失败时，其它 Source 仍正常更新，且最终退出码非零。
func TestUpdateSourceIsolation(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	regDir, _ := withTestRegistryUpdate2(t)
	reg := registry.New(regDir)

	repoA := makeGitRepo(t, "iso-a")
	repoB := makeGitRepo(t, "iso-b")
	if err := registerFromSource(reg, repoA, "", nil, false, "", false); err != nil {
		t.Fatalf("register iso-a: %v", err)
	}
	if err := registerFromSource(reg, repoB, "", nil, false, "", false); err != nil {
		t.Fatalf("register iso-b: %v", err)
	}

	// 破坏 repoA 的 source cache：加入未提交改动 → pull 前被 gitDirty 拦截。
	cacheDir, _ := sourceCachePaths(repoA, "")
	if _, err := os.Stat(cacheDir); err != nil {
		t.Fatalf("source cache for A missing: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "dirty.txt"), []byte("dirty"), 0644); err != nil {
		t.Fatal(err)
	}

	err := updateAllSkills()
	if err == nil {
		t.Fatal("expected non-zero error because source A failed")
	}

	// B 仍更新成功（其 skill 内容正常）。
	dataB, err := os.ReadFile(filepath.Join(regDir, "skills", "global", "iso-b", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dataB), "# v1") {
		t.Errorf("source B should have been updated despite A failing; content: %s", dataB)
	}
	// A 保持旧内容（未被部分改写）。
	dataA, err := os.ReadFile(filepath.Join(regDir, "skills", "global", "iso-a", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dataA), "# v1") {
		t.Errorf("source A content should remain its prior valid version; content: %s", dataA)
	}
}

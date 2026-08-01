package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/registry"
)

// TestRmRefusesWhileReferenced verifies sm rm refuses to delete a Registry
// original that is still referenced by an install (ADR 0017).
func TestRmRefusesWhileReferenced(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	regDir := withTestRegistry(t)
	projectDir := t.TempDir()
	rmDir = projectDir
	t.Cleanup(func() { rmDir = "" })

	src := t.TempDir()
	makeValidSkillDir(t, src, "protected")
	reg := registry.New(regDir)
	res, err := reg.Register(filepath.Join(src, "protected"), "",
		registry.SkillOrigin{SourceKind: registry.SourceLocalSnapshot, Source: src}, false)
	if err != nil {
		t.Fatal(err)
	}
	// Create a project install referencing it.
	installDir := filepath.Join(projectDir, ".claude", "skills")
	os.MkdirAll(installDir, 0755)
	link := filepath.Join(installDir, "protected")
	if err := os.Symlink(res.Path, link); err != nil {
		t.Fatal(err)
	}

	rmForce = false
	err = removeRegistryOriginal("protected")
	if err == nil {
		t.Fatal("expected refusal while referenced")
	}
	// Registry original must still exist.
	if _, err := os.Stat(res.Path); err != nil {
		t.Errorf("registry original should be kept on refusal: %v", err)
	}
}

// TestRmForceRemovesInstallsAndOriginal verifies --force cleans installs then
// deletes the registry original (ADR 0017).
func TestRmForceRemovesInstallsAndOriginal(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	regDir := withTestRegistry(t)
	projectDir := t.TempDir()
	rmDir = projectDir
	t.Cleanup(func() { rmDir = "" })

	src := t.TempDir()
	makeValidSkillDir(t, src, "forced")
	reg := registry.New(regDir)
	res, err := reg.Register(filepath.Join(src, "forced"), "",
		registry.SkillOrigin{SourceKind: registry.SourceLocalSnapshot, Source: src}, false)
	if err != nil {
		t.Fatal(err)
	}
	installDir := filepath.Join(projectDir, ".claude", "skills")
	os.MkdirAll(installDir, 0755)
	link := filepath.Join(installDir, "forced")
	if err := os.Symlink(res.Path, link); err != nil {
		t.Fatal(err)
	}

	rmForce = true
	t.Cleanup(func() { rmForce = false })
	if err := removeRegistryOriginal("forced"); err != nil {
		t.Fatalf("force remove: %v", err)
	}
	// Install link removed.
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("install link should be removed, got %v", err)
	}
	// Registry original removed.
	if _, err := os.Stat(res.Path); !os.IsNotExist(err) {
		t.Errorf("registry original should be deleted, got %v", err)
	}
}

// TestRmForceReportsInaccessibleHistoricalProjects 验证 --force 删除 Registry 原件时
// 明确报告数据库中已知但当前不可访问的历史项目（ADR 0017）。
func TestRmForceReportsInaccessibleHistoricalProjects(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	regDir, dataDir := withTestRegistryUpdate2(t)
	rmForce = true
	t.Cleanup(func() { rmForce = false })

	src := t.TempDir()
	makeValidSkillDir(t, src, "hist-skill")
	reg := registry.New(regDir)
	res, err := reg.Register(filepath.Join(src, "hist-skill"), "",
		registry.SkillOrigin{SourceKind: registry.SourceLocalSnapshot, Source: src}, false)
	if err != nil {
		t.Fatal(err)
	}

	// 造一个 DB 记录：指向不存在的项目路径。
	db, err := openDB()
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "gone-project")
	if err := db.UpsertProject(missing, "dev", []string{"hist-skill"}, nil); err != nil {
		t.Fatalf("UpsertProject: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// 捕获 stderr。
	var buf bytes.Buffer
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	removeErr := removeRegistryOriginal("hist-skill")
	w.Close()
	os.Stderr = oldStderr
	io.Copy(&buf, r)

	if removeErr != nil {
		t.Fatalf("removeRegistryOriginal: %v", removeErr)
	}
	if !strings.Contains(buf.String(), "inaccessible") || !strings.Contains(buf.String(), "gone-project") {
		t.Errorf("expected inaccessible historical project report, got: %s", buf.String())
	}
	if _, err := os.Stat(res.Path); !os.IsNotExist(err) {
		t.Errorf("registry original should be deleted after --force: %v", err)
	}
	_ = dataDir
}

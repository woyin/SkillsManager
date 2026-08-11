package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/tool"
)

func TestCheckSymlinksReportsAndFixesBrokenAndOrphanedLinks(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	home.ResetForTest()
	oldRegistry, oldFix := RegistryDir, checkFix
	RegistryDir, checkFix = t.TempDir(), false
	t.Cleanup(func() { RegistryDir, checkFix = oldRegistry, oldFix })

	dir := filepath.Join(tmpHome, tool.Claude.SkillDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), filepath.Join(dir, "broken")); err != nil {
		t.Fatal(err)
	}
	orphanTarget := filepath.Join(t.TempDir(), "external-skill")
	if err := os.MkdirAll(orphanTarget, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(orphanTarget, filepath.Join(dir, "orphan")); err != nil {
		t.Fatal(err)
	}

	if issues, err := checkSymlinks(); err != nil || issues != 2 {
		t.Fatalf("checkSymlinks = %d, %v", issues, err)
	}
	checkFix = true
	if issues, err := checkSymlinks(); err != nil || issues != 2 {
		t.Fatalf("fixed checkSymlinks = %d, %v", issues, err)
	}
	for _, name := range []string{"broken", "orphan"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s should be removed: %v", name, err)
		}
	}
}

func TestCheckProjectRecordsReportsAndFixesMissingProject(t *testing.T) {
	oldData, oldFix := DataDir, checkFix
	DataDir, checkFix = t.TempDir(), false
	t.Cleanup(func() { DataDir, checkFix = oldData, oldFix })
	database, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing-project")
	if err := database.UpsertProject(missing, "", nil, nil); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	if issues, err := checkProjectRecords(); err != nil || issues != 1 {
		t.Fatalf("checkProjectRecords = %d, %v", issues, err)
	}
	checkFix = true
	if issues, err := checkProjectRecords(); err != nil || issues != 1 {
		t.Fatalf("fixed checkProjectRecords = %d, %v", issues, err)
	}
	database, err = openDB()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	projects, err := database.GetAllProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Fatalf("stale project remained: %+v", projects)
	}
}

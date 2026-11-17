package updater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/lockfile"
	"github.com/woyin/skills-manager/internal/registry"
)

func TestRefreshProjectLockUpdatesHashAndSkipsMissingEntries(t *testing.T) {
	project := t.TempDir()
	skillDir := filepath.Join(t.TempDir(), "skill-a")
	writeSkillFile(t, skillDir, validSkillV1)

	lm := lockfile.NewManager(project)
	if err := lm.Save(&lockfile.LocalLock{Skills: map[string]*lockfile.SkillEntry{
		"skill-a": {Source: "src", ComputedHash: "old"},
		"other":   {Source: "src", ComputedHash: "keep"},
	}}); err != nil {
		t.Fatalf("seed lock: %v", err)
	}

	res := RefreshProjectLock(project, []OriginTarget{
		{Name: "skill-a", SkillDir: skillDir, Origin: registry.SkillOrigin{Ref: "v1"}},
		{Name: "missing", SkillDir: filepath.Join(t.TempDir(), "nope")},
	})
	if res.Err != nil {
		t.Fatalf("RefreshProjectLock: %v", res.Err)
	}
	if res.Updated != 1 {
		t.Fatalf("updated = %d, want 1", res.Updated)
	}

	reloaded, err := lockfile.NewManager(project).Load()
	if err != nil {
		t.Fatalf("reload lock: %v", err)
	}
	wantHash, err := lockfile.ComputeHash(skillDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Skills["skill-a"]; got.ComputedHash != wantHash || got.Ref != "v1" {
		t.Fatalf("skill-a entry = %+v", got)
	}
	if got := reloaded.Skills["other"]; got.ComputedHash != "keep" {
		t.Fatalf("unrelated entry changed: %+v", got)
	}
	if _, ok := reloaded.Skills["missing"]; ok {
		t.Fatal("missing entry must not be created")
	}
}

func TestRefreshProjectLockReportsSaveError(t *testing.T) {
	project := t.TempDir()
	skillDir := filepath.Join(t.TempDir(), "skill-a")
	writeSkillFile(t, skillDir, validSkillV1)
	lm := lockfile.NewManager(project)
	if err := lm.Save(&lockfile.LocalLock{Skills: map[string]*lockfile.SkillEntry{
		"skill-a": {Source: "src", ComputedHash: "old"},
	}}); err != nil {
		t.Fatalf("seed lock: %v", err)
	}

	// 锁文件仍可读，但目录不可写：Save 的临时文件写入失败。
	if err := os.Chmod(project, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(project, 0755) })
	probe := filepath.Join(project, "probe.tmp")
	if err := os.WriteFile(probe, []byte("x"), 0644); err == nil {
		_ = os.Remove(probe)
		t.Skip("directory stays writable (root or non-POSIX permissions)")
	}

	res := RefreshProjectLock(project, []OriginTarget{{Name: "skill-a", SkillDir: skillDir, Origin: registry.SkillOrigin{Ref: "v1"}}})
	if res.Err == nil || !strings.Contains(res.Err.Error(), "writing") {
		t.Fatalf("Err = %v, want a writing skills-lock.json error", res.Err)
	}
	if res.Updated != 0 {
		t.Fatalf("Updated = %d, want 0 on save failure", res.Updated)
	}
}

func TestRefreshProjectLockPersistsGoodEntriesWhenOneRehashFails(t *testing.T) {
	project := t.TempDir()
	goodDir := filepath.Join(t.TempDir(), "good")
	writeSkillFile(t, goodDir, validSkillV1)
	lm := lockfile.NewManager(project)
	if err := lm.Save(&lockfile.LocalLock{Skills: map[string]*lockfile.SkillEntry{
		"good": {Source: "src", ComputedHash: "old"},
		"bad":  {Source: "src", ComputedHash: "old"},
	}}); err != nil {
		t.Fatalf("seed lock: %v", err)
	}

	res := RefreshProjectLock(project, []OriginTarget{
		{Name: "good", SkillDir: goodDir},
		{Name: "bad", SkillDir: filepath.Join(t.TempDir(), "missing")},
	})
	if res.Err == nil || !strings.Contains(res.Err.Error(), "rehashing bad") {
		t.Fatalf("Err = %v, want rehash failure for bad", res.Err)
	}
	if res.Updated != 1 {
		t.Fatalf("Updated = %d, want 1 persisted entry", res.Updated)
	}

	reloaded, err := lockfile.NewManager(project).Load()
	if err != nil {
		t.Fatalf("reload lock: %v", err)
	}
	wantHash, err := lockfile.ComputeHash(goodDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Skills["good"].ComputedHash; got != wantHash {
		t.Fatalf("good hash = %s, want %s", got, wantHash)
	}
}

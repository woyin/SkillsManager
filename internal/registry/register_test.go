// internal/registry/register_test.go 验证 Register 原语：默认 global、
// 写入前验证、本地单文件物化、同名同 Source 刷新、同名不同 Source 失败/
// --force 替换、跨 category 同名拒绝。
package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func writeValidSkill(t *testing.T, dir string) {
	t.Helper()
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "SKILL.md"),
		[]byte("---\nname: "+filepath.Base(dir)+"\ndescription: a valid skill\n---\n# x"), 0644)
}

func TestRegisterDefaultsToGlobal(t *testing.T) {
	regDir := t.TempDir()
	os.MkdirAll(filepath.Join(regDir, "skills", "global"), 0755)
	reg := New(regDir)
	src := filepath.Join(t.TempDir(), "my-skill")
	writeValidSkill(t, src)

	res, err := reg.Register(src, "", SkillOrigin{SourceKind: SourceLocalSnapshot, Source: src}, false)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if res.Category != "global" {
		t.Errorf("category = %s, want global", res.Category)
	}
	want := filepath.Join(regDir, "skills", "global", "my-skill")
	if res.Path != want {
		t.Errorf("path = %s, want %s", res.Path, want)
	}
	if res.Outcome != OutcomeCreated {
		t.Errorf("outcome = %s, want created", res.Outcome)
	}
	if _, err := os.Stat(filepath.Join(want, "SKILL.md")); err != nil {
		t.Errorf("skill not materialized: %v", err)
	}
}

func TestRegisterRejectsMissingName(t *testing.T) {
	regDir := t.TempDir()
	reg := New(regDir)
	src := filepath.Join(t.TempDir(), "bad")
	os.MkdirAll(src, 0755)
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\ndescription: no name\n---\n# x"), 0644)

	_, err := reg.Register(src, "", SkillOrigin{SourceKind: SourceLocalSnapshot, Source: src}, false)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	// Registry 必须无变化。
	if _, err := os.Stat(filepath.Join(regDir, "skills", "global", "bad")); !os.IsNotExist(err) {
		t.Error("skill should not be written on validation failure")
	}
}

func TestRegisterRejectsInvalidName(t *testing.T) {
	regDir := t.TempDir()
	reg := New(regDir)
	src := filepath.Join(t.TempDir(), "src")
	os.MkdirAll(src, 0755)
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: Bad_Name\ndescription: x\n---\n# x"), 0644)

	_, err := reg.Register(src, "", SkillOrigin{SourceKind: SourceLocalSnapshot, Source: src}, false)
	if err == nil {
		t.Fatal("expected error for invalid name")
	}
}

func TestRegisterRejectsMissingDescription(t *testing.T) {
	regDir := t.TempDir()
	reg := New(regDir)
	src := filepath.Join(t.TempDir(), "src")
	os.MkdirAll(src, 0755)
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: ok\n---\n# x"), 0644)

	_, err := reg.Register(src, "", SkillOrigin{SourceKind: SourceLocalSnapshot, Source: src}, false)
	if err == nil {
		t.Fatal("expected error for missing description")
	}
}

func TestRegisterMaterializesSingleSkillMD(t *testing.T) {
	regDir := t.TempDir()
	reg := New(regDir)
	src := filepath.Join(t.TempDir(), "SKILL.md")
	os.MkdirAll(filepath.Dir(src), 0755)
	os.WriteFile(src, []byte("---\nname: file-skill\ndescription: from a single file\n---\n# body"), 0644)

	res, err := reg.Register(src, "", SkillOrigin{SourceKind: SourceLocalSnapshot, Source: src}, false)
	if err != nil {
		t.Fatalf("Register single file failed: %v", err)
	}
	want := filepath.Join(regDir, "skills", "global", "file-skill", "SKILL.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("single file not materialized to standard dir: %v", err)
	}
	if res.Name != "file-skill" {
		t.Errorf("name = %s, want file-skill", res.Name)
	}
}

func TestRegisterRejectsNonSkillMDFile(t *testing.T) {
	regDir := t.TempDir()
	reg := New(regDir)
	src := filepath.Join(t.TempDir(), "README.md")
	os.MkdirAll(filepath.Dir(src), 0755)
	os.WriteFile(src, []byte("# not a skill"), 0644)

	_, err := reg.Register(src, "", SkillOrigin{SourceKind: SourceLocalSnapshot, Source: src}, false)
	if err == nil {
		t.Fatal("expected error for non-SKILL.md single file")
	}
}

func TestRegisterSameSourceRefreshes(t *testing.T) {
	regDir := t.TempDir()
	reg := New(regDir)
	src := filepath.Join(t.TempDir(), "my-skill")
	writeValidSkill(t, src)
	origin := SkillOrigin{SourceKind: SourceLocalSnapshot, Source: src}

	first, err := reg.Register(src, "", origin, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Outcome != OutcomeCreated {
		t.Fatalf("first outcome = %s, want created", first.Outcome)
	}

	second, err := reg.Register(src, "", origin, false)
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if second.Outcome != OutcomeRefreshed {
		t.Errorf("second outcome = %s, want refreshed", second.Outcome)
	}
	if second.Path != first.Path {
		t.Errorf("path changed: %s vs %s", first.Path, second.Path)
	}
}

func TestRegisterDifferentSourceFails(t *testing.T) {
	regDir := t.TempDir()
	reg := New(regDir)
	src1 := filepath.Join(t.TempDir(), "a", "my-skill")
	writeValidSkill(t, src1)
	src2 := filepath.Join(t.TempDir(), "b", "my-skill")
	writeValidSkill(t, src2)

	if _, err := reg.Register(src1, "", SkillOrigin{SourceKind: SourceLocalSnapshot, Source: src1}, false); err != nil {
		t.Fatal(err)
	}
	_, err := reg.Register(src2, "", SkillOrigin{SourceKind: SourceLocalSnapshot, Source: src2}, false)
	if _, ok := err.(*CrossSourceError); !ok {
		t.Fatalf("err type = %T, want *CrossSourceError", err)
	}
}

func TestRegisterForceReplaces(t *testing.T) {
	regDir := t.TempDir()
	reg := New(regDir)
	src1 := filepath.Join(t.TempDir(), "a", "my-skill")
	writeValidSkill(t, src1)
	src2 := filepath.Join(t.TempDir(), "b", "my-skill")
	writeValidSkill(t, src2)

	if _, err := reg.Register(src1, "", SkillOrigin{SourceKind: SourceLocalSnapshot, Source: src1}, false); err != nil {
		t.Fatal(err)
	}
	res, err := reg.Register(src2, "", SkillOrigin{SourceKind: SourceLocalSnapshot, Source: src2}, true)
	if err != nil {
		t.Fatalf("force replace failed: %v", err)
	}
	if res.Outcome != OutcomeReplaced {
		t.Errorf("outcome = %s, want replaced", res.Outcome)
	}
	// 新 origin 应反映 src2。
	read := reg.ReadOrigin(res.Path)
	if read.Origin.Source != src2 {
		t.Errorf("origin source = %s, want %s", read.Origin.Source, src2)
	}
}

func TestRegisterCrossCategorySameNameFails(t *testing.T) {
	regDir := t.TempDir()
	reg := New(regDir)
	src := filepath.Join(t.TempDir(), "my-skill")
	writeValidSkill(t, src)

	if _, err := reg.Register(src, "global", SkillOrigin{SourceKind: SourceLocalSnapshot, Source: src}, false); err != nil {
		t.Fatal(err)
	}
	_, err := reg.Register(src, "codex-only", SkillOrigin{SourceKind: SourceLocalSnapshot, Source: src}, false)
	if _, ok := err.(*NameConflictError); !ok {
		t.Fatalf("err type = %T, want *NameConflictError", err)
	}
}

func TestRegisterWritesOrigin(t *testing.T) {
	regDir := t.TempDir()
	reg := New(regDir)
	src := filepath.Join(t.TempDir(), "my-skill")
	writeValidSkill(t, src)

	_, err := reg.Register(src, "", SkillOrigin{
		SourceKind: SourceGit,
		Source:     "owner/repo",
		RefKind:    RefBranch,
		Ref:        "main",
		SubPath:    "skills/my-skill",
		Commit:     "abc123",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(regDir, "skills", "global", "my-skill")
	read := reg.ReadOrigin(dir)
	if !read.Valid {
		t.Fatal("origin not valid after register")
	}
	if read.Origin.Source != "owner/repo" {
		t.Errorf("source = %s", read.Origin.Source)
	}
	if read.Origin.RefKind != RefBranch {
		t.Errorf("ref_kind = %s, want branch", read.Origin.RefKind)
	}
	if read.Origin.SubPath != "skills/my-skill" {
		t.Errorf("sub_path = %s", read.Origin.SubPath)
	}
}

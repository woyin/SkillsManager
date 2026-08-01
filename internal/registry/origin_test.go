// internal/registry/origin_test.go 验证 Skill Origin provenance 元数据：
// schema 校验、ref kind 分类、Snapshot/Orphan 区分、全局唯一身份解析、
// 跨 category 冲突检测、旧 .sm-origin.json 向后兼容读取。
package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSkillName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"a", true},
		{"my-skill", true},
		{"skill-1", true},
		{"a1-b2-c3", true},
		{"ab", true},
		{"", false},
		{"-leading", false},
		{"trailing-", false},
		{"double--hyphen", false},
		{"UPPER", false},
		{"has space", false},
		{"has_underscore", false},
		{strings.Repeat("a", 64), true},
		{strings.Repeat("a", 65), false},
	}
	for _, c := range cases {
		err := ValidateSkillName(c.name)
		if c.ok && err != nil {
			t.Errorf("ValidateSkillName(%q) = %v, want nil", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("ValidateSkillName(%q) = nil, want error", c.name)
		}
	}
}

func TestValidateDescription(t *testing.T) {
	if err := ValidateDescription(""); err == nil {
		t.Error("empty description should fail")
	}
	if err := ValidateDescription("ok"); err != nil {
		t.Errorf("short valid description failed: %v", err)
	}
	long := strings.Repeat("x", 1025)
	if err := ValidateDescription(long); err == nil {
		t.Error("overlong description should fail")
	}
	exact := strings.Repeat("x", 1024)
	if err := ValidateDescription(exact); err != nil {
		t.Errorf("exactly 1024 should pass: %v", err)
	}
}

func TestOriginClassifyTracking(t *testing.T) {
	dir := makeSkillWithOrigin(t, "tracking-skill", SkillOrigin{
		SourceKind: SourceGit,
		Source:     "owner/repo",
		RefKind:    RefDefaultBranch,
	})
	reg := New(filepath.Dir(filepath.Dir(dir)))
	read := reg.ReadOrigin(dir)
	if read.Class != ClassTracking {
		t.Errorf("class = %s, want tracking", read.Class)
	}
	if read.Origin.IsPinned() {
		t.Error("default-branch must not be pinned")
	}
}

func TestOriginClassifyPinned(t *testing.T) {
	for _, rk := range []RefKind{RefTag, RefCommit} {
		dir := makeSkillWithOrigin(t, "pin-"+string(rk), SkillOrigin{
			SourceKind: SourceGit,
			Source:     "owner/repo",
			RefKind:    rk,
		})
		reg := New(filepath.Dir(filepath.Dir(dir)))
		read := reg.ReadOrigin(dir)
		if read.Class != ClassPinned {
			t.Errorf("ref_kind=%s class = %s, want pinned", rk, read.Class)
		}
	}
}

func TestOriginClassifySnapshot(t *testing.T) {
	dir := makeSkillWithOrigin(t, "snap-skill", SkillOrigin{
		SourceKind: SourceLocalSnapshot,
		Source:     "/abs/local/path",
	})
	reg := New(filepath.Dir(filepath.Dir(dir)))
	read := reg.ReadOrigin(dir)
	if read.Class != ClassSnapshot {
		t.Errorf("class = %s, want snapshot", read.Class)
	}
}

func TestOriginClassifyOrphan(t *testing.T) {
	regDir := t.TempDir()
	skillDir := filepath.Join(regDir, "skills", "global", "orphan-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# x"), 0644)
	reg := New(regDir)
	read := reg.ReadOrigin(skillDir)
	if read.Class != ClassOrphan {
		t.Errorf("class = %s, want orphan (no origin file)", read.Class)
	}
	if read.HasFile {
		t.Error("HasFile should be false when no origin file")
	}
}

func TestOriginBackwardCompatOldSchema(t *testing.T) {
	regDir := t.TempDir()
	skillDir := filepath.Join(regDir, "skills", "global", "legacy-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# x"), 0644)

	old := map[string]any{
		"source":     "owner/repo",
		"ref":        "",
		"rel_path":   ".",
		"commit":     "abc123",
		"updated_at": "2025-01-01T00:00:00Z",
	}
	data, _ := json.MarshalIndent(old, "", "  ")
	os.WriteFile(filepath.Join(skillDir, OriginFile), data, 0644)

	reg := New(regDir)
	read := reg.ReadOrigin(skillDir)
	if !read.Valid {
		t.Fatal("legacy origin should be valid")
	}
	if read.Class != ClassTracking {
		t.Errorf("legacy empty-ref class = %s, want tracking (default-branch)", read.Class)
	}
	if read.Origin.SourceKind != SourceGit {
		t.Errorf("legacy source_kind = %s, want git (inferred)", read.Origin.SourceKind)
	}
	if read.Origin.RefKind != RefDefaultBranch {
		t.Errorf("legacy empty-ref ref_kind = %s, want default-branch", read.Origin.RefKind)
	}
	if read.Origin.SubPath != "." {
		t.Errorf("legacy rel_path mapped to sub_path = %q, want '.'", read.Origin.SubPath)
	}
}

func TestOriginBackwardCompatOldSchemaPinned(t *testing.T) {
	regDir := t.TempDir()
	skillDir := filepath.Join(regDir, "skills", "global", "legacy-pin")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# x"), 0644)

	old := map[string]any{
		"source": "owner/repo",
		"ref":    "v1.2.3",
	}
	data, _ := json.MarshalIndent(old, "", "  ")
	os.WriteFile(filepath.Join(skillDir, OriginFile), data, 0644)

	reg := New(regDir)
	read := reg.ReadOrigin(skillDir)
	if read.Class != ClassPinned {
		t.Errorf("legacy non-empty ref class = %s, want pinned", read.Class)
	}
}

func TestResolveUniqueSkillSingleMatch(t *testing.T) {
	regDir := t.TempDir()
	makeSkillInRegistry(t, regDir, "global", "unique-one")
	reg := New(regDir)
	res, err := reg.ResolveUniqueSkill("unique-one")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if res.Category != "global" {
		t.Errorf("category = %s, want global", res.Category)
	}
}

func TestResolveUniqueSkillNotFound(t *testing.T) {
	regDir := t.TempDir()
	os.MkdirAll(filepath.Join(regDir, "skills", "global"), 0755)
	reg := New(regDir)
	_, err := reg.ResolveUniqueSkill("missing")
	if _, ok := err.(*NameNotFoundError); !ok {
		t.Errorf("err type = %T, want *NameNotFoundError", err)
	}
}

func TestResolveUniqueSkillConflict(t *testing.T) {
	regDir := t.TempDir()
	makeSkillInRegistry(t, regDir, "global", "dup")
	makeSkillInRegistry(t, regDir, "codex-only", "dup")
	reg := New(regDir)
	_, err := reg.ResolveUniqueSkill("dup")
	ce, ok := err.(*NameConflictError)
	if !ok {
		t.Fatalf("err type = %T, want *NameConflictError", err)
	}
	if len(ce.Categories) != 2 {
		t.Errorf("categories = %v, want 2", ce.Categories)
	}
}

func TestFindConflictsNone(t *testing.T) {
	regDir := t.TempDir()
	makeSkillInRegistry(t, regDir, "global", "a")
	makeSkillInRegistry(t, regDir, "global", "b")
	reg := New(regDir)
	conflicts, err := reg.FindConflicts()
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(conflicts))
	}
}

func TestFindConflictsReportsAll(t *testing.T) {
	regDir := t.TempDir()
	makeSkillInRegistry(t, regDir, "global", "dup1")
	makeSkillInRegistry(t, regDir, "codex-only", "dup1")
	makeSkillInRegistry(t, regDir, "global", "dup2")
	makeSkillInRegistry(t, regDir, "claude-only", "dup2")
	reg := New(regDir)
	conflicts, err := reg.FindConflicts()
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 2 {
		t.Fatalf("expected 2 conflicts, got %d", len(conflicts))
	}
}

func TestListAllOriginalsClassifies(t *testing.T) {
	regDir := t.TempDir()
	// tracking
	makeSkillWithOriginInReg(t, regDir, "global", "track", SkillOrigin{SourceKind: SourceGit, Source: "o/r", RefKind: RefDefaultBranch})
	// snapshot
	makeSkillWithOriginInReg(t, regDir, "global", "snap", SkillOrigin{SourceKind: SourceLocalSnapshot, Source: "/local"})
	// orphan (no origin)
	makeSkillInRegistry(t, regDir, "global", "orph")
	reg := New(regDir)
	originals, err := reg.ListAllOriginals()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]RegistryOriginal{}
	for _, o := range originals {
		byName[o.Name] = o
	}
	if byName["track"].Class != ClassTracking {
		t.Errorf("track class = %s", byName["track"].Class)
	}
	if byName["snap"].Class != ClassSnapshot {
		t.Errorf("snap class = %s", byName["snap"].Class)
	}
	if byName["orph"].Class != ClassOrphan {
		t.Errorf("orph class = %s", byName["orph"].Class)
	}
}

// makeSkillWithOrigin 在临时 registry 下创建 global/<name> 并写入 origin。
// 返回 skill 目录绝对路径。
func makeSkillWithOrigin(t *testing.T, name string, origin SkillOrigin) string {
	t.Helper()
	regDir := t.TempDir()
	return makeSkillWithOriginInReg(t, regDir, "global", name, origin)
}

func makeSkillWithOriginInReg(t *testing.T, regDir, category, name string, origin SkillOrigin) string {
	t.Helper()
	dir := filepath.Join(regDir, "skills", category, name)
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: test\n---\n# x"), 0644)
	reg := New(regDir)
	if err := reg.WriteOrigin(dir, origin); err != nil {
		t.Fatal(err)
	}
	return dir
}

func makeSkillInRegistry(t *testing.T, regDir, category, name string) {
	t.Helper()
	dir := filepath.Join(regDir, "skills", category, name)
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name), 0644)
}

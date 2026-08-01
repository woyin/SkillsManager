// internal/registry/refkind_test.go 验证 ref → RefKind 解析。
package registry

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveRefKindEmpty(t *testing.T) {
	r, k, err := ResolveRefKind("", "")
	if err != nil || r != "" || k != RefDefaultBranch {
		t.Errorf("got (%q,%s,%v), want (\"\",default-branch,nil)", r, k, err)
	}
}

func TestResolveRefKindQualified(t *testing.T) {
	r, k, _ := ResolveRefKind("refs/heads/main", "")
	if k != RefBranch || r != "main" {
		t.Errorf("heads: got (%q,%s)", r, k)
	}
	r, k, _ = ResolveRefKind("refs/tags/v1", "")
	if k != RefTag || r != "v1" {
		t.Errorf("tags: got (%q,%s)", r, k)
	}
}

func TestResolveRefKindCommitHash(t *testing.T) {
	h := "abcdef0123456789abcdef0123456789abcdef01"
	r, k, _ := ResolveRefKind(h, "")
	if k != RefCommit || r != h {
		t.Errorf("commit: got (%q,%s)", r, k)
	}
}

func TestResolveRefKindWithoutRepo(t *testing.T) {
	r, k, _ := ResolveRefKind("some-branch", "")
	if k != RefUnknown {
		t.Errorf("no repo: got (%q,%s), want unknown", r, k)
	}
}

func TestResolveRefKindQueryRepo(t *testing.T) {
	// 构造一个带 branch 与 tag 的本地仓库。
	repo := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	os.WriteFile(filepath.Join(repo, "x"), []byte("1"), 0644)
	run("add", "x")
	run("commit", "-m", "c")
	run("tag", "v1.0")
	// branch main 应解析为 branch。
	_, k, err := ResolveRefKind("main", repo)
	if err != nil {
		t.Fatalf("main: %v", err)
	}
	if k != RefBranch {
		t.Errorf("main kind = %s, want branch", k)
	}
	// tag v1.0 应解析为 tag。
	_, k, err = ResolveRefKind("v1.0", repo)
	if err != nil {
		t.Fatalf("v1.0: %v", err)
	}
	if k != RefTag {
		t.Errorf("v1.0 kind = %s, want tag", k)
	}
	// 未知 ref：当作 commit。
	_, k, _ = ResolveRefKind("deadbeefnotabranch", repo)
	if k != RefCommit {
		t.Errorf("unknown kind = %s, want commit", k)
	}
}

func TestResolveRefKindAmbiguousBranchTag(t *testing.T) {
	repo := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "x")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	os.WriteFile(filepath.Join(repo, "x"), []byte("1"), 0644)
	run("add", "x")
	run("commit", "-m", "c")
	// 同名 branch 与 tag。
	run("branch", "conflict", "x")
	run("tag", "conflict")
	_, _, err := ResolveRefKind("conflict", repo)
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
}

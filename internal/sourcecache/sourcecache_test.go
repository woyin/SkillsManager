package sourcecache

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestKeyIncludesRefWithoutChangingLegacyShape(t *testing.T) {
	if got, want := Key("source", ""), "41cf6794ba4200b839c53531555f0f3998df4cbb01a4d5cb0b94e3ca5e23947d"; got != want {
		t.Fatalf("Key(source, empty ref) = %s, want %s", got, want)
	}
	if Key("source", "main") == Key("source", "tag") {
		t.Fatal("different refs must use different cache keys")
	}
	if Key("source", "main") == Key("sourcemain", "") {
		t.Fatal("ref must be part of the cache key")
	}
}

func TestAcquireWritesMetadataAndReusesCache(t *testing.T) {
	repo := makeRepo(t, "cache-hit")
	store := New(t.TempDir())
	source := "file://" + repo

	first, err := store.Acquire(source, "", false)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if first.Cached {
		t.Fatal("first acquire should be a cache miss")
	}
	if first.Metadata.Source != source || first.Metadata.Ref != "" || first.Metadata.Commit == "" || first.Metadata.CreatedAt.IsZero() {
		t.Fatalf("metadata = %+v", first.Metadata)
	}
	cachePath, metadataPath := store.Paths(source, "")
	if first.Path != cachePath {
		t.Fatalf("path = %q, want %q", first.Path, cachePath)
	}
	onDisk, err := ReadMetadata(metadataPath)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if onDisk != first.Metadata {
		t.Fatalf("on-disk metadata = %+v, acquire metadata = %+v", onDisk, first.Metadata)
	}

	second, err := store.Acquire(source, "", false)
	if err != nil {
		t.Fatalf("cache hit: %v", err)
	}
	if !second.Cached || second.Path != first.Path {
		t.Fatalf("cache hit = %+v, want cached path %q", second, first.Path)
	}
	if second.Metadata.Source != source || second.Metadata.Commit != first.Metadata.Commit {
		t.Fatalf("cache-hit metadata = %+v, want source/commit from first acquire", second.Metadata)
	}
}

func TestAcquireOfflineOnlyUsesExactKey(t *testing.T) {
	repo := makeRepo(t, "offline")
	store := New(t.TempDir())
	source := "file://" + repo
	if _, err := store.Acquire(source, "", false); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	if result, err := store.Acquire(source, "", true); err != nil || !result.Cached {
		t.Fatalf("offline cache hit = %+v, %v", result, err)
	}
	if _, err := store.Acquire(source, "missing-ref", true); err == nil {
		t.Fatal("offline cache miss should fail")
	} else if !strings.Contains(err.Error(), "source not cached for offline install") {
		t.Fatalf("offline error = %v", err)
	}
	missingPath, _ := store.Paths(source, "missing-ref")
	if _, err := os.Stat(missingPath); !os.IsNotExist(err) {
		t.Fatalf("offline miss created cache path: %v", err)
	}
}

func TestAcquireTreeURLUsesParsedBranch(t *testing.T) {
	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "git.log")
	fakeGit := filepath.Join(bin, "git")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$SOURCECACHE_GIT_LOG"
if [ "$1" = "clone" ]; then
  eval "dest=\${$#}"
  mkdir -p "$dest/.git"
  exit 0
fi
if [ "$1" = "-C" ] && [ "$3" = "rev-parse" ]; then
  printf 'tree-commit\n'
fi
exit 0
`
	if err := os.WriteFile(fakeGit, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SOURCECACHE_GIT_LOG", logPath)

	store := New(t.TempDir())
	source := "https://github.com/acme/repo/tree/main/skills/demo"
	result, err := store.Acquire(source, "", false)
	if err != nil {
		t.Fatalf("tree URL acquire: %v", err)
	}
	if result.Metadata.Commit != "tree-commit" {
		t.Fatalf("tree URL commit = %q, want tree-commit", result.Metadata.Commit)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	line := string(log)
	if !strings.Contains(line, "clone --branch main --depth 1 https://github.com/acme/repo ") {
		t.Fatalf("tree URL clone args = %q", line)
	}
}

func TestMetadataRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "meta.json")
	want := Metadata{Source: "https://example.invalid/repo", Ref: "main", Commit: "abc", CreatedAt: mustTime("2025-01-02T03:04:05Z")}
	if err := WriteMetadata(path, want); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	got, err := ReadMetadata(path)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if got != want {
		t.Fatalf("metadata = %+v, want %+v", got, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("metadata is not JSON: %v", err)
	}
}

func TestAcquireNilStoreFailsWithoutFilesystemWork(t *testing.T) {
	var store *Store
	if _, err := store.Acquire("https://example.invalid/repo", "", false); err == nil {
		t.Fatal("nil Store Acquire should fail")
	}
}

func TestAcquireCacheHitBackfillsMissingCommit(t *testing.T) {
	repo := makeRepo(t, "missing-commit")
	store := New(t.TempDir())
	source := "file://" + repo
	first, err := store.Acquire(source, "", false)
	if err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	_, metadataPath := store.Paths(source, "")
	if err := os.WriteFile(metadataPath, []byte(`{"source":"`+source+`"}`), 0644); err != nil {
		t.Fatalf("removing cached commit: %v", err)
	}

	hit, err := store.Acquire(source, "", true)
	if err != nil {
		t.Fatalf("cache hit: %v", err)
	}
	if !hit.Cached || hit.Path != first.Path || hit.Metadata.Commit == "" {
		t.Fatalf("cache hit = %+v, want cached path and recovered commit", hit)
	}
}

func TestReadMetadataRejectsMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadMetadata(path); err == nil {
		t.Fatal("ReadMetadata should reject malformed JSON")
	}
}

func TestConvenienceFunctionsAndMetadataWriteFailure(t *testing.T) {
	dataDir := t.TempDir()
	cachePath, metadataPath := Paths(dataDir, "source", "main")
	if cachePath == "" || metadataPath == "" {
		t.Fatalf("Paths returned empty paths: %q, %q", cachePath, metadataPath)
	}

	blockedParent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockedParent, []byte("file"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteMetadata(filepath.Join(blockedParent, "metadata.json"), Metadata{}); err == nil {
		t.Fatal("WriteMetadata should reject a non-directory parent")
	}

	repo := makeRepo(t, "convenience")
	result, err := Acquire(dataDir, "file://"+repo, "", false)
	if err != nil || result.Path == "" {
		t.Fatalf("Acquire convenience = %+v, %v", result, err)
	}
}

func TestAcquireRemovesCheckoutWhenMetadataCannotBeWritten(t *testing.T) {
	repo := makeRepo(t, "metadata-write-failure")
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "sources-meta"), []byte("file"), 0644); err != nil {
		t.Fatal(err)
	}
	store := New(dataDir)
	source := "file://" + repo
	if _, err := store.Acquire(source, "", false); err == nil {
		t.Fatal("Acquire should fail when cache metadata cannot be written")
	}
	cachePath, _ := store.Paths(source, "")
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("failed Acquire left cached checkout behind: %v", err)
	}
}

func TestCloneAndCheckoutHelpers(t *testing.T) {
	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "git.log")
	fakeGit := filepath.Join(bin, "git")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$SOURCECACHE_GIT_LOG"
if [ "$1" = "clone" ]; then
  for argument in "$@"; do dest="$argument"; done
  mkdir -p "$dest/.git"
fi
exit 0
`
	if err := os.WriteFile(fakeGit, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SOURCECACHE_GIT_LOG", logPath)

	cloneDest, err := clone("https://github.com/acme/repo", "release")
	if err != nil {
		t.Fatalf("clone explicit ref: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(cloneDest)) })
	if err := fetchAndCheckout(cloneDest, "other-ref"); err != nil {
		t.Fatalf("fetchAndCheckout: %v", err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"clone --branch release --depth 1 https://github.com/acme/repo",
		"-C " + cloneDest + " fetch --depth=1 origin other-ref",
		"-C " + cloneDest + " checkout --detach other-ref",
	} {
		if !strings.Contains(string(log), want) {
			t.Fatalf("git log = %q, missing %q", log, want)
		}
	}
}

func TestCloneFallbackAndGitHelperFailures(t *testing.T) {
	bin := t.TempDir()
	fakeGit := filepath.Join(bin, "git")
	script := `#!/bin/sh
case "$*" in
  *bad-repo*) exit 1 ;;
  *bad-fetch*) exit 1 ;;
  *bad-checkout*) exit 1 ;;
esac
if [ "$1" = "clone" ]; then
  if [ "$2" = "--branch" ]; then
    exit 1
  fi
  for argument in "$@"; do dest="$argument"; done
  mkdir -p "$dest/.git"
  exit 0
fi
if [ "$1" = "-C" ] && [ "$3" = "rev-parse" ]; then
  printf 'test-head\n'
fi
exit 0
`
	if err := os.WriteFile(fakeGit, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	defaultClone, err := clone("https://github.com/acme/default", "")
	if err != nil {
		t.Fatalf("default-branch clone: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(defaultClone)) })

	fallbackClone, err := clone("https://github.com/acme/commit", "deadbeef")
	if err != nil {
		t.Fatalf("commit fallback clone: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(fallbackClone)) })

	if _, err := clone("https://github.com/acme/bad-repo", "deadbeef"); err == nil {
		t.Fatal("clone should report a full-clone failure after branch fallback")
	}
	if err := fetchAndCheckout(defaultClone, "bad-fetch"); err == nil {
		t.Fatal("fetchAndCheckout should report a fetch failure")
	}
	if err := checkoutDetached(defaultClone, "bad-checkout"); err == nil {
		t.Fatal("checkoutDetached should report a checkout failure")
	}
	if got := headCommit(defaultClone); got != "test-head" {
		t.Fatalf("headCommit = %q, want test-head", got)
	}
	if got := headCommit("bad-repo"); got != "" {
		t.Fatalf("headCommit failure = %q, want empty", got)
	}
}

func BenchmarkAcquireCacheHit(b *testing.B) {
	repo := makeBenchmarkRepo(b)
	store := New(b.TempDir())
	source := "file://" + repo
	if _, err := store.Acquire(source, "", false); err != nil {
		b.Fatalf("seeding cache: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := store.Acquire(source, "", true)
		if err != nil || !result.Cached {
			b.Fatalf("cache hit = %+v, %v", result, err)
		}
	}
}

func makeRepo(t *testing.T, name string) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(repo, "skills", "demo"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "skills", "demo", "SKILL.md"), []byte("---\nname: demo\ndescription: a sourcecache test skill\n---\n# demo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q")
	git(t, repo, "config", "user.name", "sourcecache-test")
	git(t, repo, "config", "user.email", "sourcecache@example.invalid")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-qm", "initial")
	return repo
}

func makeBenchmarkRepo(b *testing.B) string {
	b.Helper()
	repo := filepath.Join(b.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(repo, "skills", "demo"), 0755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "skills", "demo", "SKILL.md"), []byte("---\nname: demo\ndescription: a benchmark skill\n---\n# demo\n"), 0644); err != nil {
		b.Fatal(err)
	}
	gitBenchmark(b, repo, "init", "-q")
	gitBenchmark(b, repo, "config", "user.name", "sourcecache-benchmark")
	gitBenchmark(b, repo, "config", "user.email", "sourcecache@example.invalid")
	gitBenchmark(b, repo, "add", ".")
	gitBenchmark(b, repo, "commit", "-qm", "initial")
	return repo
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func gitBenchmark(b *testing.B, dir string, args ...string) {
	b.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func mustTime(value string) (result time.Time) {
	result, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return result
}

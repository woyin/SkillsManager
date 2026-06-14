package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// fakeGitSleep writes a `git` shell script that sleeps a fixed duration (to
// emulate network-bound `git pull` latency) and logs its arguments. Returns
// a bin dir to prepend to PATH and the invocation log path.
func fakeGitSleep(t testing.TB, seconds float64) (binDir, logPath string) {
	t.Helper()
	binDir = t.TempDir()
	logPath = filepath.Join(t.TempDir(), "git.log")
	fakeGit := filepath.Join(binDir, "git")
	script := fmt.Sprintf("#!/bin/sh\nsleep %.3f\nprintf '%%s\\n' \"$*\" >> %s\nexit 0\n", seconds, logPath)
	if err := os.WriteFile(fakeGit, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return binDir, logPath
}

// makeFakeRepos creates n git-marked repos under a registry dir.
func makeFakeRepos(t testing.TB, n int) []namedRepo {
	t.Helper()
	regDir := t.TempDir()
	repos := make([]namedRepo, n)
	for i := 0; i < n; i++ {
		dir := filepath.Join(regDir, "skills", "cat", skillLabel(i))
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
			t.Fatal(err)
		}
		repos[i] = namedRepo{path: dir, label: skillLabel(i)}
	}
	return repos
}

func skillLabel(i int) string {
	const a = "abcdefghijklmnopqrstuvwxyz"
	if i < len(a) {
		return "skill-" + string(a[i])
	}
	return "skill-x" + skillLabel(i-len(a))
}

// silentEnv returns a copy of os.Environ() with stdout discarded by directing
// git's output to os.DevNull via the shell is not feasible here. Instead we
// keep the benchmark honest but suppress sm's own progress prints by swapping
// os.Stdout during the measured section.
func withSilentStdout(b *testing.B, fn func()) {
	orig := os.Stdout
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Skipf("cannot open /dev/null: %v", err)
	}
	os.Stdout = f
	defer func() { os.Stdout = orig; f.Close() }()
	fn()
}

// BenchmarkPullReposParallel measures the wall-clock of pulling many repos
// concurrently (16 repos, 50ms fake latency each). Compare against
// BenchmarkPullReposSerial16: serial would be ~16×50ms=800ms, parallel with
// 8 workers finishes in ~2 batches ≈ 100ms.
func BenchmarkPullReposParallel(b *testing.B) {
	binDir, _ := fakeGitSleep(b, 0.05)
	b.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	repos := makeFakeRepos(b, 16)

	b.ResetTimer()
	withSilentStdout(b, func() {
		for i := 0; i < b.N; i++ {
			pullReposConcurrently(repos)
		}
	})
}

// BenchmarkPullReposSerial16 is the serial baseline: the same 16 repos pulled
// one at a time (the pre-optimization behavior). The ratio of this over the
// parallel benchmark is the speedup factor.
func BenchmarkPullReposSerial16(b *testing.B) {
	binDir, _ := fakeGitSleep(b, 0.05)
	b.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	repos := makeFakeRepos(b, 16)

	b.ResetTimer()
	withSilentStdout(b, func() {
		for i := 0; i < b.N; i++ {
			for _, r := range repos {
				exec.Command("git", "-C", r.path, "pull", "--ff-only").Run()
			}
		}
	})
}

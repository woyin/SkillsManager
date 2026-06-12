package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRootLongDescriptionListsSupportedTools(t *testing.T) {
	for _, name := range []string{"Codex", "Claude", "Gemini", "OpenCode", "Hermes", "OpenClaw"} {
		if !strings.Contains(rootCmd.Long, name) {
			t.Fatalf("root long description should mention %s, got: %q", name, rootCmd.Long)
		}
	}
}

func TestDefaultPathFlagsUseUserSMDirectory(t *testing.T) {
	base := filepath.Join(userHomeDir(t), ".sm")

	tests := map[string]string{
		"registry": filepath.Join(base, "registry"),
		"data":     filepath.Join(base, "data"),
		"profiles": filepath.Join(base, "profiles"),
	}

	for name, want := range tests {
		flag := rootCmd.PersistentFlags().Lookup(name)
		if flag == nil {
			t.Fatalf("expected persistent flag %q to be registered", name)
		}
		if flag.DefValue != want {
			t.Fatalf("default for --%s = %q, want %q", name, flag.DefValue, want)
		}
	}
}

func userHomeDir(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("getting user home dir: %v", err)
	}
	return home
}

func TestReadmeDocumentsRootFlagsAndNoVersionSubcommand(t *testing.T) {
	readme := readProjectFile(t, "README.md")

	if strings.Contains(readme, "sm version") {
		t.Fatalf("README.md documents nonexistent `sm version` command")
	}
	for _, want := range []string{"`--registry`", "`--data`", "`--profiles`", "`-v, --version`"} {
		if !strings.Contains(readme, want) {
			t.Fatalf("README.md should document global flag %s", want)
		}
	}
	for _, want := range []string{"`~/.sm/registry`", "`~/.sm/data`", "`~/.sm/profiles`"} {
		if !strings.Contains(readme, want) {
			t.Fatalf("README.md should document default path %s", want)
		}
	}
}

func readProjectFile(t *testing.T, path string) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locating current test file")
	}
	projectRoot := filepath.Dir(filepath.Dir(currentFile))
	data, err := os.ReadFile(filepath.Join(projectRoot, path))
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

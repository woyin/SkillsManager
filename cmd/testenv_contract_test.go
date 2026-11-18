package cmd

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/spf13/pflag"
	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/tool"
)

func TestPlanFixtureRestoresHomeCache(t *testing.T) {
	before := home.Dir()
	t.Cleanup(home.ResetForTest)
	t.Run("preceding-plan", func(t *testing.T) { setupPlanCommand(t) })
	if got := home.Dir(); got != before {
		t.Fatalf("plan fixture leaked home cache: got %q, want %q", got, before)
	}
}

func TestCmdGlobalsRestoredAfterMutation(t *testing.T) {
	isolateCmdGlobals(t)
	rootCmd.SetArgs(nil)
	planCmd.SetArgs(nil)
	planCmd.SetOut(nil)
	planCmd.SetErr(nil)
	planCmd.SetIn(nil)
	beforeRegistry, beforeProfiles, beforeData := RegistryDir, ProfilesDir, DataDir
	flag := installCmd.Flags().Lookup("agent")
	values := flag.Value.(pflag.SliceValue)
	wantAgents := []string{"claude", "codex"}
	if err := values.Replace(wantAgents); err != nil {
		t.Fatal(err)
	}
	flag.Changed = false
	t.Run("preceding-command", func(t *testing.T) {
		isolateCmdGlobals(t)
		RegistryDir, ProfilesDir, DataDir = "registry-changed", "profiles-changed", "data-changed"
		rootCmd.SetArgs([]string{"plan", "--apply"})
		planCmd.SetArgs([]string{"--check"})
		planCmd.SetOut(&bytes.Buffer{})
		planCmd.SetErr(&bytes.Buffer{})
		planCmd.SetIn(bytes.NewBufferString("y\n"))
		if err := installCmd.Flags().Set("agent", "pi"); err != nil {
			t.Fatal(err)
		}
	})
	if RegistryDir != beforeRegistry || ProfilesDir != beforeProfiles || DataDir != beforeData {
		t.Error("command globals were not restored")
	}
	if !cobraFieldIsNil(rootCmd, "args") || !cobraFieldIsNil(planCmd, "args") {
		t.Error("nil command Args must retain their inherited state")
	}
	for _, field := range []string{"outWriter", "errWriter", "inReader"} {
		if !cobraFieldIsNil(planCmd, field) {
			t.Errorf("plan command leaked %s", field)
		}
	}
	if !reflect.DeepEqual(values.GetSlice(), wantAgents) || flag.Changed {
		t.Errorf("agent flag leaked: value=%v Changed=%v", values.GetSlice(), flag.Changed)
	}
}

func TestAgentDetectionOverrideIsRestored(t *testing.T) {
	before := detectAgents()
	t.Run("explicit-agents", func(t *testing.T) {
		isolateAgents(t, "claude")
		if got := detectAgents(); !reflect.DeepEqual(got, []tool.Tool{tool.Claude}) {
			t.Fatalf("detected agents = %v", got)
		}
	})
	if !reflect.DeepEqual(detectAgents(), before) {
		t.Fatal("detector override leaked to the next caller")
	}
}

func TestFakeProcessRecordsGitArgsAndExit(t *testing.T) {
	dir, _, read := installFakeProcess(t, "git", fakeProcessOptions{ExitCode: 7, Stdout: "synthetic-output"})
	name := "git"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out, err := exec.Command(filepath.Join(dir, name), "-C", "synthetic-repo", "pull", "--ff-only").CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 7 {
		t.Fatalf("fake exit = %v, want status 7", err)
	}
	if string(out) != "synthetic-output" || read() != "-C synthetic-repo pull --ff-only\n" {
		t.Fatalf("fake output/argv mismatch: output=%q log=%q", out, read())
	}
}

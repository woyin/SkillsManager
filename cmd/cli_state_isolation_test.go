package cmd

import (
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/home"
)

// 本文件是 T2 段 1（fixture-red）的唯一新增内容：两个 before/after 顺序泄漏
// fixture。它们只观察现有生产/夹具的真实行为，不新增 helper、不新增生产 seam，
// 也不内联镜像将来要实现的恢复逻辑。fixture 自身负责 cleanup，避免污染其他用例。

// TestCLIStateIsolationRestoresProfilesDir 证明现有 setupPlanCommand 会改变包级
// ProfilesDir 却不注册恢复：子测试结束后，父测试作用域仍能观察到被改写的值。
//
// 观察点在子测试之后，因此断言对象是"前序测试改动是否被复原"这一真实状态，
// 而不是对某个 helper 实现的镜像。
func TestCLIStateIsolationRestoresProfilesDir(t *testing.T) {
	origProfiles, origRegistry, origData := ProfilesDir, RegistryDir, DataDir
	t.Cleanup(func() {
		// fixture 自清理：无论断言结果如何都不把临时目录留给后序用例。
		ProfilesDir, RegistryDir, DataDir = origProfiles, origRegistry, origData
		home.ResetForTest()
	})

	t.Run("plan-command-fixture-runs", func(t *testing.T) {
		// 调用真实共享夹具；它会给 RegistryDir/DataDir 注册 cleanup，
		// 但 ProfilesDir 只赋值、不注册恢复。
		_ = setupPlanCommand(t)
	})

	if ProfilesDir != origProfiles {
		t.Fatalf("setupPlanCommand leaked ProfilesDir: got %q, want original %q", ProfilesDir, origProfiles)
	}
}

// TestResetPlanFlagsRestoresPreviousValues 证明现有 resetPlanFlags 把包级 flag
// 写成零值，而不是恢复到调用前的值：非零原值在 reset 之后必然丢失。
func TestResetPlanFlagsRestoresPreviousValues(t *testing.T) {
	prevDir, prevJSON, prevCheck, prevApply := planDir, planJSON, planCheck, planApply
	prevProfile, prevSkills := planProfile, planSkills
	t.Cleanup(func() {
		// fixture 自清理：恢复进入测试前的包级状态。
		planDir, planJSON, planCheck, planApply = prevDir, prevJSON, prevCheck, prevApply
		planProfile, planSkills = prevProfile, prevSkills
	})

	wantDir := filepath.Join(t.TempDir(), "project")
	planDir = wantDir
	planJSON = true
	planCheck = true
	planApply = true
	planProfile = "web"
	planSkills = []string{"foo", "bar"}

	resetPlanFlags()()

	if planDir != wantDir {
		t.Errorf("resetPlanFlags lost planDir: got %q, want %q", planDir, wantDir)
	}
	if !planJSON {
		t.Errorf("resetPlanFlags lost planJSON: got false, want true")
	}
	if !planCheck {
		t.Errorf("resetPlanFlags lost planCheck: got false, want true")
	}
	if !planApply {
		t.Errorf("resetPlanFlags lost planApply: got false, want true")
	}
	if planProfile != "web" {
		t.Errorf("resetPlanFlags lost planProfile: got %q, want %q", planProfile, "web")
	}
	if len(planSkills) != 2 || planSkills[0] != "foo" || planSkills[1] != "bar" {
		t.Errorf("resetPlanFlags lost planSkills: got %v, want [foo bar]", planSkills)
	}
}

package curation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/project"
)

func TestApplyRefusesBootstrapWithoutTarget(t *testing.T) {
	env := newTestEnv(t)
	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	if !plan.Baseline.IsBootstrap() {
		t.Fatal("expected bootstrap plan")
	}
	_, err = plan.Apply(ApplyOptions{ApplyRemovals: true})
	if err == nil {
		t.Fatal("expected error applying bootstrap plan without explicit target")
	}
}

func TestApplyRemovesOwnedLinkInstall(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("foo")
	env.symlinkAgentSkill("claude", "foo")
	env.writeConfig(&project.Config{
		Curation: &project.Curation{Managed: map[string][]string{"claude": {"foo"}}},
	})

	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	res, err := plan.Apply(ApplyOptions{ApplyRemovals: true})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Removed) != 1 {
		t.Errorf("expected 1 removal, got %d (%v)", len(res.Removed), res.Removed)
	}
	link := filepath.Join(env.projectDir, filepath.FromSlash(".claude/skills/foo"))
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("expected link %s to be removed, stat err=%v", link, err)
	}
	// managed should be cleared.
	pm := project.NewManager(env.projectDir)
	config, err := pm.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if config.Curation != nil && len(config.Curation.Managed) != 0 {
		t.Errorf("expected managed cleared, got %+v", config.Curation.Managed)
	}
}

func TestApplyDoesNotRemoveUnowned(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("foo")
	env.symlinkAgentSkill("claude", "foo") // unowned Link Install
	env.mkCopySkill("manual")
	// Curated project with empty managed: makes a non-bootstrap plan without
	// granting ownership to foo or manual.
	env.writeConfig(&project.Config{
		Curation: &project.Curation{Managed: map[string][]string{}},
	})

	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	res, err := plan.Apply(ApplyOptions{ApplyRemovals: true})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Removed) != 0 {
		t.Errorf("unowned remove must not be applied, got %v", res.Removed)
	}
	for _, name := range []string{"foo", "manual"} {
		p := filepath.Join(env.projectDir, filepath.FromSlash(".claude/skills/"+name))
		if _, err := os.Lstat(p); os.IsNotExist(err) {
			t.Errorf("expected %s to remain", name)
		}
	}
}

func TestApplyWithoutRemovalFlagLeavesEverything(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("foo")
	env.symlinkAgentSkill("claude", "foo")
	env.writeConfig(&project.Config{
		Curation: &project.Curation{Managed: map[string][]string{"claude": {"foo"}}},
	})
	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	res, err := plan.Apply(ApplyOptions{ApplyRemovals: false})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Removed) != 0 {
		t.Errorf("ApplyRemovals=false must not remove, got %v", res.Removed)
	}
	link := filepath.Join(env.projectDir, filepath.FromSlash(".claude/skills/foo"))
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("link should remain: %v", err)
	}
}

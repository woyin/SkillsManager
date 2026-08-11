package curation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/woyin/skills-manager/internal/profile"
	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/tool"
)

// newTestEnv builds a temp registry + profiles dir and a project dir, returning
// a Planner wired to them.
type testEnv struct {
	registryDir string
	profilesDir string
	projectDir  string
	tools       []tool.Tool
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	reg := filepath.Join(t.TempDir(), "registry")
	prof := filepath.Join(t.TempDir(), "profiles")
	proj := t.TempDir()
	// Only claude for deterministic scanning.
	return &testEnv{registryDir: reg, profilesDir: prof, projectDir: proj, tools: []tool.Tool{tool.Claude}}
}

func (e *testEnv) planner() *Planner {
	return NewPlanner(e.registryDir, e.profilesDir, e.tools, e.projectDir)
}

func (e *testEnv) writeConfig(config *project.Config) {
	t := e // reuse
	_ = t
	pm := project.NewManager(e.projectDir)
	_ = pm.Save(config)
}

// mkRegistrySkill creates a registry skill original under global category.
func (e *testEnv) mkRegistrySkill(name string) {
	dir := filepath.Join(e.registryDir, "skills", "global", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name+"\n"), 0644)
}

// symlinkAgentSkill creates a project-scope Link Install pointing at a registry skill.
func (e *testEnv) symlinkAgentSkill(agent string, name string) {
	pm := &project.Manager{}
	_ = pm
	linkHost := filepath.Join(e.projectDir, tool.Claude.ProjectSkillDir)
	if agent != "" {
		t := tool.ToolByName(agent)
		if t != nil {
			linkHost = filepath.Join(e.projectDir, t.ProjectSkillDir)
		}
	}
	_ = os.MkdirAll(linkHost, 0755)
	src := filepath.Join(e.registryDir, "skills", "global", name)
	_ = os.Symlink(src, filepath.Join(linkHost, name))
}

// mkCopySkill creates a plain directory (manual / copy) install.
func (e *testEnv) mkCopySkill(name string) {
	dir := filepath.Join(e.projectDir, tool.Claude.ProjectSkillDir, name)
	_ = os.MkdirAll(dir, 0755)
	_ = os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name+"\n"), 0644)
}

func TestBootstrapPlanForProjectWithoutConfig(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("foo")
	planner := env.planner()
	plan, err := planner.PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	if !plan.Bootstrap {
		t.Errorf("expected bootstrap plan for project without explicit target")
	}
	if plan.ExplicitTargetSet {
		t.Errorf("expected explicit target unset for bootstrap")
	}
	if !plan.CheckOK {
		t.Errorf("bootstrap plan should be check-ok")
	}
	if len(plan.Proposals) != 0 {
		t.Errorf("bootstrap plan should propose nothing to add/remove, got %+v", plan.Proposals)
	}
}

func TestPlanLeavesInstalledBaselineMember(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("foo")
	env.symlinkAgentSkill("claude", "foo")
	env.writeConfig(&project.Config{Skills: []string{"foo"}})

	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	if plan.Bootstrap {
		t.Fatal("expected non-bootstrap for explicit skills")
	}
	found := false
	for _, pr := range plan.Proposals {
		if pr.Skill == "foo" && pr.Agent == "claude" {
			if pr.Action != ActionLeave {
				t.Errorf("baseline member should be leave, got %s", pr.Action)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("expected a proposal for foo, got %+v", plan.Proposals)
	}
	if plan.NeedsAction() {
		t.Errorf("expected no action needed when baseline satisfied")
	}
	if !plan.CheckOK {
		t.Errorf("expected check-ok when baseline satisfied")
	}
}

func TestPlanOwnedLinkInstallRemovable(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("foo")
	env.symlinkAgentSkill("claude", "foo")
	// Baseline does not include foo, and foo is recorded as owned.
	env.writeConfig(&project.Config{
		Curation: &project.Curation{
			Managed: map[string][]string{"claude": {"foo"}},
		},
	})

	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	for _, pr := range plan.Proposals {
		if pr.Skill == "foo" && pr.Agent == "claude" {
			if pr.Action != ActionRemove {
				t.Errorf("unowned-but-recorded should be removable, got %+v", pr.Action)
			}
			if !pr.Owned {
				t.Errorf("expected owned=true for recorded Link Install")
			}
		}
	}
}

func TestPlanDoesNotRemoveUnownedOrCopy(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("foo")
	env.symlinkAgentSkill("claude", "foo") // Link Install, not owned
	env.mkCopySkill("manual")              // plain dir (manual/copy)

	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	for _, pr := range plan.Proposals {
		if pr.Skill == "manual" {
			if pr.Action != ActionLeave {
				t.Errorf("manual dir must never be removed, got %s", pr.Action)
			}
			if pr.Owned {
				t.Errorf("manual dir must not be owned")
			}
		}
		if pr.Skill == "foo" {
			if pr.Owned {
				t.Errorf("unowned Link Install must not be owned")
			}
			if pr.Action != ActionRemove {
				t.Errorf("unowned Link Install is a remove candidate (unsafe), got %s", pr.Action)
			}
		}
	}
}

func TestPlanProposesAddForMissingBaselineMember(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("foo")
	env.writeConfig(&project.Config{Skills: []string{"foo"}})

	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	found := false
	for _, pr := range plan.Proposals {
		if pr.Skill == "foo" && pr.Action == ActionAdd {
			found = true
		}
	}
	if !found {
		t.Errorf("expected add proposal for missing baseline member foo, got %+v", plan.Proposals)
	}
}

func TestPlanExpandsProfileSkills(t *testing.T) {
	env := newTestEnv(t)
	os.MkdirAll(env.profilesDir, 0755)
	loader := profile.NewLoader(env.profilesDir)
	if err := loader.Save("web", &profile.Profile{Skills: []string{"foo", "bar"}, MCP: []string{"github"}}); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	env.mkRegistrySkill("foo")
	env.symlinkAgentSkill("claude", "foo")
	env.writeConfig(&project.Config{Profile: "web"})

	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	// foo is leave (in baseline), bar is add (missing).
	for _, pr := range plan.Proposals {
		if pr.Skill == "foo" && pr.Action == ActionLeave {
			// ok
		}
		if pr.Skill == "bar" && pr.Action == ActionAdd {
			// ok
		}
	}
	if !plan.NeedsAction() {
		t.Errorf("expected a need action because bar is missing")
	}
}

func TestPlanWarnsOnMissingProfile(t *testing.T) {
	env := newTestEnv(t)
	env.writeConfig(&project.Config{Profile: "nope"})
	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	if len(plan.Warnings) == 0 {
		t.Errorf("expected warning for missing profile")
	}
}

func TestPlanEvaluateJSON(t *testing.T) {
	env := newTestEnv(t)
	env.mkRegistrySkill("foo")
	env.symlinkAgentSkill("claude", "foo")
	env.writeConfig(&project.Config{Skills: []string{"foo"}})
	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	ev := plan.Evaluate()
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"project"`) || !strings.Contains(s, `"check"`) {
		t.Errorf("expected JSON fields in %s", s)
	}
}

func TestBootstrapRecommendationsPopulated(t *testing.T) {
	env := newTestEnv(t)
	os.MkdirAll(env.profilesDir, 0755)
	loader := profile.NewLoader(env.profilesDir)
	if err := loader.Save("web", &profile.Profile{Skills: []string{"foo"}}); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	env.mkRegistrySkill("foo")

	plan, err := env.planner().PlanForProject()
	if err != nil {
		t.Fatalf("PlanForProject: %v", err)
	}
	if !plan.Bootstrap {
		t.Fatal("expected bootstrap")
	}
	if len(plan.RecommendedProfiles) != 1 || plan.RecommendedProfiles[0] != "web" {
		t.Errorf("expected recommended profile web, got %v", plan.RecommendedProfiles)
	}
	found := false
	for _, o := range plan.RecommendedSkills {
		if o == "foo" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected foo in recommended skills, got %v", plan.RecommendedSkills)
	}
}

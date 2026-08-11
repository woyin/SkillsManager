package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/lockfile"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
	"github.com/woyin/skills-manager/internal/wellknown"
)

func TestInstallFromSourceRegistersAndInstallsLocalSkill(t *testing.T) {
	projectDir := t.TempDir()
	source := filepath.Join(t.TempDir(), "direct-skill")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: direct-skill\ndescription: a local direct-install test skill\n---\n# Direct Skill\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldRegistry, oldData := RegistryDir, DataDir
	oldInstallDir, oldAgents := installDir, installAgents
	oldSkills, oldCopy := installSkills, installCopy
	oldRef, oldOffline := installRef, installOffline
	oldFullDepth, oldYes, oldAll := installFullDepth, installYes, installAll
	oldGlobal, oldSubagents := installGlobal, installSubagents
	RegistryDir = filepath.Join(t.TempDir(), "registry")
	DataDir = t.TempDir()
	installDir = projectDir
	installAgents = []string{"codex"}
	installSkills = nil
	installCopy = false
	installRef = ""
	installOffline = false
	installFullDepth = false
	installYes = true
	installAll = false
	installGlobal = false
	installSubagents = nil
	t.Cleanup(func() {
		RegistryDir, DataDir = oldRegistry, oldData
		installDir, installAgents = oldInstallDir, oldAgents
		installSkills, installCopy = oldSkills, oldCopy
		installRef, installOffline = oldRef, oldOffline
		installFullDepth, installYes, installAll = oldFullDepth, oldYes, oldAll
		installGlobal, installSubagents = oldGlobal, oldSubagents
	})

	if err := installFromSource(source); err != nil {
		t.Fatalf("installFromSource: %v", err)
	}
	registered := filepath.Join(RegistryDir, "skills", "global", "direct-skill")
	if _, err := os.Stat(filepath.Join(registered, "SKILL.md")); err != nil {
		t.Fatalf("registered skill missing: %v", err)
	}
	codex := tool.ToolByName("codex")
	if codex == nil {
		t.Fatal("codex should be in the supported tool catalog")
	}
	installed := filepath.Join(tool.GetProjectSkillDir(*codex, projectDir), "direct-skill")
	info, err := os.Lstat(installed)
	if err != nil {
		t.Fatalf("installed skill missing: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("installed skill is not a Link Install: %v", info.Mode())
	}
	if _, err := os.Stat(filepath.Join(projectDir, "skills-lock.json")); err != nil {
		t.Fatalf("lockfile missing: %v", err)
	}
}

func TestInstallFromSourceRejectsRefForLocalSource(t *testing.T) {
	oldRef := installRef
	installRef = "main"
	t.Cleanup(func() { installRef = oldRef })
	if err := installFromSource(t.TempDir()); err == nil {
		t.Fatal("local source with --ref should fail")
	}
}

func TestInstallFromLockFileRestoresNonLocalEntries(t *testing.T) {
	projectDir := t.TempDir()
	source := filepath.Join(t.TempDir(), "locked-skill")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: locked-skill\ndescription: a lock restore test skill\n---\n# Locked Skill\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lock := &lockfile.LocalLock{Skills: map[string]*lockfile.SkillEntry{
		"locked-skill": {
			Source:       source,
			SourceURL:    source,
			SourceType:   "github",
			ComputedHash: "test-hash",
		},
		"local-snapshot": {
			Source:       "/unavailable/local-source",
			SourceType:   "local",
			ComputedHash: "test-hash",
		},
	}}
	if err := lockfile.NewManager(projectDir).Save(lock); err != nil {
		t.Fatal(err)
	}

	oldRegistry, oldData := RegistryDir, DataDir
	oldInstallDir, oldAgents := installDir, installAgents
	oldSkills, oldCopy, oldRef := installSkills, installCopy, installRef
	oldSubagents, oldGlobal := installSubagents, installGlobal
	RegistryDir, DataDir = filepath.Join(t.TempDir(), "registry"), t.TempDir()
	installDir, installAgents = projectDir, []string{"codex"}
	installSkills, installCopy, installRef, installSubagents, installGlobal = nil, false, "", nil, false
	t.Cleanup(func() {
		RegistryDir, DataDir = oldRegistry, oldData
		installDir, installAgents = oldInstallDir, oldAgents
		installSkills, installCopy, installRef = oldSkills, oldCopy, oldRef
		installSubagents, installGlobal = oldSubagents, oldGlobal
	})

	if err := installFromLockFile(nil); err != nil {
		t.Fatalf("installFromLockFile: %v", err)
	}
	codex := tool.ToolByName("codex")
	if codex == nil {
		t.Fatal("codex should be in the supported tool catalog")
	}
	installed := filepath.Join(tool.GetProjectSkillDir(*codex, projectDir), "locked-skill")
	if info, err := os.Lstat(installed); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("restored Link Install = %v, %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(tool.GetProjectSkillDir(*codex, projectDir), "local-snapshot")); !os.IsNotExist(err) {
		t.Fatalf("local lock entry should be skipped: %v", err)
	}
}

func TestInstallFromWellKnownSourceUsesInjectedFetcher(t *testing.T) {
	projectDir := t.TempDir()
	oldRegistry, oldData := RegistryDir, DataDir
	oldInstallDir, oldAgents := installDir, installAgents
	oldSkills, oldCopy, oldList := installSkills, installCopy, installList
	oldRef, oldOffline, oldGlobal := installRef, installOffline, installGlobal
	oldFetcher := fetchWellKnownSkills
	RegistryDir, DataDir = filepath.Join(t.TempDir(), "registry"), t.TempDir()
	installDir, installAgents = projectDir, []string{"codex"}
	installSkills, installCopy, installList = nil, false, false
	installRef, installOffline, installGlobal = "", false, false
	fetchWellKnownSkills = func(context.Context, string) ([]wellknown.Skill, error) {
		return []wellknown.Skill{{
			Name:        "well-known-skill",
			InstallName: "well-known-skill",
			Description: "a deterministic Well-Known Source test skill",
			Files: map[string][]byte{
				"SKILL.md":              []byte("---\nname: well-known-skill\ndescription: a deterministic Well-Known Source test skill\n---\n# Well Known\n"),
				"references/example.md": []byte("reference"),
			},
		}}, nil
	}
	t.Cleanup(func() {
		RegistryDir, DataDir = oldRegistry, oldData
		installDir, installAgents = oldInstallDir, oldAgents
		installSkills, installCopy, installList = oldSkills, oldCopy, oldList
		installRef, installOffline, installGlobal = oldRef, oldOffline, oldGlobal
		fetchWellKnownSkills = oldFetcher
	})

	if err := installFromWellKnownSource("https://skills.example.test"); err != nil {
		t.Fatalf("installFromWellKnownSource: %v", err)
	}
	if _, err := os.Stat(filepath.Join(RegistryDir, "skills", "global", "well-known-skill", "references", "example.md")); err != nil {
		t.Fatalf("materialized registry skill missing supporting file: %v", err)
	}
	codex := tool.ToolByName("codex")
	if codex == nil {
		t.Fatal("codex should be in the supported tool catalog")
	}
	if _, err := os.Lstat(filepath.Join(tool.GetProjectSkillDir(*codex, projectDir), "well-known-skill")); err != nil {
		t.Fatalf("Well-Known Source skill was not installed: %v", err)
	}
}

func TestInstallFromWellKnownSourceRejectsUnsupportedFlagsAndLists(t *testing.T) {
	oldRef, oldOffline, oldList := installRef, installOffline, installList
	oldFetcher := fetchWellKnownSkills
	installRef, installOffline, installList = "main", false, false
	if err := installFromWellKnownSource("https://skills.example.test"); err == nil {
		t.Fatal("Well-Known Source should reject --ref")
	}
	installRef, installOffline, installList = "", true, false
	if err := installFromWellKnownSource("https://skills.example.test"); err == nil {
		t.Fatal("Well-Known Source should reject --offline")
	}
	installRef, installOffline, installList = "", false, true
	fetchWellKnownSkills = func(context.Context, string) ([]wellknown.Skill, error) {
		return []wellknown.Skill{{InstallName: "listed", Description: "listed skill"}}, nil
	}
	t.Cleanup(func() {
		installRef, installOffline, installList = oldRef, oldOffline, oldList
		fetchWellKnownSkills = oldFetcher
	})
	if err := installFromWellKnownSource("https://skills.example.test"); err != nil {
		t.Fatalf("Well-Known Source list: %v", err)
	}
	fetchWellKnownSkills = func(context.Context, string) ([]wellknown.Skill, error) {
		return nil, errors.New("source unavailable")
	}
	if err := installFromWellKnownSource("https://skills.example.test"); err == nil {
		t.Fatal("Well-Known Source fetch error should be returned")
	}
}

func TestSelectSkillsForInstallHonorsFiltersAndNonInteractiveSelection(t *testing.T) {
	oldSkills, oldYes := installSkills, installYes
	t.Cleanup(func() { installSkills, installYes = oldSkills, oldYes })
	discovered := []registry.DiscoveredSkill{{Name: "alpha"}, {Name: "beta"}}

	installSkills, installYes = []string{"BETA"}, false
	selected, err := selectSkillsForInstall(discovered, installSkills)
	if err != nil || len(selected) != 1 || selected[0].Name != "beta" {
		t.Fatalf("filtered selection = %#v, %v", selected, err)
	}
	installSkills = []string{"*"}
	selected, err = selectSkillsForInstall(discovered, installSkills)
	if err != nil || len(selected) != 2 {
		t.Fatalf("wildcard selection = %#v, %v", selected, err)
	}
	installSkills, installYes = nil, true
	selected, err = selectSkillsForInstall(discovered, nil)
	if err != nil || len(selected) != 2 {
		t.Fatalf("non-interactive selection = %#v, %v", selected, err)
	}
	if _, err := selectSkillsForInstall(nil, nil); err == nil {
		t.Fatal("empty discovery should fail")
	}
	if _, err := selectSkillsForInstall(discovered, []string{"missing"}); err == nil {
		t.Fatal("unmatched filter should fail")
	}
}

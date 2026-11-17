package lifecycle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

func writeSkillSource(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: demo\ndescription: fixture\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestBuildJobsDedupesUniversalProjectDir 验证多个 Agent 解析到同一项目目录时
// 只保留一个安装目标（.agents/skills 共享目录）。
func TestBuildJobsDedupesUniversalProjectDir(t *testing.T) {
	project := t.TempDir()
	tools := []tool.Tool{
		{Name: "codex", ProjectSkillDir: ".agents/skills"},
		{Name: "gemini", ProjectSkillDir: ".agents/skills"},
		{Name: "cursor", ProjectSkillDir: ".agents/skills"},
	}
	skills := []registry.DiscoveredSkill{{Name: "demo", Path: "/src/demo"}}

	jobs := BuildJobs(tools, skills, BuildOptions{Project: true, ProjectDir: project})
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want 3 before dedupe", len(jobs))
	}
	deduped := DedupeJobs(jobs)
	if len(deduped) != 1 {
		t.Fatalf("deduped = %d, want 1", len(deduped))
	}
	want := filepath.Join(project, ".agents", "skills", "demo")
	if deduped[0].Dest != want {
		t.Fatalf("dest = %s, want %s", deduped[0].Dest, want)
	}
}

// TestBuildJobsExpandsEveSubagents 验证 Eve --subagent 展开：root 用根目录，
// 具名子代理用 agent/subagents/<name>/skills。
func TestBuildJobsExpandsEveSubagents(t *testing.T) {
	project := t.TempDir()
	eve := tool.Tool{Name: "eve", ProjectSkillDir: "agent/skills"}
	skills := []registry.DiscoveredSkill{{Name: "demo", Path: "/src/demo"}}

	jobs := BuildJobs([]tool.Tool{eve}, skills, BuildOptions{
		Project:    true,
		ProjectDir: project,
		Subagents:  []string{"root", "reviewer"},
	})
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2: %+v", len(jobs), jobs)
	}
	if got := jobs[0].Dest; got != filepath.Join(project, "agent", "skills", "demo") {
		t.Fatalf("root dest = %s", got)
	}
	if got := jobs[1].Dest; got != filepath.Join(project, "agent", "subagents", "reviewer", "skills", "demo") {
		t.Fatalf("subagent dest = %s", got)
	}
	if jobs[1].Subagent != "reviewer" || jobs[0].Subagent != "root" {
		t.Fatalf("subagent labels = %q/%q", jobs[0].Subagent, jobs[1].Subagent)
	}
}

// TestApplyJobsInstallsAndCommits 验证成功落地被计为 Installed 且实体存在。
func TestApplyJobsInstallsAndCommits(t *testing.T) {
	source := writeSkillSource(t, filepath.Join(t.TempDir(), "demo"))
	destParent := t.TempDir()
	jobs := []InstallJob{{ToolName: "claude", Dest: filepath.Join(destParent, "demo"), SkillName: "demo", SourceDir: source}}

	result := ApplyJobs(jobs, InstallOptions{})
	if len(result.Installed) != 1 || len(result.Failures) != 0 {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Lstat(filepath.Join(destParent, "demo")); err != nil {
		t.Fatalf("destination missing: %v", err)
	}
}

// TestApplyJobsReportsPerJobFailure 验证单任务失败被记录且不影响结果结构。
func TestApplyJobsReportsPerJobFailure(t *testing.T) {
	source := writeSkillSource(t, filepath.Join(t.TempDir(), "demo"))
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
	jobs := []InstallJob{{ToolName: "claude", Dest: filepath.Join(blocker, "demo"), SkillName: "demo", SourceDir: source}}

	result := ApplyJobs(jobs, InstallOptions{})
	if len(result.Installed) != 0 || len(result.Failures) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if result.Failures[0].Err == nil || result.Failures[0].Job.SkillName != "demo" {
		t.Fatalf("failure = %+v", result.Failures[0])
	}
}

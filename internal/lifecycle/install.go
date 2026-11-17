// Package lifecycle 收拢安装与删除的可复用业务单元：安装 job 构建/去重与
// Placement 批处理结果，以及删除引用扫描/清理（显式入参，不读 cmd 全局）。
// CLI 仍负责 flags、确认、输出与 ADR 0017 的拒绝/force 决策；本包不做策略。
//
// Input: fmt, path/filepath, github.com/woyin/skills-manager/internal/installer, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/internal/tool
// Output: type InstallJob, type BuildOptions, func BuildJobs, func DedupeJobs, type InstallOptions, type JobFailure, type InstallResult, func ApplyJobs
// Pos: 业务层-安装 job 与 Placement 批处理
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package lifecycle

import (
	"path/filepath"

	"github.com/woyin/skills-manager/internal/installer"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

// InstallJob 是一个 (Agent 目录 × Skill) 的安装目标。
type InstallJob struct {
	ToolName  string
	Dest      string
	SkillName string
	SourceDir string
	Subagent  string // Eve 子代理名；空表示非子代理目标
}

// BuildOptions 控制任务构建；Subagents 只对 Eve 生效。
type BuildOptions struct {
	Project    bool
	ProjectDir string
	Subagents  []string
}

// BuildJobs 为每个 (agent 目录 × skill) 组合生成安装任务。
// Eve 的 --subagent 可重复："root"/"."/"" 指根 Eve agent（普通 agent/skills
// 目录），其余名字安装到 agent/subagents/<name>/skills。
func BuildJobs(targetTools []tool.Tool, skills []registry.DiscoveredSkill, opts BuildOptions) []InstallJob {
	jobs := make([]InstallJob, 0, len(targetTools)*len(skills))
	for _, t := range targetTools {
		if t.Name == "eve" && len(opts.Subagents) > 0 {
			jobs = append(jobs, eveSubagentJobs(t, skills, opts)...)
			continue
		}
		scope := installer.ProjectScope
		if !opts.Project {
			scope = installer.GlobalScope
		}
		for _, target := range installer.TargetDirectories([]tool.Tool{t}, opts.ProjectDir, scope) {
			for _, skill := range skills {
				jobs = append(jobs, InstallJob{
					ToolName:  t.Name,
					Dest:      filepath.Join(target.Directory, skill.Name),
					SkillName: skill.Name,
					SourceDir: skill.Path,
				})
			}
		}
	}
	return jobs
}

// eveSubagentJobs 为 Eve 的每个子代理目标生成任务；root/./"" 解析到根 agent
// 目录（目录不存在则跳过该目标）。
func eveSubagentJobs(t tool.Tool, skills []registry.DiscoveredSkill, opts BuildOptions) []InstallJob {
	jobs := make([]InstallJob, 0, len(opts.Subagents)*len(skills))
	for _, sub := range opts.Subagents {
		var dir string
		if sub == "root" || sub == "." || sub == "" {
			dir = tool.GetProjectSkillDir(t, opts.ProjectDir)
			if dir == "" {
				continue
			}
		} else {
			dir = filepath.Join(opts.ProjectDir, "agent", "subagents", sub, "skills")
		}
		for _, skill := range skills {
			jobs = append(jobs, InstallJob{
				ToolName:  t.Name,
				Dest:      filepath.Join(dir, skill.Name),
				SkillName: skill.Name,
				SourceDir: skill.Path,
				Subagent:  sub,
			})
		}
	}
	return jobs
}

// DedupeJobs 保留每个目标路径的第一个任务。多个 Agent 可能解析到同一目录
// （项目级共享 .agents/skills），重复任务会在并发落地时互相竞争。
func DedupeJobs(jobs []InstallJob) []InstallJob {
	if len(jobs) <= 1 {
		return jobs
	}
	seen := make(map[string]bool, len(jobs))
	out := make([]InstallJob, 0, len(jobs))
	for _, j := range jobs {
		if seen[j.Dest] {
			continue
		}
		seen[j.Dest] = true
		out = append(out, j)
	}
	return out
}

// InstallOptions 控制落地模式。
type InstallOptions struct {
	CopyMode bool
}

// JobFailure 记录单个任务失败。
type JobFailure struct {
	Job InstallJob
	Err error
}

// InstallResult 汇总一次批处理：Installed 保持任务顺序，Failures 保存每项
// 落地/提交错误。未产生实体的任务（例如无变化）既不计数也不算失败。
type InstallResult struct {
	Installed []InstallJob
	Failures  []JobFailure
}

// ApplyJobs 用共享 Placement 引擎并发落地并提交。单个失败不阻止其他任务。
func ApplyJobs(jobs []InstallJob, opts InstallOptions) InstallResult {
	result := InstallResult{}
	if len(jobs) == 0 {
		return result
	}
	mode := installer.SymlinkMode
	fallback := installer.CopyOnSymlinkFailure
	if opts.CopyMode {
		mode = installer.CopyMode
		fallback = installer.NoSymlinkFallback
	}
	placer := installer.NewPlacement(installer.PlacementOptions{
		Mode:          mode,
		Fallback:      fallback,
		Conflict:      installer.ReplaceOnConflict,
		RejectOverlap: true,
	})
	requests := make([]installer.PlacementRequest, len(jobs))
	for i, job := range jobs {
		requests[i] = installer.PlacementRequest{
			Source:      job.SourceDir,
			Destination: job.Dest,
			Label:       job.SkillName,
		}
	}
	for i, outcome := range placer.PlaceMany(requests, 8) {
		job := jobs[i]
		if outcome.Err != nil {
			result.Failures = append(result.Failures, JobFailure{Job: job, Err: outcome.Err})
			continue
		}
		if outcome.Result == nil || !outcome.Result.Applied {
			continue
		}
		if err := outcome.Result.Commit(); err != nil {
			result.Failures = append(result.Failures, JobFailure{Job: job, Err: err})
			continue
		}
		result.Installed = append(result.Installed, job)
	}
	return result
}

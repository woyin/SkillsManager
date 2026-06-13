# SkillsManager v3 — 对齐 npx skills、性能优化、重构

> 创建于 2026-06-13。基于对 `npx skills`（vercel-labs/skills）README 的核对与对 sm 源码的逐项验证。

## 目标

1. **功能对齐**：补齐 sm 相对 npx skills 的真实行为差距（经验证仅 1 项真 gap）。
2. **性能提升 ~30%**：先建 Go benchmark 基准，再迭代并行化与 I/O 优化，用数据验证。
3. **重构 + 注释**：抽公共包、合并重复实现、用标准库替换手写工具、补 Go doc 注释；保留现有接口与测试。

## 范围决策（用户已拍板，2026-06-13）

- **保持 sm 的 registry + symlink + profile 单实例模型**，不引入 npx 的"项目级复制安装"默认行为。sm 的 `list` 默认列注册表（这是 sm 身份），已安装视图通过 `sm list -a` 获取。
- **性能**：先建基准再迭代，目标关键命令（add/update）实测提升 30%。
- **重构力度中等**：抽 `internal/source`、`internal/fsutil` 公共包；`specialFlags` 从 tool 注册表派生；补注释。不重新组织 cmd/internal 分层。

## 任务一：功能对齐（仅 1 项真 gap）

经源码核实，4 个疑似差距的真实状态：

| 差距 | 状态 | 结论 |
|---|---|---|
| 已安装技能列表 | 已实现（`sm list -a` 扫描 agent 目录） | 不动，保持 sm 默认语义 |
| Plugin manifest 发现 | 完整实现（registry.go:324-403） | 不动 |
| `metadata.internal` 隐藏 | **未实现（真 gap）** | **需实现** |
| `-a '*'` 通配 | 完整实现（tool.go:493） | 不动 |

### 1.1 实现 `metadata.internal` + `INSTALL_INTERNAL_SKILLS`

**变更：**

- `internal/registry/registry.go`：`DiscoveredSkill` 增加 `Internal bool` 字段。
- 统一 frontmatter 解析：新增 `parseSkillFrontmatter(skillMDPath) (description string, internal bool)`，解析 `metadata.internal: true`。现有 `parseSkillDescription`（registry.go:422）、`cmd/add.go:327 parseSkillDesc`、`cmd/find.go:224 extractDescription` 三处重复解析收敛到这一处。
- `DiscoverSkills`：发现技能时填充 `Internal` 字段；并读取环境变量 `INSTALL_INTERNAL_SKILLS`，若未置位（`1`/`true`）则跳过 `Internal==true` 的技能（不在 list/安装中显示）。
- `cmd/add.go addWithAgents`、`cmd/add.go listSkillsFromSource`：同样尊重该环境变量（通过收敛后的解析函数自动生效）。

**测试：** 新增 `internal/registry` 测试覆盖：(a) 含 `metadata.internal: true` 的 SKILL.md 默认被隐藏；(b) `INSTALL_INTERNAL_SKILLS=1` 时显示；(c) 无该字段的技能不受影响。

## 任务二：性能优化（目标关键命令 +30%）

### 2.1 建立基准（先行）

- 新增 `internal/registry/bench_test.go`、`cmd/bench_test.go`：用 `testing.B` 为 `DiscoverSkills`、`copyDirRecursive`、`AddSkillWithOptions`（本地路径模式，避免网络抖动）建基准。
- 记录优化前基线数据（ops/sec、allocs），写入本文件"基线"小节。

### 2.2 SQLite：开 WAL + 连接复用

- `internal/db/db.go Open`：执行 `PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA busy_timeout=5000;`。WAL 显著降低写放大与锁竞争。
- 为高频读路径（web handler 每次 Open）评估连接池：`sql.DB.SetMaxOpenConns`、复用单例。最小改动：仅在 `Open` 加 pragma（纯收益，无接口变更）。

### 2.3 `update`：并行 git pull

- `cmd/update.go`：当前串行 `git pull`。改为有界 worker pool（`runtime.NumCPU()` 与 8 取小），每个 git 仓库一个任务。
- 用 `sync.WaitGroup` + buffered channel 控制并发；保留逐条进度输出（用 mutex 串行化 stdout）。
- 保持 `-g/-p/-y` 语义不变。

### 2.4 `add --agent`：并行安装阶段

- `cmd/add.go addWithAgents`：clone 一次后，对 (agent × skill) 安装操作并行化（worker pool，并发度同上）。
- symlink/copy 互相独立，天然可并行；仅汇总输出需 mutex。
- `--copy` 模式下同样并行复制。

### 2.5 `copyDirRecursive`：流式 + 跳过 .git

- 当前用 `os.ReadFile/WriteFile` 全量读入内存，且**会复制 `.git` 目录**（潜在大目录，浪费 I/O）。
- 改为 `io.Copy` 流式复制；跳过名为 `.git` 的目录条目。
- 此函数被 `copyDir`（registry）、`copyDirAll`/`copySkillDir`（add.go）、`copyDirRecursive` 共 3 处近重复实现 —— 见任务三收敛。

### 基线与验证

优化后重跑同一 benchmark，对比 ops/sec。目标：`update`（多仓库场景）与 `add --agent`（多 agent 场景）wall-clock 提升 ≥30%。将前后数据记入本文件。

## 任务三：重构 + 注释

### 3.1 抽 `internal/fsutil` 公共包

- 合并 3 份重复的目录复制实现（registry `copyDirRecursive`、add.go `copyDirAll`/`copySkillDir`）为 `internal/fsutil.CopyDir(src, dst, opts)`，支持"跳过 .git""流式复制"。
- 该包单一职责：文件系统辅助操作。

### 3.2 抽 `internal/source` 公共包

- 收敛散落三处的 URL/source 解析：`registry.IsGitURL`、`registry.ParseTreeURL`、`registry.normalizeGitURL`、`registry.isGitHubShorthand`、`cmd/add.go normalizeSourceURL/hasScheme/splitBySlash`。
- 暴露 `source.Resolve(raw) (kind, repoURL, branch, subPath, err)` 统一入口。
- 已有 `internal/registry/source_test.go` 测试随包迁移。

### 3.3 `specialFlags` 从 tool 注册表派生

- 当前 `add.go`/`rm.go`/`list.go` 各自硬编码 codex/claude/gemini/opencode/hermes/openclaw 七个布尔 flag。
- 改为遍历 `tool.AllTools()` 自动注册 `<agent>-only` flag（动态生成），消除每命令一份的重复。保留现有 flag 名以兼容。

### 3.4 用标准库替换手写字符串工具

- `cmd/add.go` 的 `splitLines`→`strings.Split(s, "\n")`、`trimSpace`→`strings.TrimSpace`、`splitBySlash`→`strings.Split(s, "/")`、`hasScheme`→用 `url.Parse` 或 `strings.Contains`。这些在收敛到 fsutil/source 后自然消失。

### 3.5 补注释

- 为所有导出符号（`Registry`、`Installer`、`DiscoveredSkill`、`tool.Tool`、新增的 fsutil/source 等）补 Go doc 注释，遵循已有注释密度（项目现有注释偏少，按"包注释 + 导出函数注释"的最小可读标准补齐）。

## 执行顺序与提交粒度

按原子提交推进，每步保持 `go build ./...` + `go test ./...` 通过：

1. `feat: support metadata.internal skills and INSTALL_INTERNAL_SKILLS`（任务一）
2. `perf(db): enable WAL and connection pragmas`（任务二 2.2）
3. `test: add benchmarks for DiscoverSkills and copyDir`（任务二 2.1 基线）
4. `refactor: extract internal/fsutil for dir copy`（任务三 3.1，含跳过 .git + 流式，顺带实现 2.5）
5. `refactor: extract internal/source for URL resolution`（任务三 3.2）
6. `refactor: derive specialFlags from tool registry`（任务三 3.3）
7. `perf(update): parallelize git pull with worker pool`（任务二 2.3）
8. `perf(add): parallelize agent install phase`（任务二 2.4）
9. `docs: add Go doc comments across packages`（任务三 3.5）
10. `test: record before/after benchmark results`（任务二验证）

## 风险与回滚

- **并行化引入竞态**：worker pool 的共享状态（计数、stdout）必须加锁；每个任务操作独立路径，无共享文件。通过 `go test -race` 验证。
- **specialFlags 动态化破坏 CLI 兼容**：保留现有 flag 名与语义，仅改变注册方式。回归 `cmd/*_test.go`。
- **WAL 在某些环境行为不同**：WAL 对 SQLite 是纯增强，modernc.org/sqlite 完整支持；若极端情况可回退（删 pragma 即可）。
- **source/fsutil 抽包改变导入路径**：内部包，无外部消费者；仅本仓库内 import 调整。

## 不做（YAGNI）

- 不引入项目级 `.//skills/` 复制安装（保留 sm 模型）。
- 不为 list/find 等已足够快的命令做并行化。
- 不重新组织 cmd/internal 整体分层。
- 不加 telemetry（sm 无此需求）。

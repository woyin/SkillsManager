# Skill 启发式评分 + update delta 提示

**日期**：2026-07-03
**范围**：新功能

## 背景与动机

`sm update` 当前是无脑 `git pull --ff-only`：拉到新提交即视为成功，但**新版未必更好**。Skill 可能在更新中退化——description 被改模糊、正文被删减、引入 prompt-injection 风格内容。用户无从判断"这次更新到底改好了还是改坏了"。

引入启发式评分（0-100），在 update 前后对比，把 delta 打印出来，给用户一个客观的"是否真的改善"信号。

## 目标

1. 纯启发式评分函数 `ScoreSkill`，0-100，四维度加权。
2. `sm update` 对每个 git-managed skill 在 pull 前后算分，打印 delta；分数下降加 ⚠ 提示。
3. **仅提示，不拦截**——update 行为不变，exit code 不变。

## 评分设计（0-100）

| 维度 | 满分 | 规则 |
|------|------|------|
| frontmatter | 35 | 复用 `LintSkill` findings：每条 Error 扣 18、Warning 扣 8，扣到 0 |
| 内容量 | 25 | body 字节 ∈ [200, 5000] → 满分；< 200 线性衰减到 0；> 5000 线性衰减到 10（过臃肿也给点分） |
| 结构 | 25 | `##` 标题数：≥ 3 → 满分；0 → 0；1-2 线性插值 |
| 可疑内容 | 15 | 起始满分，每命中一项扣 5（扣到 0）：含 "ignore previous instructions" 类短语、含二进制/不可打印字节块、单行 > 2000 字符 |

总分 = 各维度之和（封顶 100）。`Notes` 记录主要扣分原因（如 "description 过短"、"无结构化标题"）。

## 数据结构

```go
// internal/registry/score.go
type SkillScore struct {
    Total     int
    Breakdown map[string]int // {"frontmatter":35,"content":20,"structure":25,"suspicious":15}
    Notes     []string
}

func (r *Registry) ScoreSkill(skillDir string) *SkillScore
```

`skillDir` 为注册表内相对路径（如 `"global/my-skill"`），与 `LintSkill` 一致。

## update 集成

`pullResult` 扩展：

```go
type pullResult struct {
    label         string
    ok            bool
    before        *registry.SkillScore // pull 前评分（nil 表示非 skill 或评分失败）
    after         *registry.SkillScore
    commitChanged bool                  // git rev-parse HEAD 前后是否不同
}
```

worker 内流程（仅当 repo 是 skill，即位于 `skills/` 子树且含 SKILL.md）：
1. `ScoreSkill(relPath)` → before
2. `git rev-parse HEAD` → beforeHash
3. `git pull --ff-only`
4. `git rev-parse HEAD` → afterHash
5. `ScoreSkill(relPath)` → after
6. `commitChanged = beforeHash != afterHash`

输出格式（commit 变化时才打 delta）：
```
✓ my-skill  (a1b2c3d → d4e5f6g)
  Score: 82 → 88 (+6)  [description 改善]
⚠ my-skill  (a1b2c3d → d4e5f6g)
  Score: 88 → 79 (-9)  [正文删减, 无结构化标题]
```
commit 未变：保持原 `OK` 输出，不打分。

MCP 条目（`mcp/` 子树）不参与评分（无 SKILL.md），`before/after` 留 nil。

## 隔离性

- `ScoreSkill` 纯函数：读目录 + SKILL.md，算分，无副作用。无 DB、无网络。
- 评分逻辑全部在 `score.go`，update.go 只调用与格式化。
- 各 worker 评不同 repo 目录，无共享状态，并发安全。

## 验证

1. `internal/registry/score_test.go`：
   - 每维度正反例（合规 frontmatter 满分 vs 缺 description；body 200-5000 vs 50 字节；3 标题 vs 0；含/不含可疑短语）
   - 总分边界（完美 skill = 100；空 skill = 低分）
2. `cmd/update_test.go` 或 `_bench_test.go`：mock git 仓库，pull 前后 commit 不同 → 验证 delta 输出含 `Score: X → Y`
3. 手测：真实 update 一个有新提交的 skill，观察 delta 行

## 非目标（YAGNI）

- 不存评分历史到 SQLite（仅当前打印）
- 不做用户人工评分 / LLM 评分（明确排除）
- 不做 `sm score` 独立命令（聚焦 update delta；后续可加）
- 不拦截更新（纯 advisory）
- 不评分 MCP（无 SKILL.md）

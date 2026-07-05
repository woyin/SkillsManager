# sm lint：全注册表批量 lint + 评分

**日期**：2026-07-03
**范围**：新命令，复用已有 ScoreSkill + LintSkill

## 背景

`sm add` 的 lint 只在入库时触发一次。已注册的全部 skills 从未被批量复审——存量 skill 若 frontmatter 残缺、内容退化，无人知晓。ScoreSkill（已实现）目前只在 `sm update` delta 中出现，无法主动审视当前注册表质量。

## 目标

独立命令 `sm lint [name...]`：扫全注册表（或指定）skills，输出每个 skill 的评分与关键 finding。

## 设计

### 命令形态

```
sm lint              # 扫全部注册表 skills
sm lint my-skill     # 只扫指定（按 name 匹配，跨 category）
sm lint --strict     # 存在 Error 级 finding 时 exit 1（CI 友好）
```

### 输出格式（表格）

```
NAME                         SCORE  STATUS
global/good                    100  ✓
codex-only/shorty               42  ⚠  description 过短
global/bad                       0  ✗  missing description; missing name
```

- SCORE：`ScoreSkill().Total`（0-100）
- STATUS：`✓`（无 finding）/ `⚠`（仅 warning）/ `✗`（有 error）
- 详情列：拼接 findings 消息（截断到 ~50 字符）

末尾汇总：`X skills, Y warnings, Z errors`。

### 实现要点

- `cmd/lint.go` 新命令。复用：
  - `registry.ListSkillDetails()` 枚举 skills（已有，返回相对路径）
  - `registry.ScoreSkill(rel)` 评分（已有）
  - `registry.LintSkill(rel).Findings`（已有，含 level）
- 不新增评分/lint 逻辑——纯组合 + 格式化。
- `--strict`：统计 error 数，`return fmt.Errorf` 或 `os.Exit(1)` 让 CI 可检测。

### 隔离性

- 纯读，不改任何文件、不写 DB。
- 与 `sm check`（安装完整性）正交，不互相依赖。

## 验证

1. `cmd/lint_test.go`：构造含 good/short/bad skill 的注册表，断言表格行含正确 SCORE 与 STATUS 标记
2. `--strict` 下有 error skill 时 exit ≠ 0
3. 手测：`sm lint` 在含混合质量 skill 的注册表上输出正确

## 非目标

- 不评分 MCP（无 SKILL.md）
- 不修复（纯报告；`--fix` 留待后续）
- 不改 `sm check`

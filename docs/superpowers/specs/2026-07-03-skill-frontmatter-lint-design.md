# Skill 质量门禁：SKILL.md frontmatter lint

**日期**：2026-07-03
**范围**：阶段二（AI 适配方向之一：Skill 质量门禁）

## 背景

Skill（SKILL.md）正成为跨 agent 的事实标准单元。Anthropic 官方规范要求 frontmatter 含 `name` 与 `description`，其中 `description` 直接决定 agent 何时触发该 skill——缺失或空泛的 description 会导致 skill 永不被调用。

sm 当前 `parseFrontmatterBytes` 已抽取 name/description/internal，但**只解析不校验**：一个无 description 或 name 的 SKILL.md 会被静默注册，用户无从知晓 skill 实际不可用。

## 目标

在 `sm add`（注册表入库）环节对 SKILL.md 做轻量 lint，发现问题打印非阻塞警告。不破坏现有流程（警告而非错误），即时提升所有新入库 skill 的可见性。

## 设计

### `internal/registry/lint.go`（新文件）

```go
// LintResult 描述单个 SKILL.md 的质量问题。
type LintResult struct {
	SkillDir   string // 技能目录（相对注册表）
	Errors     []string // 必须修复：skill 基本不可用
	Warnings   []string // 建议修复：影响触发质量
}

// LintSkill 校验单个技能目录下的 SKILL.md frontmatter。
// 返回的 LintResult 始终非 nil；无问题时 Errors/Warnings 均为空。
func (r *Registry) LintSkill(skillDir string) *LintResult
```

校验规则（基于 Anthropic Skill 规范 + 实用经验）：

| 级别 | 规则 |
|------|------|
| Error | 缺少 SKILL.md 文件 |
| Error | frontmatter 缺少 `name` 字段 |
| Error | frontmatter 缺少 `description` 字段 |
| Warning | `description` 过短（< 20 字符）——难以让 agent 判断触发时机 |
| Warning | `description` 过长（> 1024 字符）——超出常见 agent 上下文预算 |
| Warning | `name` 含非法字符（非 `[a-z0-9-]`）——影响跨工具兼容 |

### `cmd/add.go` 集成

`AddSkillWithOptions` 成功后，遍历实际入库的技能目录调用 `LintSkill`，对每条 Error/Warning 打印到 stderr（不阻断，exit 0）。新增辅助函数 `printLintResults`。

### 复用现有解析

`LintSkill` 复用 `parseSkillFrontmatter`（已抽取 name/description），不重复实现 YAML 扫描。

## 行为契约

- `sm add` 成功入库 + lint 全过：输出不变。
- `sm add` 成功入库 + lint 有问题：在成功消息后追加警告段，仍 exit 0。
- lint 不影响 `sm install`、`sm list` 等其它命令（本期范围外）。
- 未来可扩展 `sm check --lint` 批量校验全注册表。

## 验证

1. `go test ./internal/registry/` 含新增 `lint_test.go`（覆盖每条规则的正反例）
2. `go test ./cmd/` 含 add 集成（构造无 description 的 SKILL.md，断言 stderr 含警告）
3. 手测：`sm add` 一个已知合规技能无警告；构造残缺 frontmatter 看警告输出

## 非目标

- 不引入完整 YAML 库（保持零新增依赖）。
- 不做 `sm check --lint` 批量校验（留待后续）。
- 不校验 body 内容（仅 frontmatter 元数据）。

# 结构债重构：特殊目录映射单一来源

**日期**：2026-07-03
**范围**：阶段一，仅项目结构优化（目标第一部分）

## 背景

7 个 first-class agent（claude/codex/gemini/opencode/hermes/openclaw，外加 global）的"特殊目录"映射当前在 **4 处**重复维护：

| # | 位置 | 内容 |
|---|------|------|
| 1 | `internal/registry/types.go` | 7 常量 `Global`/`CodexOnly`/... + `specialDirs` map |
| 2 | `internal/tool/data.go` | catalog 前 6 行隐式 first-class（无字段标记） |
| 3 | `internal/tool/tool.go` | `specialToolByDir` map（6 行硬编码） |
| 4 | `cmd/resolve.go` | `specialFlags` 7 个具名布尔字段 + `specs()` 表 |

新增一个 first-class agent 需同步改 4 处，易漂移、易遗漏。

## 目标

降到 **2 处**，且新增 first-class agent 只需在 catalog 加 1 行。

## 设计

### 不变项

- `internal/registry/types.go`：保留 7 常量与 `specialDirs` map。`registry` 是底层语义包（被 `tool` 之外的底层逻辑使用），不应依赖 `tool`，否则倒置分层。`Global` 额外保留——"全部工具"语义不属于任何单 tool。
- `registry.IsSpecialDir` / `registry.New` 等对外 API 不变。

### 变更项

#### 1. `internal/tool/data.go`：catalog 加 `specialDir` 字段

`toolDef` 增 `specialDir string`。仅 6 行 first-class 填 `<name>-only`：

```go
type toolDef struct {
    name           string
    agentName      string
    skillDir       string
    projectSkillDir string
    configFile     string
    binary         string
    specialDir     string // 非空表示该工具对应注册表特殊目录（如 "codex-only"）
}

// first-class 行示例
{name: "codex", ..., specialDir: "codex-only"},
```

`Tool` 结构体同步增 `SpecialDir string` 字段，`makeTools` 时从 toolDef 拷入。

#### 2. `internal/tool/tool.go`：删 `specialToolByDir`，从 catalog 派生

删除 6 行硬编码 `specialToolByDir` map。改为 init 时遍历 `allTools` 构建：

```go
var specialToolByDir = func() map[string]string {
    m := make(map[string]string)
    for _, t := range allTools {
        if t.SpecialDir != "" {
            m[t.SpecialDir] = t.Name
        }
    }
    return m
}()
```

`NameForSpecialDir` 签名不变。

新增导出函数供 `cmd` 派生 flags：

```go
// SpecialFlagSpec 描述一个 --<agent> 特殊目录标志。
type SpecialFlagSpec struct {
    Flag       string // cobra 标志名，如 "codex"
    SpecialDir string // 对应注册表特殊目录，如 "codex-only"
}

// SpecialFlagSpecs 返回全部单工具特殊目录标志（不含 global）。
func SpecialFlagSpecs() []SpecialFlagSpec
```

#### 3. `cmd/resolve.go`：`specialFlags` 改 map 派生

删除 7 个具名布尔字段。改为：

```go
type specialFlags struct {
    vals map[string]*bool // key = SpecialFlagSpec.Flag
}

func newSpecialFlags() *specialFlags {
    sf := &specialFlags{vals: make(map[string]*bool)}
    for _, spec := range tool.SpecialFlagSpecs() {
        b := false
        sf.vals[spec.Flag] = &b
    }
    return sf
}

func (f *specialFlags) Bind(c *cobra.Command, verb string) {
    // global 单独注册（非 tool 派生）
    // 遍历 tool.SpecialFlagSpecs() 注册 --<flag>
}

func (f *specialFlags) Resolve() string {
    // global 优先匹配（保持原 first-match 行为），再遍历 tool specs
}
```

`global` 标志不属任何单 tool，仍在 `cmd` 内单独处理（一个 bool + 一行）。

`addFlags`/`rmFlags` 的声明与调用点（`add.go:66,80`、`rm.go:198,230`）相应改为 `newSpecialFlags()`。

#### 4. `cmd/resolve_test.go`：改用构造器

`specialFlags{Codex: true}` 字面量不再可用（字段未导出且改 map）。测试改：

```go
func flagsSet(flag string) *specialFlags {
    sf := newSpecialFlags()
    *sf.vals[flag] = true
    return sf
}
// 用例：{"codex", "codex-set", flagsSet("codex"), registry.CodexOnly}
```

global 用例单独构造。

## 行为契约

- `add`/`rm` 的 `--global`/`--codex`/... flag 集合与解析顺序不变。
- `resolve_test.go` 全部用例预期输出不变。
- `tool.NameForSpecialDir("codex-only")` 仍返回 `"codex"`。
- `registry.IsSpecialDir` 行为不变。

## 验证

1. `go build ./...`
2. `go vet ./...`
3. `go test ./cmd/ ./internal/tool/ ./internal/registry/` 全绿
4. 手测：`sm add --help` 列出 7 个 flag；`sm rm --claude <skill>` 解析到 claude-only（用 dry-run 或现有测试覆盖）

## 非目标

- 不改 `registry` 的常量与 `specialDirs`（接受 catalog 字段值与常量字面一致）。
- 不动 first-class 6 agent 名单。
- MCP 标准化、Skill 质量门禁等 AI 适配方向属阶段二+，独立 spec。

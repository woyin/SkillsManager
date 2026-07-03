# MCP transport 可视化

**日期**：2026-07-03
**范围**：阶段二（AI 适配方向之二：MCP 标准化）

## 背景

MCP（Model Context Protocol）现为跨 agent 事实标准。一个 `.mcp.json` 可同时声明多个 server，每个 server 走 stdio（`command`）或 HTTP/SSE（`url`）transport。sm 当前 `sm list --mcp` 只显示 MCP 名称，用户无从分辨每个 MCP 含几个 server、各走什么 transport——而这直接影响启动方式与可调试性。

不深校验 server 语义（不同 agent 字段变体多，过严误伤），改为**只读展示 transport 类型**。

## 目标

`sm list --mcp` 每个 MCP 显示其 server 列表与各自 transport。

## 设计

### `internal/registry/mcp.go` 新增导出

```go
// ServerTransport 描述一个 MCP server 的 transport 摘要。
type ServerTransport struct {
    Server    string // server 名（mcpServers 的 key）
    Transport string // "stdio" | "http" | "sse" | "unknown"
    Detail    string // stdio: command 名；http/sse: url；unknown: ""
}

// MCPServerTransports 解析一个 MCP 定义文件，返回其全部 server 的 transport 摘要。
func MCPServerTransports(defPath string) ([]ServerTransport, error)
```

transport 推断规则（保守、只读）：
- 含 `command` 字段 → `stdio`，Detail = command 值
- 含 `url` 且 `type == "sse"` → `sse`，Detail = url
- 含 `url`（无 type 或 type=http）→ `http`，Detail = url
- 否则 → `unknown`

### `cmd/list.go` 集成

`sm list --mcp` 改为表格输出：`NAME\tSERVERS\tTRANSPORT`。单 server 时合并显示；多 server 时每 server 一行（MCP 名首行显示，续行缩进）。用 `GetMCPPath` 拿定义文件路径，调 `MCPServerTransports`。

解析失败（JSON 损坏等）不阻断：显示 `NAME\t(parse error)`。

### Web API

本期不改 web（`/api/registry` 已返回 MCP 详情，可在后续扩展加 transports 字段）。保持 CLI 优先。

## 行为契约

- `sm list --mcp` 输出含 transport 列。
- 无法解析的 MCP 不阻断列举，标记为 error。
- 不修改任何 MCP 文件。

## 验证

1. `go test ./internal/registry/` 含 `mcp_test.go` 新增 transport 推断用例（stdio/http/sse/unknown 各一）
2. `go test ./cmd/` 含 list --mcp 输出格式
3. 手测：构造含多 server 的 .mcp.json，看输出

## 非目标

- 不校验 server 字段语义（command 是否存在等留待深校验方向）。
- 不改 web API。
- 不改 add/install 流程。

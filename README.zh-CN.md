# SkillsManager (sm)

一个用于管理 AI 代理技能（Codex、Claude）和 MCP 服务器配置的 CLI 工具，支持跨项目管理。

## 设计哲学

### 一份原件，全部软链接

注册表持有原始文件。所有安装位置（`~/.codex/skills/`、`~/.claude/skills/`）都是指向注册表的软链接。这意味着：

- **无重复** — 磁盘占用极小
- **即时更新** — 更新注册表后，所有安装自动生效
- **轻松清理** — 删除注册表条目，所有软链接立即可见失效

### 预设配置即 Profile

Profile 将一组技能和 MCP 配置打包为一个场景（例如 "cloudflare 开发"、"前端"、"安全审计"）。项目以 Profile 为基础，在其上叠加自定义配置。

### 最小化全局，最大化本地

只有真正跨工具的技能才放入 `global/`。领域特定的技能通过 Profile 按项目安装。这让每个项目的 AI 环境保持专注和轻量。

### 三个特殊目录

| 目录 | 行为 |
|------|------|
| `global/` | 安装到 Codex 和 Claude |
| `codex-only/` | 仅安装到 Codex |
| `claude-only/` | 仅安装到 Claude |

其他所有目录均为用户自定义类别。类别目录中的技能默认安装到两个工具。

## 安装

### Homebrew (macOS/Linux)

```bash
brew tap woyin/tap
brew install sm
```

### Go

```bash
go install github.com/woyin/skills-manager@latest
```

### 从源码构建

```bash
git clone https://github.com/woyin/skills-manager.git
cd skills-manager
go build -o sm .
mv sm /usr/local/bin/
```

## 命令

### `sm add <source> [category]`

向注册表添加技能或 MCP。

```bash
# 从 GitHub 添加
sm add github.com/user/repo/path cloudflare

# 从本地路径添加，设为全局
sm add ./my-skill --global

# 添加 MCP 定义
sm add github.com/user/mcp-server --mcp
sm add ./cloudflare.mcp.json --mcp
```

**参数：**
- `--global` — 添加到 `global/` 目录
- `--codex` — 添加到 `codex-only/` 目录
- `--claude` — 添加到 `claude-only/` 目录
- `--mcp` — 作为 MCP 服务器定义添加。来源可以是 JSON 文件、目录，或包含 `.mcp.json`、`mcp.json`、其他带 `mcpServers` JSON 文件的 Git 仓库。Git MCP 来源会以仓库形式保存在 `registry/mcp/` 下，因此 `sm update` 可以继续刷新它们。

### `sm rm <name> [category]`

从注册表删除技能或 MCP。同时清理已安装位置的软链接。

```bash
sm rm my-skill
sm rm my-skill --global
sm rm cloudflare --mcp
```

### `sm install`

将技能和 MCP 安装到项目目录。

```bash
# 在项目目录内
cd ~/my-project
sm install --profile cloudflare

# 或指定目录
sm install --profile frontend --dir ~/my-project
```

读取 `.sm.json`（如存在）。在 `~/.codex/skills/` 和 `~/.claude/skills/` 中创建软链接。为 MCP 配置写入 `.mcp.json`。在 SQLite 数据库中记录安装详情。

如果已安装的技能软链接指向其他位置，`sm install` 会先警告并询问是否替换。
如果项目 `.mcp.json` 中已有同名 MCP server key，新定义会覆盖旧定义，并打印 warning。

### `sm update`

更新所有 Git 管理的注册表条目。

```bash
sm update
```

遍历 `registry/`，找到含 `.git` 的目录，对每个执行 `git pull --ff-only`。

### `sm check`

检查安装完整性。

```bash
sm check
sm check --fix  # 自动修复损坏的软链接
```

`sm check` 会报告损坏软链接、指向当前 registry 外部的软链接，以及目录已不存在的项目记录。

### `sm list [--skills|--mcp]`

列出所有注册表内容。

```bash
sm list
sm list --skills
sm list --mcp
```

### `sm web`

启动 Web 仪表板。

```bash
sm web           # 默认端口 3721
sm web -p 8080   # 自定义端口
```

仪表板会展示注册表类别、Git 来源条目的 source URL、最后更新时间、项目记录、可筛选/排序的安装历史，以及来自 `sm check` 的健康问题。

## 项目配置 (.sm.json)

项目在 `.sm.json` 中存储配置：

```json
{
  "profile": "cloudflare",
  "skills": ["extra-skill"],
  "mcp": ["extra-mcp"]
}
```

Profile 是基础配置，`skills` 和 `mcp` 数组是额外叠加的配置。

## 目录结构

```
SkillsManager/
├── registry/
│   ├── skills/
│   │   ├── global/          ← 特殊目录：所有工具
│   │   ├── codex-only/      ← 特殊目录：仅 Codex
│   │   ├── claude-only/     ← 特殊目录：仅 Claude
│   │   ├── cloudflare/      ← 用户自定义类别
│   │   └── ...
│   └── mcp/
│       ├── cloudflare.json
│       └── ...
├── profiles/
│   ├── cloudflare.json
│   └── ...
├── data/
│   └── sm.db                ← SQLite（本地，gitignore）
└── ...
```

## 贡献

提交修改前请运行：

```bash
go test ./...
go build -o sm .
```

不要把 registry 数据、profiles 和本地 SQLite 状态混入无关代码改动。CLI 行为、registry 规则、installer 流程和 Web API 变更都应添加聚焦测试。

## 发布

GitHub Actions 只会在推送 release tag 时构建 release 二进制：

```bash
git tag v0.1.0
git push origin v0.1.0
```

支持的 tag 模式是 `v*`、`release` 和 `release-*`。推荐使用 `v0.1.0` 这类版本 tag，因为发布历史更稳定。

## 许可证

MIT

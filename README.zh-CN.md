# SkillsManager (sm)

[![Go Reference](https://pkg.go.dev/badge/github.com/woyin/skills-manager.svg)](https://pkg.go.dev/github.com/woyin/skills-manager)
[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/tag/woyin/skills-manager?label=release&sort=semver)](https://github.com/woyin/skills-manager/releases)

一个用于管理 AI 代理技能（Codex、Claude、Gemini、OpenCode、Hermes、OpenClaw）和 MCP 服务器配置的 CLI 工具，支持跨项目管理。

## 目录

- [设计理念](#设计理念)
- [安装](#安装)
- [命令](#命令)
- [项目配置](#项目配置-smjson)
- [目录结构](#目录结构)
- [Web 仪表盘](#web-仪表盘)
- [支持的 AI 助手](#支持的-ai-助手)
- [aivo 集成](#aivo-集成)
- [架构](#架构)
- [贡献](#贡献)
- [发布](#发布)
- [许可证](#许可证)

## 设计理念

### 一份原件，全部符号链接

注册表保存原始文件。所有安装位置（`~/.codex/skills/`、`~/.claude/skills/`、`~/.gemini/skills/` 等）都是指向注册表的符号链接。这意味着：

- **无重复** — 磁盘占用最小
- **即时更新** — 更新注册表，所有安装位置立即反映
- **轻松清理** — 删除注册表条目，所有符号链接立即可见地断开

### 配置文件即预设

配置文件（Profile）为特定场景捆绑一组技能和 MCP 配置（例如"cloudflare 开发"、"前端"、"安全审计"）。项目引用配置文件作为基础，然后在其上叠加临时添加。

### 最小全局，最大本地

只有真正跨工具的技能才放入 `global/`。特定领域的技能通过配置文件按项目安装。这使每个项目的 AI 环境保持专注和轻量。

### 特殊目录

| 目录 | 行为 |
|------|------|
| `global/` | 安装到所有工具 |
| `codex-only/` | 仅安装到 Codex |
| `claude-only/` | 仅安装到 Claude |
| `gemini-only/` | 仅安装到 Gemini |
| `opencode-only/` | 仅安装到 OpenCode |
| `hermes-only/` | 仅安装到 Hermes |
| `openclaw-only/` | 仅安装到 OpenClaw |

所有其他目录是用户自定义类别。类别目录中的技能默认安装到所有工具。

## 安装

### Homebrew (macOS/Linux)

formula 内嵌于本仓库 `Formula/` 目录,每次发版由 CI 自动同步版本号与各平台 SHA-256(见 `.github/scripts/sync_formula.py`)。先用自定义 URL 指向本仓库,再安装:

```bash
brew tap woyin/skills-manager https://github.com/woyin/SkillsManager
brew install woyin/skills-manager/sm
```

> Homebrew 6.0+ 首次安装第三方 tap 会提示信任(trust),按提示确认即可。后续升级:`brew upgrade sm`。

### Go

```bash
go install github.com/woyin/skills-manager@latest
```

### 从源码构建

```bash
git clone https://github.com/woyin/skills-manager.git
cd skills-manager
go build -o sm .
# 移动到 PATH
mv sm /usr/local/bin/
```

## 命令

### `sm add <source> [category]`

将技能或 MCP 添加到注册表。

对于单技能来源，`add` 会优先使用 `SKILL.md` frontmatter 中的 `name:` 字段；如果不存在，再回退到来源路径最后一段。

```bash
# 从 GitHub 添加
sm add github.com/user/repo/path cloudflare

# 从本地路径添加，全局
sm add ./my-skill --global

# 添加 MCP 定义
sm add github.com/user/mcp-server --mcp
sm add ./cloudflare.mcp.json --mcp
```

**选项：**
- `--global` — 添加到 `global/` 目录（所有工具）
- `--codex` — 添加到 `codex-only/` 目录
- `--claude` — 添加到 `claude-only/` 目录
- `--gemini` — 添加到 `gemini-only/` 目录
- `--opencode` — 添加到 `opencode-only/` 目录
- `--hermes` — 添加到 `hermes-only/` 目录
- `--openclaw` — 添加到 `openclaw-only/` 目录
- `--mcp` — 作为 MCP 服务器定义处理

### `sm rm <name> [category]`

从注册表中删除技能或 MCP。同时清理已安装位置的符号链接。

```bash
sm rm my-skill
sm rm my-skill --global
sm rm cloudflare --mcp
```

**选项：**
- `--global` — 从 `global/` 目录删除
- `--codex` — 从 `codex-only/` 目录删除
- `--claude` — 从 `claude-only/` 目录删除
- `--gemini` — 从 `gemini-only/` 目录删除
- `--opencode` — 从 `opencode-only/` 目录删除
- `--hermes` — 从 `hermes-only/` 目录删除
- `--openclaw` — 从 `openclaw-only/` 目录删除
- `--mcp` — 删除 MCP 服务器定义

### `sm install`

将技能和 MCP 安装到项目目录。

```bash
# 在项目目录中
cd ~/my-project
sm install --profile cloudflare

# 或指定目录
sm install --profile frontend --dir ~/my-project
```

读取 `.sm.json`（如果存在）。在工具特定的技能目录中创建符号链接。为 MCP 配置写入 `.mcp.json`。在 SQLite 数据库中记录安装。

**选项：**
- `--profile` — 要安装的配置文件名称
- `--dir` — 项目目录（默认：当前目录）

### `sm uninstall`

从所有 AI 工具技能目录中移除 SkillsManager 创建的所有符号链接。符号链接是全局的（跨项目共享），因此会移除所有指向注册表的符号链接。不会删除注册表条目或配置文件。

```bash
sm uninstall
```

### `sm status`

显示当前项目中已安装的内容。展示 `.sm.json` 配置和每个工具技能目录中安装的所有符号链接。

```bash
# 在项目目录中
sm status

# 或指定目录
sm status --dir ~/my-project
```

**选项：**
- `--dir` — 项目目录（默认：当前目录）

### `sm init`

初始化新项目，创建 `.sm.json` 配置文件。

```bash
cd ~/my-project
sm init

# 指定配置文件
sm init --profile cloudflare
```

**选项：**
- `--profile` — 用作基础的配置文件名称

### `sm update`

更新所有 git 管理的注册表条目。

```bash
sm update
```

遍历 `registry/`，找到包含 `.git` 的目录，对每个执行 `git pull --ff-only`。

### `sm check`

验证安装完整性。

```bash
sm check
sm check --fix  # 自动修复损坏的符号链接
```

扫描所有工具技能目录中损坏或孤立的符号链接，并检查数据库中的项目记录。

**选项：**
- `--fix` — 自动修复损坏的符号链接和过期记录

### `sm doctor`

检查环境和依赖。

```bash
sm doctor
```

验证所有 AI 工具 CLI 二进制文件（Git、Claude、Codex、Gemini、OpenCode、Hermes、OpenClaw、Go）、目录、数据库和环境变量。同时检测可选的 [aivo](https://github.com/yuanchuan/aivo) 集成。

### `sm list [--skills|--mcp]`

列出所有注册表内容。

```bash
sm list
sm list --skills
sm list --mcp
```

**选项：**
- `--skills` — 仅列出技能
- `--mcp` — 仅列出 MCP

### `sm profile`

管理技能配置文件。配置文件为特定场景捆绑技能和 MCP 配置。

```bash
# 列出可用配置文件
sm profile list

# 显示配置文件内容
sm profile show cloudflare

# 创建新配置文件
sm profile create my-profile --skills "skill-a,skill-b" --mcp "mcp-server"

# 删除配置文件
sm profile delete my-profile
```

**选项（create）：**
- `--skills` — 逗号分隔的技能列表
- `--mcp` — 逗号分隔的 MCP 服务器列表

### `sm backup` / `sm restore`

创建和恢复配置备份。

```bash
# 创建备份
sm backup
sm backup --name "pre-upgrade"
sm backup --rotate 5  # 仅保留最近 5 个

# 从备份恢复
sm restore backup-20260607-150405
sm restore --latest
```

备份包括数据库、注册表和配置文件。恢复时自动创建恢复前备份以确保安全。

**选项（backup）：**
- `--name` — 自定义备份名称
- `--rotate` — 仅保留最近 N 个备份

**选项（restore）：**
- `--latest` — 从最近的备份恢复

### `sm completion [bash|zsh|fish|powershell]`

生成 shell 补全脚本。

```bash
# Bash
source <(sm completion bash)

# Zsh
source <(sm completion zsh)

# Fish
sm completion fish | source

# PowerShell
sm completion powershell | Out-String | Invoke-Expression
```

### `sm prompt`

管理不同 AI 编程助手的提示集。提示集是特定工具的提示文件（如 `CLAUDE.md`、`AGENTS.md`、`GEMINI.md`）的集合，以 JSON 格式存储在注册表中。

```bash
# 列出可用提示集
sm prompt list

# 显示提示集内容
sm prompt show my-prompts

# 将提示集应用到项目
sm prompt apply my-prompts --dir ~/my-project

# 仅应用特定的提示文件
sm prompt apply my-prompts --tools CLAUDE.md,AGENTS.md

# 从当前项目创建提示集
sm prompt create my-prompts --dir ~/my-project

# 仅从特定文件创建
sm prompt create my-prompts --files CLAUDE.md,AGENTS.md

# 删除提示集
sm prompt delete my-prompts
```

**选项（apply）：**
- `--dir` — 项目目录（默认：当前目录）
- `--tools` — 逗号分隔的要应用的提示文件列表（默认：集合中的全部）

**选项（create）：**
- `--dir` — 项目目录（默认：当前目录）
- `--files` — 逗号分隔的要包含的提示文件列表（默认：`CLAUDE.md,AGENTS.md,GEMINI.md`）

### `sm export` / `sm import`

导出和导入配置。

```bash
# 导出所有内容到文件
sm export --output config.json

# 仅导出注册表、配置文件和提示集
sm export --include registry,profiles,prompts --output config.json

# 导出到标准输出
sm export

# 从文件导入（合并模式，默认）
sm import config.json

# 从标准输入导入
sm import -

# 使用替换模式导入
sm import config.json --replace

# 试运行（预览更改）
sm import config.json --dry-run
```

**选项（export）：**
- `-o, --output` — 输出文件路径（默认：标准输出）
- `--include` — 逗号分隔的导出项：`registry`、`profiles`、`prompts`、`projects`（默认：全部）

**选项（import）：**
- `--merge` — 与现有数据合并（默认）
- `--replace` — 替换现有数据（先清除所有内容）
- `--dry-run` — 显示将要导入的内容，不实际执行
- `--merge` 和 `--replace` 互斥

### `sm web`

启动 Web 仪表盘。

```bash
sm web           # 默认端口 3721
sm web -p 8080   # 自定义端口
```

**选项：**
- `-p, --port` — 监听端口（默认：3721）

### 全局选项

所有命令都支持这些持久化路径选项，默认指向用户级存储目录：

- `--registry` — 注册表目录路径（默认：`~/.sm/registry`）
- `--data` — 数据目录路径（默认：`~/.sm/data`）
- `--profiles` — 配置文件目录路径（默认：`~/.sm/profiles`）
- `-v, --version` — 打印版本并退出

## 项目配置 (.sm.json)

项目在 `.sm.json` 中存储配置：

```json
{
  "profile": "cloudflare",
  "skills": ["extra-skill"],
  "mcp": ["extra-mcp"]
}
```

配置文件是基础。`skills` 和 `mcp` 数组是临时添加。

## 目录结构

```
SkillsManager/
├── cmd/                     ← CLI 命令（Cobra）
│   ├── root.go
│   ├── add.go
│   ├── rm.go
│   ├── install.go
│   ├── uninstall.go
│   ├── status.go
│   ├── init.go
│   ├── update.go
│   ├── check.go
│   ├── doctor.go
│   ├── list.go
│   ├── profile.go
│   ├── prompt.go
│   ├── backup.go
│   ├── restore.go
│   ├── export.go
│   ├── import.go
│   ├── web.go
│   └── completion.go
├── internal/                ← 核心包
│   ├── registry/            ← 技能和 MCP 注册表管理
│   ├── installer/           ← 基于符号链接的安装流程
│   ├── profile/             ← 配置文件加载器和管理器
│   ├── project/             ← .sm.json 配置处理器
│   ├── prompt/              ← 提示集管理器
│   ├── db/                  ← SQLite 数据库（项目和安装记录）
│   ├── backup/              ← 备份和恢复逻辑
│   ├── symlink/             ← 符号链接创建/验证/清理
│   ├── tool/                ← AI 工具定义和检测
│   └── aivo/                ← 可选 aivo 集成
├── web/                     ← Web 仪表盘
│   ├── handler.go           ← REST API 处理器
│   └── static/              ← 嵌入式前端（HTML/CSS/JS）
├── registry/                ← 技能和 MCP 数据
│   ├── skills/
│   │   ├── global/          ← 特殊：所有工具
│   │   ├── codex-only/      ← 特殊：仅 Codex
│   │   ├── claude-only/     ← 特殊：仅 Claude
│   │   ├── gemini-only/     ← 特殊：仅 Gemini
│   │   ├── opencode-only/   ← 特殊：仅 OpenCode
│   │   ├── hermes-only/     ← 特殊：仅 Hermes
│   │   ├── openclaw-only/   ← 特殊：仅 OpenClaw
│   │   └── ...
│   └── mcp/
│       └── ...
├── profiles/                ← 配置文件定义
├── data/                    ← 本地状态（gitignore）
│   ├── sm.db                ← SQLite 数据库
│   └── backups/             ← 配置备份
├── docs/                    ← 设计规范和计划
├── .github/workflows/       ← CI/CD（Go 测试 + 多平台发布）
├── go.mod
└── LICENSE
```

提示集存储在 `registry/prompts/` 下。

### `sm --version`

显示当前版本。

```bash
sm --version
```

## Web 仪表盘

`sm web` 命令启动一个嵌入式 HTTP 服务器，提供浏览器端的仪表盘，用于浏览和监控技能、MCP 服务器、项目和安装历史。

### 标签页

| 标签 | 描述 |
|------|------|
| **概览** | 汇总统计（技能、MCP 服务器、项目、健康状态）、aivo 集成状态、最近安装 |
| **注册表** | 所有技能按类别分组（带特殊目录标记）、MCP 服务器详情 |
| **项目** | 已注册项目，包含配置文件、额外技能和最近安装时间 |
| **历史** | 完整安装历史，支持过滤和排序（最新/最旧/项目名 A-Z） |

### REST API

仪表盘由以下端点提供数据：

| 端点 | 描述 |
|------|------|
| `GET /api/registry` | 列出所有技能和 MCP 服务器及详情 |
| `GET /api/projects` | 列出所有已注册项目 |
| `GET /api/history` | 列出所有安装记录 |
| `GET /api/check` | 运行健康检查（损坏/孤立符号链接、缺失项目） |
| `GET /api/tools` | 检测已安装的 AI 工具和技能目录 |
| `GET /api/aivo` | aivo 集成状态、活跃密钥/模型、Token 使用量、密钥健康状态 |

所有端点返回 JSON。前端在构建时通过 `//go:embed` 嵌入到二进制文件中。

## 支持的 AI 助手

| 助手 | 技能目录 | 配置文件 |
|------|----------|----------|
| Claude | `~/.claude/skills/` | `CLAUDE.md` |
| Codex | `~/.codex/skills/` | `AGENTS.md` |
| Gemini | `~/.gemini/skills/` | `GEMINI.md` |
| OpenCode | `~/.opencode/skills/` | `OPENCODE.md` |
| Hermes | `~/.hermes/skills/` | `HERMES.md` |
| OpenClaw | `~/.openclaw/skills/` | `OPENCLAW.md` |

## aivo 集成

SkillsManager 可选集成 [aivo](https://github.com/yuanchuan/aivo)，一个 AI 工具启动器和 API 密钥管理器。

- `sm doctor` 检测 aivo 安装状态，报告版本、密钥数量、活跃密钥和不健康的密钥
- `sm status` 在 aivo 已安装时显示活跃密钥和使用统计
- Web 仪表盘（`sm web`）在概览标签页中显示 aivo 统计信息

aivo 是**可选的** —— 所有 sm 命令在没有 aivo 的情况下也能正常工作。安装方式：

```bash
brew install yuanchuan/tap/aivo
```

## 架构

SkillsManager 使用 Go 构建，技术栈如下：

| 组件 | 技术 |
|------|------|
| CLI 框架 | [Cobra](https://github.com/spf13/cobra) |
| 数据库 | [SQLite](https://gitlab.com/cznic/sqlite)（纯 Go，无需 CGO） |
| Web 前端 | 原生 HTML/CSS/JS，通过 `//go:embed` 嵌入 |
| 构建/发布 | GitHub Actions（6 个平台：linux/darwin/windows × amd64/arm64） |
| 分发 | Homebrew tap + Go install + GitHub Releases |

### 关键设计决策

- **基于符号链接的安装** — 注册表保存原件；工具目录通过符号链接实现零重复和即时更新
- **嵌入式 Web UI** — 单一二进制文件，无外部依赖；静态文件在编译时打包
- **纯 Go SQLite** — 无需 CGO，轻松实现交叉编译和静态二进制
- **配置文件系统** — 声明式项目配置；配置文件提供可复用预设

## 贡献

提交更改前：

```bash
go test ./...
go build -o sm .
```

保持注册表数据、配置文件和本地 SQLite 状态与无关代码更改分离。为 CLI 行为、注册表规则、安装流程和 Web API 更改添加有针对性的测试。

## 发布

GitHub Actions 仅在推送发布标签时构建发布二进制文件：

```bash
git tag v0.1.0
git push origin v0.1.0
```

支持的标签模式为 `v*`、`release` 和 `release-*`。推荐使用版本标签如 `v0.1.0`，因为它们创建稳定的发布历史。每个发布包含 Linux（amd64/arm64）、macOS（amd64/arm64）和 Windows（amd64/arm64）的二进制文件及 SHA-256 校验和。

## 许可证

[MIT](LICENSE) © 2026 woyin

# SkillsManager (sm)

[![Go Reference](https://pkg.go.dev/badge/github.com/woyin/skills-manager.svg)](https://pkg.go.dev/github.com/woyin/skills-manager)
[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/tag/woyin/skills-manager?label=release&sort=semver)](https://github.com/woyin/skills-manager/releases)

一个用于管理 AI 代理技能（Codex、Claude、Gemini、OpenCode、Hermes、OpenClaw）和 MCP 服务器配置的 CLI 工具，支持跨项目管理 —— 以**跨项目个人 Skill Registry** 为核心：原件由你拥有、按名称复用、通过 Profile 批量部署、一条 `sm update` 统一刷新。

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

SkillsManager 是一个**跨项目个人 Skill Registry**：一份属于你的原件库，
位于 `~/.sm/registry`，按名称在你的所有项目与代理间复用，通过 Profile 批量
部署，并用一条命令整体刷新。

- **Registry 优先** — `sm add` 注册一次原件；`sm install <name>` 按名称部署；
  `sm update` 一条命令刷新所有可更新原件。
- **Profile 批量部署** — Profile 引用 Registry 中已存在的技能与 MCP 定义；
  `sm install --profile` 原子化部署整套内容。
- **一次更新** — Link Install 指向 Registry 原件，刷新 Registry 即同步更新
  所有项目与代理目录。

### 一份原件，全部符号链接

注册表保存原始文件。所有安装位置（`~/.codex/skills/`、`~/.claude/skills/`、`~/.gemini/skills/` 等）都是指向注册表的符号链接。这意味着：

- **无重复** — 磁盘占用最小
- **即时更新** — 更新注册表，所有安装位置立即反映
- **轻松清理** — 删除注册表条目，所有符号链接立即可见地断开

### 配置文件即预设

配置文件（Profile）为特定场景捆绑一组技能和 MCP 配置（例如"cloudflare 开发"、"前端"、"安全审计"）。项目引用配置文件作为基础，然后在其上叠加临时添加。

### 默认全局，需要时收窄

新注册默认进入 `global/`（所有工具），常见场景无需任何 flag。需要时用
`--codex`、`--claude` 等把技能收窄到单个代理，或通过 Profile 把特定领域的
技能按项目部署，让每个项目的 AI 环境保持专注和轻量。

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

## 从旧版本升级

Registry-first 迁移说明见 [docs/upgrades/registry-first-migration.md](docs/upgrades/registry-first-migration.md)：
旧 `.sm-origin.json` 兼容、ref kind 推断、跨 category 同名清理，以及命令默认值
变化（`sm install <name>`、`sm list`、`sm update`、`sm rm`）。

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

将技能或 MCP 注册进你的**个人跨项目 Registry**。仅注册 —— 不会安装到任何
agent 目录。部署用 `sm install <name>`（Registry Install）。

技能的全局唯一身份来自 `SKILL.md` frontmatter 的 `name:` 字段（1–64 个小写
字母、数字或单连字符）。`add` 在写入前校验 name 与 description，绝不从来源
路径臆造名称。本地目录和单个 `SKILL.md` 文件会作为独立 Snapshot 复制入库
（重新 `sm add` 才刷新）。

```bash
# 从 GitHub 注册（默认 category：global）
sm add github.com/user/repo/path

# 从 bundle 注册指定技能
sm add owner/repo@my-skill

# 从本地路径或单个 SKILL.md 文件注册
sm add ./my-skill
sm add ./SKILL.md

# 注册多技能来源中的全部技能
sm add owner/repo --all

# 注册 MCP 定义
sm add github.com/user/mcp-server --mcp
sm add ./cloudflare.mcp.json --mcp
```

**选项：**
- `--all` — 注册多技能来源中发现的全部技能
- `--force` — 同名已从不同来源注册时强制替换
- `--ref <branch|tag|commit>` — 指定 Git ref 注册（解析并记录；tag/commit 保持 pinned）
- `--global` — 注册到 `global/`（所有工具；**默认**）
- `--codex` — 注册到 `codex-only/` 目录
- `--claude` — 注册到 `claude-only/` 目录
- `--gemini` — 注册到 `gemini-only/` 目录
- `--opencode` — 注册到 `opencode-only/` 目录
- `--hermes` — 注册到 `hermes-only/` 目录
- `--openclaw` — 注册到 `openclaw-only/` 目录
- `--mcp` — 作为 MCP 服务器定义处理
- `-l, --list` — 仅列出来源中可用技能，不注册
- `-s, --skill <names>` — 按名称注册指定技能（`*` 或 `--all` 表示全部）
- `--copy` — 兼容别名（注册总是把原件复制进 Registry）

> 同名同来源重新注册 = 刷新；同名不同来源 = 失败，需 `--force`（所有 Link
> Install 都会受影响）。

### `sm rm <name> [category]`

删除 Registry 原件（ADR 0017）。若任何已知项目或全局安装仍引用该原件，
`rm` 会拒绝并列出引用；用 `--force` 先清理所有已知安装与 lock entries，再
删除原件。不可访问的历史项目会被明确报告。

只删除 Installed Skill（保留 Registry 原件）请用 `sm uninstall`。

```bash
sm rm my-skill
sm rm my-skill --force
sm rm cloudflare --mcp
```

**选项：**
- `--force` — 强制删除：先清理所有已知 Link Installs 与 lock entries，再删除 Registry 原件
- `-a, --agent <agents>` — 指定目标代理
- `-s, --skill <names>` — 指定要删除的技能（`*` 表示全部）
- `--project` — 仅清理项目级安装（`./<agent>/skills`）
- `--global` — 从 `global/` 目录删除
- `--codex` — 从 `codex-only/` 目录删除
- `--claude` — 从 `claude-only/` 目录删除
- `--gemini` — 从 `gemini-only/` 目录删除
- `--opencode` — 从 `opencode-only/` 目录删除
- `--hermes` — 从 `hermes-only/` 目录删除
- `--openclaw` — 从 `openclaw-only/` 目录删除
- `--mcp` — 删除 MCP 服务器定义

### `sm install [source]`

三种模式：

- **Registry Install**（`sm install <name>`）：按名称从本地 Registry 安装
  已注册技能。绝不联网；名称不在 Registry 时报错并提示 `sm add`，不会猜测。
- **Direct Install**（`sm install <source>`）：从仓库 / URL / 本地路径发现
  技能，写入 Registry 原件，再 symlink 到 agent 目录。默认：**项目级范围**、
  **检测到的代理**；可用 `--global` / `--agent` 覆盖。
- **Profile 模式**（无 source）：按 `.sm.json` / `--profile` 把整套技能与
  MCP 原子化安装到项目目录。

```bash
# Registry Install —— 按名称部署已注册技能（不联网）
sm install my-skill
sm install my-skill --global -a claude

# Direct Install —— 典型：项目级 + 检测到的代理
sm install owner/repo

# Direct Install —— 指定技能 / 代理 / 全局
sm install owner/repo --global -a claude

# Direct Install —— 全部技能装到全部代理
sm install owner/repo --all

# 列出来源中可用技能
sm install owner/repo --list

# Profile 模式
sm install --profile cloudflare
sm install --profile frontend --dir ~/my-project
```

**Registry Install 注意：**
- 裸名称（如 `sm install foo`）一律按 Registry 名称解析。
- 本地目录恰好与技能同名时，请显式写来源：`sm install ./foo`。

**Direct Install 选项：**
- `-a, --agent` — 目标代理（默认：本机已检测；`*` = 全部）
- `-g, --global` — 全局范围（默认项目级）
- `-s, --skill` — 指定技能
- `--all` — 全装
- `-y, --yes` — 跳过确认
- `-l, --list` — 仅列出来源中可用技能，不安装
- `--copy` — 拷贝到 agent 目录（update 不会自动同步这些拷贝）
- `--from-lock` — 从 `skills-lock.json` 恢复项目技能（可复现安装）

**Profile 模式选项：**
- `--profile` / `--dir` — Profile 名称与项目目录（默认：当前目录）

### `sm uninstall`

从 agent 技能目录移除 sm 安装的符号链接。**不删除 registry 原件**（删原件用 `sm rm`）。

默认范围：**项目 + 全局**。可用 `--project` / `--global` 收窄，用 `--agent` / `--skill` 过滤。大范围卸载用 `--all -y`。

```bash
# 移除所有全局 SkillsManager 符号链接
sm uninstall

# 从所有 agent 移除某个全局 skill
sm uninstall --skill my-skill

# 从某个 agent 移除所有全局 skill
sm uninstall --agent codex

# 从某个 agent 移除某个 skill
sm uninstall --agent codex --skill my-skill

# 只移除当前项目 skill 符号链接
sm uninstall --project

# 只移除另一个项目目录的 skill 符号链接
sm uninstall --project --dir ~/my-project

# 明确执行大范围卸载
sm uninstall --all -y
```

**选项：**
- `-a, --agent <agents>` — 选择指定 agent（使用 `*` 表示全部）
- `-s, --skill <names>` — 选择指定 skill（使用 `*` 表示全部）
- `--project` — 处理项目技能目录，而不是全局 agent 目录
- `--dir` — `--project` 使用的项目目录（默认：当前目录）
- `--all` — 移除所选作用域内所有 SkillsManager 符号链接
- `-y, --yes` — 确认破坏性 `--all` 卸载

### `sm status`

项目健康一页纸：profile、项目已装技能、全局摘要、断链/orphan 问题与下一步建议。

```bash
sm status
sm status --dir ~/my-project
```


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

### `sm update [skills...]`

默认刷新整个个人 Registry 中所有可更新原件（ADR 0008），所有 Link Install
无需逐项目访问即可观察到刷新后的内容。

```bash
# 更新整个 Registry（默认）
sm update

# 更新指定的 Registry 技能
sm update frontend-design web-design-guidelines

# 只更新当前项目引用的 Registry 技能
sm update --project [--dir PATH]

# 只更新全局 Agent 安装引用的 Registry 技能
sm update --global

# 原地刷新项目 Copy Install（不改 Registry）
sm update --in-place

# 非交互
sm update -y
```

tracking Git 技能（默认分支或指定分支）会更新；pinned tag/commit 技能与本地
Snapshot 技能是健康跳过；Orphan 技能（provenance 损坏）计为错误。各 Source
独立更新 —— 失败的 Source 保留其旧的有效原件，其它 Source 继续更新；任一
失败都会让退出码非零。

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

验证所有 AI 工具 CLI 二进制文件（Git、Claude、Codex、Gemini、OpenCode、Hermes、OpenClaw、Go）、目录、数据库、环境变量，以及 Registry 完整性（跨 category 同名、orphan 技能、损坏的 provenance 元数据）。同时检测可选的 [aivo](https://github.com/yuanchuan/aivo) 集成。

### `sm list`

默认显示**个人 Registry 清单**（你的可复用原件）。用 `--installed` 列出
Installed Skills（agent 目录里可加载的技能）。

```bash
# Registry 清单（默认）
sm list

# 已安装技能
sm list --installed

# 仅项目级安装
sm list --installed --project

# 仅全局安装
sm list --installed --global

# 按代理过滤
sm list --installed -a claude

# Registry MCP 定义
sm list --mcp
```

**选项：**
- `--installed` — 列出 Installed Skills（agent 目录）而非 Registry 清单
- `--project` / `-g, --global` — 已安装范围
- `-a, --agent` — 过滤代理
- `--dir <path>` — 已安装清单的项目根目录
- `--json` — 输出为 JSON
- `--mcp` — 仅 MCP（registry 视图）
- `--skills` — 仅技能
- `--registry` — 默认 Registry 视图的弃用别名

### `sm profile`

管理技能配置文件。Profile 引用 **Registry 中已存在**的技能与 MCP 定义，为
特定场景（如"cloudflare 开发"、"前端"、"安全审计"）捆绑成组。
`profile create`/`update` 保存前校验每个引用 —— 引用未知技能或 MCP 会失败
且不改写旧 Profile。`sm install --profile` 原子化部署整套内容（无部分安装）。

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

- **Registry 优先** — Registry 是用户拥有的跨项目原件库；安装是按名称部署，而非复制
- **Provenance 元数据** — 每个已注册技能记录其来源（`.sm-origin.json`）：source kind、ref kind、解析后的 commit —— 让 `sm update` 知道该刷新什么、该保持 pinned
- **基于符号链接的安装** — 注册表保存原件；工具目录通过符号链接实现零重复和即时更新
- **原子操作** — Profile 安装在写入前完整预检、失败回滚；`sm rm` 在仍有引用时拒绝删除
- **嵌入式 Web UI** — 单一二进制文件，无外部依赖；静态文件在编译时打包
- **纯 Go SQLite** — 无需 CGO，轻松实现交叉编译和静态二进制
- **配置文件系统** — 声明式项目配置；配置文件引用 Registry 中已存在的技能并提供可复用预设

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

`sm update` 会拒绝更新含未提交修改的仓库。对于更新前有效的注册技能，拉取后会重新校验必要 frontmatter；若新版破坏技能有效性，则自动恢复旧 commit。本地修改不会被丢弃。

使用 Git 分支、标签或 commit 创建版本快照；跨机器可复现时应使用完整 commit hash：

```bash
sm install github.com/user/repo --ref v1.2.0 --all
sm install github.com/user/repo --ref 0123456789abcdef --agent codex --skill my-skill
```

离线安装按来源和 `--ref` 精确命中缓存，不会发起网络克隆：

```bash
sm install github.com/user/repo --ref <完整-commit-hash> --offline --all
```

每份缓存记录来源、请求 ref、解析后 commit 和创建时间。即使 Git remote 配置发生变化，`sm cache` 仍可准确追溯来源。

固定来源使用隔离缓存和 detached HEAD。`sm update` 会将其报告为 `pinned` 并保持不变；需要升级时，用新 `--ref` 重新安装。

远程来源安装会在 `~/.sm/data/sources/` 保留一份持久克隆。软链接不会在 `sm` 退出后失效，重复安装复用克隆，`sm update` 会刷新 Registry 原件（并因此刷新缓存来源）。Well-Known 来源按需拉取；项目安装把端点记录在 `skills-lock.json`，`sm update` 可重新拉取刷新。若用了 `--copy`，update 只更新 Registry，agent 目录里的拷贝不会被改写；用 `sm update --in-place` 原地刷新 Copy Installs。

### `sm cache`

查看持久远程来源缓存，包括来源 URL、commit、跟踪/固定模式、引用数和磁盘占用：

```bash
sm cache
sm cache --prune -y
```

`--prune` 只删除全局代理目录与 SkillsManager 已记录项目中均无链接引用的缓存。由于未记录项目中的链接无法保护缓存，删除必须显式确认。

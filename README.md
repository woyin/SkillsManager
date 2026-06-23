# SkillsManager (sm)

[![Go Reference](https://pkg.go.dev/badge/github.com/woyin/skills-manager.svg)](https://pkg.go.dev/github.com/woyin/skills-manager)
[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/tag/woyin/skills-manager?label=release&sort=semver)](https://github.com/woyin/skills-manager/releases)

A CLI tool for managing AI agent skills (Codex, Claude, Gemini, OpenCode, Hermes, OpenClaw) and MCP server configurations across multiple projects.

## Table of Contents

- [Design Philosophy](#design-philosophy)
- [Installation](#installation)
- [Commands](#commands)
- [Project Config](#project-config-smjson)
- [Directory Structure](#directory-structure)
- [Web Dashboard](#web-dashboard)
- [Supported AI Assistants](#supported-ai-assistants)
- [aivo Integration](#aivo-integration)
- [Architecture](#architecture)
- [Contributing](#contributing)
- [Release](#release)
- [License](#license)

> **New:** `sm` now supports 67+ AI coding agents, GitHub/GitLab source shorthand, skill discovery, and the `use`/`find` commands. See [Supported AI Assistants](#supported-ai-assistants) for the full list.

## Design Philosophy

### One Original, All Symlinks

The registry holds the original files. All installed locations (`~/.codex/skills/`, `~/.claude/skills/`, `~/.gemini/skills/`, etc.) are symlinks pointing back to the registry. This means:

- **No duplication** — disk space is minimal
- **Instant updates** — update the registry, all installations reflect the change
- **Easy cleanup** — remove the registry entry, all symlinks break visibly

### Profiles as Presets

A profile bundles a set of skills and MCP configurations for a scenario (e.g., "cloudflare development", "frontend", "security audit"). Projects reference a profile as a base, then layer ad-hoc additions on top.

### Minimal Global, Maximal Local

Only truly cross-tool skills go into `global/`. Domain-specific skills are installed per-project via profiles. This keeps each project's AI environment focused and lightweight.

### Special Directories

| Directory | Behavior |
|-----------|----------|
| `global/` | Installs to all tools |
| `codex-only/` | Installs to Codex only |
| `claude-only/` | Installs to Claude only |
| `gemini-only/` | Installs to Gemini only |
| `opencode-only/` | Installs to OpenCode only |
| `hermes-only/` | Installs to Hermes only |
| `openclaw-only/` | Installs to OpenClaw only |

All other directories are user-defined categories. Skills in category directories install to all tools by default.

## Installation

### Homebrew (macOS/Linux)

formula 内嵌于本仓库 `Formula/` 目录,每次发版由 CI 自动同步版本号与各平台 SHA-256(见 `.github/scripts/sync_formula.py`):

```bash
brew install woyin/skills-manager/sm
```

> Homebrew 6.0+ 首次安装第三方 tap 会提示信任(trust),按提示确认即可。后续升级:`brew upgrade sm`。

### 一键脚本 (curl | bash)

无需 Homebrew,从 `latest` release 拉取与当前平台匹配的预编译二进制,解压到 `~/.local/bin`(可用 `BIN_DIR` 覆盖):

```bash
curl -fsSL https://raw.githubusercontent.com/woyin/SkillsManager/latest/install.sh | bash
```

> `latest` 标签由 CI 在每次发版后自动移动到最新提交(见 [.github/workflows/release.yml](.github/workflows/release.yml))。

### Go

```bash
go install github.com/woyin/skills-manager@latest
```

### Build from source

```bash
git clone https://github.com/woyin/skills-manager.git
cd skills-manager
go build -o sm .
# Move to PATH
mv sm /usr/local/bin/
```

## Commands

### `sm add <source> [category]`

Add a skill or MCP to the registry.

```bash
# Add from GitHub
sm add github.com/user/repo/path cloudflare

# Add from local path, globally
sm add ./my-skill --global

# Add MCP definition
sm add github.com/user/mcp-server --mcp
sm add ./cloudflare.mcp.json --mcp
```

**Flags:**
- `--global` — Add to `global/` directory (all tools)
- `--codex` — Add to `codex-only/` directory
- `--claude` — Add to `claude-only/` directory
- `--gemini` — Add to `gemini-only/` directory
- `--opencode` — Add to `opencode-only/` directory
- `--hermes` — Add to `hermes-only/` directory
- `--openclaw` — Add to `openclaw-only/` directory
- `--mcp` — Treat as MCP server definition

### `sm rm <name> [category]`

Remove a skill or MCP from the registry. Also cleans up symlinks in installed locations.

```bash
sm rm my-skill
sm rm my-skill --global
sm rm cloudflare --mcp
```

**Flags:**
- `--global` — Remove from `global/` directory
- `--codex` — Remove from `codex-only/` directory
- `--claude` — Remove from `claude-only/` directory
- `--gemini` — Remove from `gemini-only/` directory
- `--opencode` — Remove from `opencode-only/` directory
- `--hermes` — Remove from `hermes-only/` directory
- `--openclaw` — Remove from `openclaw-only/` directory
- `--mcp` — Remove MCP server definition

### `sm install`

Install skills and MCP into a project directory.

```bash
# In project directory
cd ~/my-project
sm install --profile cloudflare

# Or specify directory
sm install --profile frontend --dir ~/my-project
```

Reads `.sm.json` if present. Creates symlinks in tool-specific skills directories. Writes `.mcp.json` for MCP configurations. Records installation in SQLite database.

**Flags:**
- `--profile` — Profile name to install
- `--dir` — Project directory (default: current dir)

### `sm uninstall`

Remove all SkillsManager symlinks from all AI tool skill directories. Symlinks are global (shared across projects), so this removes every symlink pointing into the registry. Does not remove registry entries or profiles.

```bash
sm uninstall
```

### `sm status`

Show what's installed in the current project. Displays the `.sm.json` configuration and all symlinks installed in each tool's skills directory.

```bash
# In project directory
sm status

# Or specify directory
sm status --dir ~/my-project
```

**Flags:**
- `--dir` — Project directory (default: current dir)

### `sm init [name]`

Two modes of operation:

1. **Without arguments:** Initialize a project with a `.sm.json` configuration file.
2. **With a name:** Create a new `SKILL.md` template in a subdirectory.

```bash
# Initialize project config
cd ~/my-project
sm init

# With a profile
sm init --profile cloudflare

# Create a new skill template
sm init my-skill

# Create in a custom directory
sm init my-skill --dir custom-path
```

**Flags:**
- `--profile` — Profile name to use as base (project mode)
- `--dir` — Directory name for skill template (default: skill name)

### `sm update [skills...]`

Update git-managed registry entries to latest versions.

```bash
# Update all skills
sm update

# Update a single skill by name
sm update my-skill

# Update multiple specific skills
sm update frontend-design web-design-guidelines

# Non-interactive (auto-detects scope)
sm update -y
```

**Flags:**
- `-g, --global` — Only update global skills
- `-p, --project` — Only update project skills
- `-y, --yes` — Skip scope prompt (auto-detect)

### `sm check`

Verify installation integrity.

```bash
sm check
sm check --fix  # Auto-repair broken symlinks
```

Scans all tool skill directories for broken or orphaned symlinks and checks project records in the database.

**Flags:**
- `--fix` — Auto-fix broken symlinks and stale records

### `sm doctor`

Check environment and dependencies.

```bash
sm doctor
```

Verifies all AI tool CLI binaries (Git, Claude, Codex, Gemini, OpenCode, Hermes, OpenClaw, Go), directories, database, and environment variables. Also detects optional [aivo](https://github.com/yuanchuan/aivo) integration.

### `sm list [--skills|--mcp]`

List all registry contents.

```bash
sm list
sm list --skills
sm list --mcp
```

**Flags:**
- `--skills` — List only skills
- `--mcp` — List only MCP
- `-g, --global` — List only global skills
- `-a, --agent <agents>` — Filter by specific agents

### `sm profile`

Manage skill profiles. Profiles bundle skills and MCP configurations for scenarios.

```bash
# List available profiles
sm profile list

# Show profile contents
sm profile show cloudflare

# Create a new profile
sm profile create my-profile --skills "skill-a,skill-b" --mcp "mcp-server"

# Delete a profile
sm profile delete my-profile
```

**Flags (create):**
- `--skills` — Comma-separated list of skills
- `--mcp` — Comma-separated list of MCP servers

### `sm backup` / `sm restore`

Create and restore configuration backups.

```bash
# Create backup
sm backup
sm backup --name "pre-upgrade"
sm backup --rotate 5  # Keep only 5 most recent

# Restore from backup
sm restore backup-20260607-150405
sm restore --latest
```

Backups include the database, registry, and profiles. Restoring automatically creates a pre-restore backup for safety.

**Flags (backup):**
- `--name` — Custom backup name
- `--rotate` — Keep only N most recent backups

**Flags (restore):**
- `--latest` — Restore from the most recent backup

### `sm completion [bash|zsh|fish|powershell]`

Generate shell completion scripts.

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

Manage prompt sets for different AI coding assistants. Prompt sets are collections of tool-specific prompt files (e.g., `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`) stored as JSON in the registry.

```bash
# List available prompt sets
sm prompt list

# Show contents of a prompt set
sm prompt show my-prompts

# Apply a prompt set to a project
sm prompt apply my-prompts --dir ~/my-project

# Apply only specific prompt files
sm prompt apply my-prompts --tools CLAUDE.md,AGENTS.md

# Create a prompt set from current project
sm prompt create my-prompts --dir ~/my-project

# Create from specific files only
sm prompt create my-prompts --files CLAUDE.md,AGENTS.md

# Delete a prompt set
sm prompt delete my-prompts
```

**Flags (apply):**
- `--dir` — Project directory (default: current dir)
- `--tools` — Comma-separated list of prompt files to apply (default: all in set)

**Flags (create):**
- `--dir` — Project directory (default: current dir)
- `--files` — Comma-separated list of prompt files to include (default: `CLAUDE.md,AGENTS.md,GEMINI.md`)


### `sm use <source>`

Use a skill without installing it. Resolves the source (same formats as `sm add`), writes the selected skill to a temporary directory, and either prints a generated prompt to stdout or starts a supported agent interactively.

```bash
# Print skill prompt to stdout (pipe to an agent)
sm use owner/repo --skill my-skill | claude

# Start an agent interactively with the skill
sm use owner/repo --skill my-skill --agent claude-code

# Use a local skill directory
sm use ./my-skill
```

**Flags:**
- `-s, --skill` — Specific skill to use
- `-a, --agent` — Start an agent interactively

### `sm find [query]`

Search for installed skills interactively or by keyword. Without arguments in an interactive terminal, shows an fzf-style picker to browse skills.

```bash
# Interactive picker (fzf-style browse)
sm find

# Search by keyword
sm find typescript
sm find "web design"
```

### `sm browse [query]`

Browse and search the online [skills.sh](https://skills.sh) directory. Selected skills can be installed directly.

```bash
# Browse all skills (interactive picker or table)
sm browse

# Search for a specific skill
sm browse typescript

# Browse trending skills
sm browse --trending

# Browse hot skills
sm browse --hot

# Browse skills for a specific agent
sm browse --agent claude-code

# Browse skills by topic
sm browse --topic react
```

Set `SKILLS_SH_TOKEN` or `VERCEL_OIDC_TOKEN` environment variable for API access.
Without a token, skill data is scraped from the public website.

### `sm export` / `sm import`

Export and import configuration.

```bash
# Export everything to file
sm export --output config.json

# Export only registry, profiles, and prompt sets
sm export --include registry,profiles,prompts --output config.json

# Export to stdout
sm export

# Import from file (merge mode, default)
sm import config.json

# Import from stdin
sm import -

# Import with replace mode
sm import config.json --replace

# Dry run (preview changes)
sm import config.json --dry-run
```

**Flags (export):**
- `-o, --output` — Output file path (default: stdout)
- `--include` — Comma-separated list of items to export: `registry`, `profiles`, `prompts`, `projects` (default: all)

**Flags (import):**
- `--merge` — Merge with existing data (default)
- `--replace` — Replace existing data (clear everything first)
- `--dry-run` — Show what would be imported without making changes
- `--merge` and `--replace` are mutually exclusive

### `sm web`

Start the web dashboard.

```bash
sm web           # Default port 3721
sm web -p 8080   # Custom port
```

**Flags:**
- `-p, --port` — Port to listen on (default: 3721)

### Global flags

All commands accept these persistent path flags, which default to user-scoped storage:

- `--registry` — Registry directory path (default: `~/.sm/registry`)
- `--data` — Data directory path (default: `~/.sm/data`)
- `--profiles` — Profiles directory path (default: `~/.sm/profiles`)
- `-v, --version` — Print the version and exit

## Project Config (.sm.json)

Projects store their configuration in `.sm.json`:

```json
{
  "profile": "cloudflare",
  "skills": ["extra-skill"],
  "mcp": ["extra-mcp"]
}
```

The profile is the base. `skills` and `mcp` arrays are ad-hoc additions.

## Directory Structure

```
SkillsManager/
├── cmd/                     ← CLI commands (Cobra)
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
├── internal/                ← Core packages
│   ├── registry/            ← Skill & MCP registry management
│   ├── installer/           ← Symlink-based install flow
│   ├── profile/             ← Profile loader & manager
│   ├── project/             ← .sm.json config handler
│   ├── prompt/              ← Prompt set manager
│   ├── db/                  ← SQLite database (projects & installations)
│   ├── backup/              ← Backup & restore logic
│   ├── symlink/             ← Symlink create/verify/cleanup
│   ├── tool/                ← AI tool definitions & detection
│   └── aivo/                ← Optional aivo integration
├── web/                     ← Web dashboard
│   ├── handler.go           ← REST API handlers
│   └── static/              ← Embedded frontend (HTML/CSS/JS)
├── registry/                ← Skill & MCP data
│   ├── skills/
│   │   ├── global/          ← Special: all tools
│   │   ├── codex-only/      ← Special: Codex only
│   │   ├── claude-only/     ← Special: Claude only
│   │   ├── gemini-only/     ← Special: Gemini only
│   │   ├── opencode-only/   ← Special: OpenCode only
│   │   ├── hermes-only/     ← Special: Hermes only
│   │   ├── openclaw-only/   ← Special: OpenClaw only
│   │   └── ...
│   └── mcp/
│       └── ...
├── profiles/                ← Profile definitions
├── data/                    ← Local state (gitignored)
│   ├── sm.db                ← SQLite database
│   └── backups/             ← Configuration backups
├── Formula/                 ← Homebrew formula (CI 自动同步版本/校验和)
├── install.sh               ← 一键安装脚本(从 latest release 拉取)
├── docs/                    ← Design specs & plans
├── .github/workflows/       ← CI/CD (Go tests + 多平台 release + formula 同步)
├── go.mod
└── LICENSE
```

Prompt sets are stored under `registry/prompts/`.

### `sm --version`

Show the current version.

```bash
sm --version
```

## Web Dashboard

The `sm web` command starts an embedded HTTP server with a browser-based dashboard for browsing and monitoring your skills, MCP servers, projects, and installation history.

### Tabs

| Tab | Description |
|-----|-------------|
| **Overview** | Summary stats (skills, MCP servers, projects, health), aivo integration status, recent installs |
| **Registry** | All skills grouped by category (with special directory badges), MCP server details |
| **Projects** | Registered projects with profile, extra skills, and last install time |
| **History** | Full install history with filter and sort (newest/oldest/project A-Z) |

### REST API

The dashboard is powered by the following endpoints:

| Endpoint | Description |
|----------|-------------|
| `GET /api/registry` | List all skills and MCP servers with details |
| `GET /api/projects` | List all registered projects |
| `GET /api/history` | List all installation records |
| `GET /api/check` | Run health check (broken/orphaned symlinks, missing projects) |
| `GET /api/tools` | Detect installed AI tools and skill directories |
| `GET /api/aivo` | aivo integration status, active key/model, token usage, key health |

All endpoints return JSON. The frontend is embedded into the binary at build time via `//go:embed`.

## Supported AI Assistants

SkillsManager supports **67+ AI coding agents**. Here are the primary ones:

| Assistant | `--agent` | Skills Directory | Config File |
|-----------|-----------|-----------------|-------------|
| Claude Code | `claude-code` | `~/.claude/skills/` | `CLAUDE.md` |
| Codex | `codex` | `~/.codex/skills/` | `AGENTS.md` |
| Gemini CLI | `gemini-cli` | `~/.gemini/skills/` | `GEMINI.md` |
| OpenCode | `opencode` | `~/.config/opencode/skills/` | `OPENCODE.md` |
| Hermes | `hermes-agent` | `~/.hermes/skills/` | `HERMES.md` |
| OpenClaw | `openclaw` | `~/.openclaw/skills/` | `OPENCLAW.md` |
| Cursor | `cursor` | `~/.cursor/skills/` | — |
| Cline | `cline` | `~/.agents/skills/` | — |
| Windsurf | `windsurf` | `~/.codeium/windsurf/skills/` | — |
| GitHub Copilot | `github-copilot` | `~/.copilot/skills/` | — |
| Kiro CLI | `kiro-cli` | `~/.kiro/skills/` | — |
| Roo Code | `roo` | `~/.roo/skills/` | — |
| Amp | `amp` | `~/.config/agents/skills/` | — |
| Goose | `goose` | `~/.config/goose/skills/` | — |

And 50+ more agents. Use `sm add --agent <name>` or `sm list --agent <name>` with any supported agent.

## aivo Integration

SkillsManager optionally integrates with [aivo](https://github.com/yuanchuan/aivo), an AI tool launcher and API key manager.

- `sm doctor` detects aivo installation, reports version, key count, active key, and unhealthy keys
- `sm status` shows the active aivo key and usage stats when aivo is installed
- The web dashboard (`sm web`) displays aivo stats in the Overview tab

aivo is **optional** — all sm commands work without it. To install:

```bash
brew install yuanchuan/tap/aivo
```

## Architecture

SkillsManager is built with Go and uses the following stack:

| Component | Technology |
|-----------|-----------|
| CLI framework | [Cobra](https://github.com/spf13/cobra) |
| Database | [SQLite](https://gitlab.com/cznic/sqlite) (pure Go, no CGO required) |
| Web frontend | Vanilla HTML/CSS/JS, embedded via `//go:embed` |
| Build/Release | GitHub Actions (6 platforms: linux/darwin/windows × amd64/arm64) |
| Distribution | Homebrew tap + Go install + GitHub Releases |

### Key Design Decisions

- **Symlink-based installs** — Registry holds originals; tool directories get symlinks for zero-dup and instant updates
- **Embedded web UI** — Single binary with no external dependencies; static files bundled at compile time
- **Pure Go SQLite** — No CGO required, enabling easy cross-compilation and static binaries
- **Profile system** — Declarative project configuration; profiles provide reusable presets

## Contributing

Before submitting changes:

```bash
go test ./...
go build -o sm .
```

Keep registry data, profiles, and local SQLite state out of unrelated code changes. Add focused tests for CLI behavior, registry rules, installer flows, and web API changes.

## Release

GitHub Actions builds release binaries only when a release tag is pushed:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Supported tag patterns are `v*`, `release`, and `release-*`. Versioned tags such as `v0.1.0` are preferred because they create stable release history. Each release includes binaries for Linux (amd64/arm64), macOS (amd64/arm64), and Windows (amd64/arm64) with SHA-256 checksums.

In addition, each versioned release automatically:

- regenerates `Formula/sm.rb` with the new version and per-platform SHA-256 (via `.github/scripts/sync_formula.py`), committed back to `main`;
- moves the `latest` tag to the newest commit, so `install.sh` and direct downloads always point at the newest release.

## License

[MIT](LICENSE) © 2026 woyin

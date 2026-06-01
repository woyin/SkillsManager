# SkillsManager (sm) — Design Spec

## Overview

A Go CLI tool for managing AI agent skills (Codex, Claude) and MCP server configurations across multiple projects. Single binary, embedded web dashboard, SQLite for state tracking.

**Binary name:** `sm`

## Core Principles

1. **Global is exclusive** — Only truly cross-tool skills go into `global/`. Everything else is project-level.
2. **Profiles are presets** — A profile bundles skills + MCP for a scenario. Projects can layer ad-hoc items on top.
3. **One original, all symlinks** — Registry holds the original files. All installed locations (`~/.codex/skills/`, `~/.claude/skills/`) are symlinks back to the registry.
4. **Always both tools** — Unless a skill is in `codex-only/` or `claude-only/`, it installs to both Codex and Claude by default.

## Directory Structure

```
SkillsManager/
├── registry/
│   ├── skills/
│   │   ├── global/              ← special: installs to ALL tools
│   │   │   └── skill-name/      ← original files (may have .git)
│   │   ├── codex-only/          ← special: Codex only
│   │   │   └── skill-name/
│   │   ├── claude-only/         ← special: Claude only
│   │   │   └── skill-name/
│   │   ├── cloudflare/          ← user-defined category
│   │   │   └── skill-name/
│   │   ├── nextjs/
│   │   ├── design/
│   │   └── security/
│   └── mcp/
│       ├── cloudflare.json
│       ├── github.json
│       └── browser.json
├── profiles/
│   ├── default.json
│   ├── cloudflare.json
│   ├── frontend.json
│   └── security-audit.json
├── data/
│   └── sm.db                    ← SQLite (gitignored)
├── cmd/                         ← CLI command implementations
├── internal/                    ← internal packages
├── web/                         ← embedded web UI assets
├── go.mod
├── go.sum
├── main.go
├── README.md
├── README.zh-CN.md
└── .gitignore
```

### Special Directories

Three directories have fixed install targets:

| Directory | Install Target |
|-----------|---------------|
| `global/` | `~/.codex/skills/` AND `~/.claude/skills/` |
| `codex-only/` | `~/.codex/skills/` only |
| `claude-only/` | `~/.claude/skills/` only |

All other directories are user-defined categories. When installed, category skills target both tools by default.

## Data Formats

### Profile (`profiles/cloudflare.json`)

```json
{
  "skills": ["global", "cloudflare"],
  "mcp": ["cloudflare", "github"]
}
```

Skill names in the `skills` array map to directory names under `registry/skills/`. MCP names map to `<name>.json` under `registry/mcp/`.

### Project Config (`.sm.json` in project directory)

```json
{
  "profile": "cloudflare",
  "skills": ["extra-skill-a"],
  "mcp": ["extra-mcp-b"]
}
```

Profile is the base. `skills` and `mcp` arrays are ad-hoc additions layered on top.

### MCP Definition (`registry/mcp/cloudflare.json`)

Standard `.mcp.json` format:

```json
{
  "mcpServers": {
    "cloudflare-api": {
      "type": "http",
      "url": "https://mcp.cloudflare.com/mcp"
    }
  }
}
```

## Commands

### `sm add <source> [category] [--global|--codex|--claude] [--mcp]`

Add a skill or MCP to the registry.

- `<source>`: GitHub URL or local path
- `[category]`: Target directory under `registry/skills/` or `registry/mcp/`
- `--global`: Place into `registry/skills/global/`
- `--codex`: Place into `registry/skills/codex-only/`
- `--claude`: Place into `registry/skills/claude-only/`
- `--mcp`: Treat as MCP server definition

Behavior:
- GitHub URL → `git clone` into target dir
- Local path → copy into target dir
- If no category and no flag → error, must specify

```
sm add github.com/user/repo/path cloudflare
sm add ./my-skill --global
sm add github.com/user/mcp-server --mcp
```

### `sm rm <name> [--global|--codex|--claude] [category] [--mcp]`

Remove from registry. Also removes all symlinks pointing to it in installed locations (`~/.codex/skills/`, `~/.claude/skills/`).

### `sm install [--profile name] [--dir path]`

Install skills and MCP into a project.

1. Read `.sm.json` from current dir (or `--dir`)
2. If `--profile` given, override the profile field
3. Resolve profile → expand to full skill + MCP lists
4. Merge with ad-hoc items from `.sm.json`
5. For each skill:
   - Determine target(s) based on special directory rules
   - Create symlink: `~/.codex/skills/<name> → registry/skills/<category>/<name>`
   - If already exists and points to same target, skip
   - If exists and points elsewhere, warn and ask
6. For each MCP:
   - Read `registry/mcp/<name>.json`
   - Merge into project's `.mcp.json` (create if missing)
7. Record in SQLite: timestamp, project dir, skills, MCP, profile
8. Write/update `.sm.json` in project dir

### `sm update`

Update all registry entries to their latest versions.

1. Walk `registry/skills/**/` and `registry/mcp/**/`
2. For each directory containing `.git/`:
   - `git pull --ff-only`
3. Report summary: updated count, skipped count, errors

### `sm check`

Verify installation integrity.

1. Scan `~/.codex/skills/` and `~/.claude/skills/` for symlinks
2. Check each symlink target exists in registry
3. Check all projects in SQLite — verify project directories still exist
4. Report: broken symlinks, missing projects, orphaned entries
5. `--fix` flag: remove broken symlinks and clean up stale project records

### `sm web [-p port]`

Start embedded HTTP server for the dashboard.

- Default port: 3721
- Dashboard views:
  - **Overview**: total skills, total MCP, total projects, recent installs
  - **Registry**: browse by category, source URL, last updated
  - **Projects**: each project's profile + ad-hoc items, install date, broken symlinks flagged
  - **History**: chronological install log

### `sm list [--skills|--mcp]`

List all registry contents, grouped by category.

## Symlink Model

Registry is the source of truth. All installations are symlinks:

```
registry/skills/cloudflare/workers-skill/  ← ORIGINAL
    ↑                                       ↑
~/.codex/skills/workers-skill/    ~/.claude/skills/workers-skill/
```

Rules:
- Only one original file set exists in the registry
- All installed locations are symlinks back to registry
- `sm rm` cleans up both the registry entry and all symlinks pointing to it
- `sm check` detects broken symlinks (target deleted or moved)

## Update Mechanism

Registry entries that originated from GitHub have a `.git` directory. `sm update` walks all registry dirs, finds `.git` repos, and runs `git pull --ff-only` on each. Non-git entries (local copies) are skipped.

No extra metadata file is needed — the `.git` directory itself is the indicator of updateability.

## SQLite Schema

File: `data/sm.db` (gitignored, local state only)

```sql
CREATE TABLE IF NOT EXISTS installations (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    project_path TEXT NOT NULL,
    profile      TEXT,
    skills       TEXT,  -- JSON array: ["skill-a", "skill-b"]
    mcp          TEXT,  -- JSON array: ["cloudflare", "github"]
    installed_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS projects (
    path           TEXT PRIMARY KEY,
    profile        TEXT,
    extra_skills   TEXT,  -- JSON array
    extra_mcp      TEXT,  -- JSON array
    last_installed DATETIME
);
```

## Web Dashboard

Embedded via Go's `embed.FS`. Single binary contains all HTML/CSS/JS.

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/registry` | GET | All skills + MCP grouped by category |
| `/api/projects` | GET | All installed projects with config |
| `/api/history` | GET | Full install history from SQLite |
| `/api/check` | GET | Run integrity check, return broken items |
| `/` | GET | Dashboard UI |

### UI Structure

- **Sidebar**: Navigation (Overview, Registry, Projects, History)
- **Overview page**: Stats cards, recent installs table
- **Registry page**: Collapsible categories, skill cards with source info
- **Projects page**: Project list with profile, items, status indicators
- **History page**: Sortable/filterable install log

## .gitignore

```gitignore
# SQLite database (local state, not shared)
data/*.db
data/*.db-wal
data/*.db-shm

# Go build artifacts
sm
*.exe
/bin/

# OS files
.DS_Store
Thumbs.db

# Editor
.idea/
.vscode/
*.swp
*~
```

## README Deliverables

Two README files:

1. **README.md** — English version with:
   - Project description and design philosophy
   - Installation instructions
   - Command reference with examples
   - Directory structure explanation
   - Symlink model explanation
   - Contributing guidelines

2. **README.zh-CN.md** — Chinese version with identical content

## Clarifications & Edge Cases

### Skill Naming

When adding from a source:
- GitHub URL `github.com/user/repo/skills/foo` → name is the last path segment (`foo`)
- GitHub URL `github.com/user/repo` → name is the repo name (`repo`)
- Local path `./my-skill` → name is the directory name (`my-skill`)

### Profile Resolution Errors

If a profile references a skill or MCP that doesn't exist in the registry, `sm install` reports the missing item and continues with what's available. Does not fail entirely.

### MCP Merging

When multiple MCP sources contribute to a project's `.mcp.json`, the `mcpServers` objects are merged. If two sources define the same server key, the later one wins (with a warning).

### Install Without Config

Running `sm install` without a `.sm.json` and without `--profile` prints an error with usage hints.

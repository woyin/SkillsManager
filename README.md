# SkillsManager (sm)

A CLI tool for managing AI agent skills (Codex, Claude) and MCP server configurations across multiple projects.

## Design Philosophy

### One Original, All Symlinks

The registry holds the original files. All installed locations (`~/.codex/skills/`, `~/.claude/skills/`) are symlinks pointing back to the registry. This means:

- **No duplication** — disk space is minimal
- **Instant updates** — update the registry, all installations reflect the change
- **Easy cleanup** — remove the registry entry, all symlinks break visibly

### Profiles as Presets

A profile bundles a set of skills and MCP configurations for a scenario (e.g., "cloudflare development", "frontend", "security audit"). Projects reference a profile as a base, then layer ad-hoc additions on top.

### Minimal Global, Maximal Local

Only truly cross-tool skills go into `global/`. Domain-specific skills are installed per-project via profiles. This keeps each project's AI environment focused and lightweight.

### Three Special Directories

| Directory | Behavior |
|-----------|----------|
| `global/` | Installs to both Codex and Claude |
| `codex-only/` | Installs to Codex only |
| `claude-only/` | Installs to Claude only |

All other directories are user-defined categories. Skills in category directories install to both tools by default.

## Installation

### Homebrew (macOS/Linux)

```bash
brew tap woyin/tap
brew install sm
```

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
- `--global` — Add to `global/` directory
- `--codex` — Add to `codex-only/` directory
- `--claude` — Add to `claude-only/` directory
- `--mcp` — Treat as MCP server definition. The source may be a JSON file, a directory, or a Git repo containing `.mcp.json`, `mcp.json`, or another JSON file with `mcpServers`. Git MCP sources are kept as repos under `registry/mcp/` so `sm update` can refresh them.

### `sm rm <name> [category]`

Remove a skill or MCP from the registry. Also cleans up symlinks in installed locations.

```bash
sm rm my-skill
sm rm my-skill --global
sm rm cloudflare --mcp
```

### `sm install`

Install skills and MCP into a project directory.

```bash
# In project directory
cd ~/my-project
sm install --profile cloudflare

# Or specify directory
sm install --profile frontend --dir ~/my-project
```

Reads `.sm.json` if present. Creates symlinks in `~/.codex/skills/` and `~/.claude/skills/`. Writes `.mcp.json` for MCP configurations. Records installation in SQLite database.

If an installed skill symlink already points somewhere else, `sm install` warns and asks before replacing it.
If an MCP server key already exists in the project `.mcp.json`, the new definition wins and `sm install` prints a warning.

### `sm update`

Update all git-managed registry entries.

```bash
sm update
```

Walks `registry/`, finds directories with `.git`, runs `git pull --ff-only` on each.

### `sm check`

Verify installation integrity.

```bash
sm check
sm check --fix  # Auto-repair broken symlinks
```

`sm check` reports broken symlinks, symlinks that point outside the configured registry, and project records whose directories no longer exist.

### `sm list [--skills|--mcp]`

List all registry contents.

```bash
sm list
sm list --skills
sm list --mcp
```

### `sm web`

Start the web dashboard.

```bash
sm web           # Default port 3721
sm web -p 8080   # Custom port
```

The dashboard shows registry categories, source URLs for Git-backed entries, last-updated timestamps, project records, filterable/sortable install history, and health issues from `sm check`.

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
├── registry/
│   ├── skills/
│   │   ├── global/          ← Special: all tools
│   │   ├── codex-only/      ← Special: Codex only
│   │   ├── claude-only/     ← Special: Claude only
│   │   ├── cloudflare/      ← User-defined category
│   │   └── ...
│   └── mcp/
│       ├── cloudflare.json
│       └── ...
├── profiles/
│   ├── cloudflare.json
│   └── ...
├── data/
│   └── sm.db                ← SQLite (local, gitignored)
└── ...
```

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

Supported tag patterns are `v*`, `release`, and `release-*`. Versioned tags such as `v0.1.0` are preferred because they create stable release history.

## License

MIT

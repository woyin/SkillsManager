# Registry Install, Profile Install to Project scope, and In-Place Update

Status: partially superseded by ADR-0010 and ADR-0016

## Context

SkillsManager's `sm add` already registers skill originals into the Registry without installing (`cmd/add.go`), and `sm install <source>` Direct Installs from a Source in one action. What was missing: a fast install path from the local Registry by name, a project-scoped one-command install of a Profile, and a way to refresh Copy Install entities without disturbing the Registry. The prior Profile Install path also installed to Global scope, which conflicted with the project-first model the rest of the CLI assumes.

## Decision

We add three things and change one default.

1. **Registry Install** (`sm install <name> --from-registry`): install a skill by name from the Registry's stored originals — a symlink into the existing original, no re-clone. Defaults to Project Scope; `--global` opts into Global Scope. If the name is absent from the Registry, it errors with a hint to `sm add` first — it never falls back to Direct Install, so install latency is predictable. If the name exists under multiple Registry categories, it errors and asks for `--category <cat>`; a single match installs directly.

2. **Profile Install defaults to Project Scope** (`sm install --profile <name>`, and `sm install` with no source): skills land in `./<agent>/skills` for the current project, not `~/<agent>/skills`. `--global` opts back into Global Scope. This is a breaking change from the prior global default (see Migration).

3. **In-Place Update** (`sm update --in-place`): refresh Copy Install entities in the current project in place from their own Skill Origin, without touching the Registry. Link Installs are no-ops (a symlink already follows the Registry). If a Copy Install's source cache is missing, it errors and points the user to `sm update` (no `--in-place`); this path never clones remotely, keeping it local and fast.

4. **Default agent targets**: keep `tool.DetectInstalled` as the rule. When no agent is detected, fall back to the {Claude, Codex, Pi} tool set (project skill dirs `.claude/skills`, `.agents/skills`, `.pi/skills`) instead of the prior {Claude, Codex}.

We also close the Profile CRUD gap with `sm profile update <name> [--skills ...] [--mcp ...]` (overwrite-by-flags); no interactive `profile edit` is added until a real use case appears.

## Rejected alternatives

- **Registry Install by arity / implicit fallback**: rejected. Falling back to Direct Install when a name is not in the Registry would trigger an unpredictable clone, breaking the "install is fast and local" promise. The flag `--from-registry` makes the intent explicit.
- **A `--duplicate` install mode**: rejected. The existing `--copy` already produces a non-symbolic install; a second word for the same thing adds a choice users cannot reason about. Copy Install is the single term.
- **`update --local`**: rejected as the flag name. `--local` reads as a peer of the existing `--project`/`--global` scope flags and would be misread as "update local project scope." `--in-place` names the action, stays orthogonal to scope, and composes (`update --in-place --global` remains expressible).
- **Letting In-Place Update clone remotely when the source cache is missing**: rejected. `--in-place` is the local, fast path; a hidden network clone would betray it. Cache reconstruction stays on the normal `sm update` path, which already implements it.
- **A new `--duplicate` flag for "fully independent copies"**: rejected. A Copy Install that escapes Registry tracking would become invisible to list/status/uninstall — contradicting the Registry-as-index model.
- **Profile-scoped scope (each Profile remembers project vs global)**: rejected. Scope is an orthogonal axis; coupling it into the Profile file couples two independent dimensions for no current use case.
- **Fixed triple default (always install .agents/.claude/.pi)**: rejected. Forcing the triple onto a machine that uses only one agent pollutes the project with unused directories. Detection remains the rule; the triple is fallback only.
- **Interactive `profile edit`**: rejected as YAGNI. Overwrite-by-flags `profile update` is scriptable and covers the CRUD gap.

## Migration (breaking change)

Profile Install's default scope moves from Global to Project. Users with `sm install --profile` muscle memory will find skills under `./<agent>/skills` instead of `~/<agent>/skills`. Migration:

- To keep the prior behavior, pass `--global` explicitly.
- Existing globally-installed Profile skills are not moved; `sm uninstall` (default project + global) removes them, and `sm install --profile <name> --global` reinstalls to Global.
- Announce in the release notes and the `sm install --help` output.

## Consequences

- Three install paths (Direct, Registry, Profile) share one default scope (Project) and one `--global` opt-in, so the scope default is uniform and memorable.
- `sm update` (no flag) remains the Registry-refresh path that serves Link Installs and reconstructs missing source caches. `sm update --in-place` is the narrow Copy-Install refresh path. The two never overlap.
- The Registry stays the single index: even Copy Installs carry Skill Origin and remain visible to list/status/uninstall; there are no orphaned invisible copies from the install path.
- CONTEXT.md gains: Registry Install, Profile Install, Link Install, Copy Install, In-Place Update. "Detected Agents" gains the {Claude, Codex, Pi} fallback. "Project Scope" now names all three install paths as defaulting to it.

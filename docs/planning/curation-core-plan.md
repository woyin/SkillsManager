# Curation Core — Small-commit implementation plan

Derived from the handoff (`skillsmanager-handoff-2026-08-12.md`) and the accepted
ADRs 0019–0029. This plan covers **tranche 1: Curation Core only**. Team Catalog
work (ADR 0019, 0024, 0025, 0026, 0029) remains out of scope until the user
explicitly expands scope.

## Goal

Introduce the `sm plan` command that produces, previews, and atomically applies
a Curation Plan for a project's Installed Skills, plus the on-disk schema and
ownership tracking those ADRs call for. It is preview-first (read-only by
default), requires explicit application (`--apply`), is atomic (Profile-Install
safety standard), and removes only owned Link Installs.

## Decisions encoded (from ADRs)

- **0020** — Plans require explicit application. `sm plan` changes nothing by default.
- **0021** — Layered baseline: explicit Profile / `.sm.json` > Team Catalog policies
  > project-environment inference (non-binding advice). (Catalog layer is toliated for tranche 2.)
- **0022** — `sm plan` is the Curation Command; `sm status` stays a health report.
- **0023** — Application removes only owned Link Installs. Manual installs, Copy
  Installs, and unowned entries are cleanup candidates, never auto-removed.
- **0027** — Inference is local and evidence-based (manifests, markers, `.sm.json`,
  existing `.sm-origin.json`). No code upload, no model, no full-content input.
- **0028** — New projects (no `.sm.json`) get a Bootstrap Curation Plan; applying
  requires the user to choose an explicit target first; only then may `.sm.json`
  be created and the selected composition installed atomically.
- **0023 + ownership** — plan-removal ownership is recorded explicitly, not inferred
  from existing registry/lockfile entries.

## On-disk / JSON schemas

### `.sm.json` (extend `internal/project.Config`)

Add an explicit, versioned record of plan ownership so deletion authority is
never inferred:

```jsonc
{
  "profile": "cloudflare",          // existing
  "skills": ["extra"],              // existing (extra skills)
  "mcp": ["github"],                // existing
  "curation": {                      // NEW
    "baseline": {                    // NEW: the layered baseline source
      "profile": "cloudflare",       // explicit profile (mirrors top-level when set)
      "skills": ["extra"],
      "mcp": ["github"]
    },
    "managed": {                     // NEW: project-scope Link Installs owned by a confirmed plan
      "claude": ["background-mcp"],  // agent -> skill names the plan may remove
      "codex": ["format-rules"]
    }
  }
}
```

`curation.baseline` snapshots the explicit composition a confirmed plan was
computed against, making later drift reproducible (ADR 0021, 0028). `managed`
records which project-scope Link Installs are owned by the plan; only these may
be removed at a later application (ADR 0023).

### `skills-lock.json` (leave as-is)

`internal/lockfile` continues to carry provenance only; it grants no deletion
authority. No schema change in tranche 1.

### JSON output for automation (`sm plan --json`)

```jsonc
{
  "project": "/abs/path",
  "baseline": { "profile": "cloudflare", "skills": ["extra"], "mcp": [] },
  "has_explicit_target": true,
  "proposals": [
    { "action": "add",    "skill": "foo", "agent": "claude", "reason": "required by project manifest", "evidence": ["found go.mod"] },
    { "action": "remove", "skill": "bar", "agent": "codex",  "reason": "not in baseline", "owned": true },
    { "action": "leave",  "skill": "baz", "agent": "claude", "reason": "in baseline", "owned": false }
  ],
  "warnings": [],
  "planned": true,
  "check": true
}
```

## Command behavior

### `sm plan [--dir DIR] [--apply] [--json]`

Preview-first. Read-only unless `--apply`.

1. Resolve project dir; load `.sm.json`.
2. If no explicit target (no profile and no skills in `.sm.json`) → **Bootstrap
   Curation Plan** (ADR 0028): recommend evidence-backed profiles/skills, but
   produce **no** add/remove until user selects an explicit target (via `--apply
   --profile <name>` / `--apply --skill <name>`). `--check` is advisory on bootstrap.
3. Otherwise build the layered baseline and diff the current Installed Skills
   against it (domain = detected agents' project skill dirs).
4. Classify each installed entry: in-baseline (leave), outside-baseline (remove
   candidate; owned flag set only if recorded in `managed`), unknown ownership
   (cleanup candidate, not auto-removable).
5. Propose adds from baseline members missing from an agent dir (when the
   agent is a target of that member, per registry category — global ⇒ all tools,
   special dir ⇒ single tool).
6. `--check`: return nonzero if any required change is unsatisfied (no required
   policy yet in tranche 1, so `--check` is satisfiable unless something is a
   hard error).
7. `--apply`: preflight entire plan (every add resolves, every owned removal
   still verify ownership) → apply atomically. **Never** removes unowned / Copy /
   manual entries; they are only reported. On success updates `managed` so future
   plans treat removed entries as outside-baseline owned (and any that were added
   become managed). On failure, roll back everything (Profile-Install standard).

### `sm plan` exit codes

- `0` — no changes needed, or changes previewed and applied successfully.
- `1` — error / `--check` failed.

## Migration path

For existing projects (which have `.sm.json` without `curation`): plan treats
existing installs as baseline-aligned by default and does **not** retroactively
grant removal ownership. No migration script needed; new/confirmed plans start
recording `managed` from their first application.

## Module shape

New package: `internal/curation` — a deep module holding planning + reconcile
logic (per ADR 0018 / codebase-design, NOT stuffed into `cmd/status.go` or
`cmd/install.go`).

- `curation.Planner` — given registry + project + tools, produce a `Plan`.
- `curation.Plan` — proposals + baseline, `JSON()`/`check`, and `Apply()`.
- `curation.Ownership` — read/write `.sm.json#curation.managed` via `project`.

`cmd/plan.go` — a narrow coordinator that wires flags → `curation` → output.

## Small commits

1. `feat(project): add curation baseline + managed ownership to .sm.json`
2. `feat(curation): planner that diffs Installed Skills against layered baseline`
3. `feat(curation): classify installs (in-baseline / outside / unowned + copy)`
4. `feat(curation): bootstrap plans for projects without an explicit target`
5. `feat(curation): atomic Apply that removes only owned Link Installs`
6. `feat(cmd): sm plan command (preview / --json / --check)` 
7. `feat(cmd): sm plan --apply with explicit target selection`
8. `docs: document sm plan and curation schema`
9. `chore: wire plan into status "Next" hints`

## Testing decisions

- Test at the public seam of `internal/curation` (Planner + Plan), and at the CLI
  seam (`cmd/plan`). Follow existing conventions in `internal/installer`/
  `internal/project` tests (temp dirs, no real registry network).
- Red→green per skill discipline (tdd): one plan → one implementation commit.

## Out of scope (tranche 1)

- Team Catalog adoption/forking, Catalog Policies (advisory/required), Catalog
  Pins, Git-first catalogs (ADR 0019, 0024, 0025, 0026, 0029).
- Any inference from source-code upload, model calls, or full content.
- Any auto-removal of manual/Copy/unowned entries.

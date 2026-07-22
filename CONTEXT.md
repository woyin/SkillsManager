# SkillsManager

CLI for managing AI agent skills and MCP configs. Users install skills so agents can load them; the registry is the lifecycle store, not the primary user concept.

## Language

**Skill**:
A directory containing a `SKILL.md` (and optional assets) that an AI agent can load.
_Avoid_: Package, plugin, tool (when meaning a skill)

**Agent**:
A supported AI coding assistant that can consume skills from a known skill directory.
_Avoid_: Tool (ambiguous with MCP tools), assistant, IDE

**Installed Skill**:
A skill present in an agent's skill directory for a given scope (project or global), so the agent can discover it.
_Avoid_: Registered skill, added skill

**Registry**:
The local store of skill originals (and MCP definitions) that `sm` owns for update, dedupe, and cleanup. Users need not manage it day to day.
_Avoid_: Cache (cache is ephemeral remote clones), catalog

**Source**:
A place to discover skills from: GitHub/GitLab shorthand or URL, local path, or skills.sh entry.
_Avoid_: Repo (too narrow), package

**Direct Install**:
The primary path: from a Source, land skills into Agent skill directories (and record originals in the Registry) in one user action.
_Avoid_: Add (add only registers), sync

**Registry Install**:
Install a skill by name from the Registry's stored originals, without re-cloning a Source. Opt-in via `--from-registry`. Defaults to Project Scope (like Direct Install and Profile Install); `--global` opts into Global Scope. The skill must already be in the Registry or it errors (no fallback to Direct Install), so install latency is predictable. If the name exists under multiple Registry categories, it errors and asks for `--category <cat>`; a single match installs directly.
_Avoid_: Local install, cache-install

**Register**:
Put skill originals into the Registry without necessarily making them Installed Skills.
_Avoid_: Install, add-as-install

**Scope**:
Where an Installed Skill lives: Project Scope (`./<agent>/skills`) or Global Scope (`~/<agent-skill-dir>`).
_Avoid_: Category (registry taxonomy), environment

**Project Scope**:
Install location under the current project root for each agent. Default scope for Direct Install, Registry Install, and Profile Install.
_Avoid_: Local (ambiguous), workspace-only

**Global Scope**:
Install location under the user home agent skill dirs. Opt-in via an explicit global flag.
_Avoid_: User scope (unless matching a specific agent term), machine-wide

**Detected Agents**:
Agents present on this machine (e.g. binary on `PATH`) used as the default install targets when the user does not name agents. If none are detected, install falls back to the {Claude, Codex, Pi} tool set, whose project skill dirs are `.claude/skills`, `.agents/skills`, and `.pi/skills`.
_Avoid_: All agents, supported agents (those are the full catalog), opinionated defaults (detection is the rule; the triple is fallback only)

**Profile**:
A named bundle of skills and MCP configs for a scenario. Secondary to Direct Install.
_Avoid_: Preset (ok synonym but Profile is canonical), template

**Profile Install**:
`sm install --profile <name>` (or `sm install` with no source): install every skill/MCP in a Profile at once. Defaults to Project Scope (breaking change from the prior global default); `--global` opts into Global Scope.
_Avoid_: Profile apply (use Profile Install), profile sync

**Skill Origin**:
Provenance metadata on a Registry skill (source, optional ref, path inside the clone, commit) that lets update refresh copy-installed skills via the source cache.
_Avoid_: Git remote alone, lockfile

**Origin-backed Skill**:
A Registry skill that has Skill Origin and can be refreshed by pulling the source cache and rewriting the original.
_Avoid_: Git-managed skill (that means a `.git` directory inside the skill path)

**Orphan Skill**:
A Registry or Installed Skill that is neither git-managed nor Origin-backed, so update cannot refresh it without reinstall.
_Avoid_: Broken skill (too broad), stale (ambiguous)

**Link Install**:
The default install mode: the Agent skill directory entry is a symlink into the Registry original. `sm update` refreshes the Registry original and every Link Install reflects it immediately.
_Avoid_: Symbolic install (say Link Install), soft install

**Copy Install**:
The alternative install mode (`--copy`): the Agent skill directory entry is an independent file copy of the Registry original, carrying its own Skill Origin so it can be refreshed in place. A Copy Install does NOT auto-reflect Registry updates; it must be refreshed by `sm update --in-place` acting on the copy itself.
_Avoid_: Duplicate (rejected synonym — use Copy Install), hardlink

**In-Place Update**:
`sm update --in-place`: refresh Copy Install entities in place from their own Skill Origin, without touching the Registry. No-ops on Link Installs (a symlink has no independent content to update — it already follows the Registry). If a Copy Install's source cache is missing, it errors and points the user to `sm update` (never clones remotely on this path).
_Avoid_: Local update (--local collides with the --project/--global scope flags), project update (ambiguous with --project scope)

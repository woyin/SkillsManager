# SkillsManager

CLI for curating a personal, cross-project library of AI agent skills and MCP configs, then deploying them to agents and projects.

## Language

**Skill**:
A directory containing a `SKILL.md` (and optional assets) that an AI agent can load. Registration requires `name` (1–64 lowercase letters, numbers, and single hyphens; no leading/trailing hyphen) and `description` (1–1024 characters). The Registry directory is normalized to the declared name, which is globally unique; category and Agent targeting do not form part of that identity.
_Avoid_: Package, plugin, tool (when meaning a skill)

**Agent**:
A supported AI coding assistant that can consume skills from a known skill directory.
_Avoid_: Tool (ambiguous with MCP tools), assistant, IDE

**Installed Skill**:
A skill present in an agent's skill directory for a given scope (project or global), so the agent can discover it.
_Avoid_: Registered skill, added skill

**Registry**:
The user-owned, cross-project source of truth for skill originals and MCP definitions. It contains at most one original for each Skill name, is the center of `sm`'s register, reuse, Profile Install, update, dedupe, and cleanup lifecycle, and lives under `~/.sm/registry` by default.
_Avoid_: Cache (cache is ephemeral remote clones), per-project canonical copy, catalog

**Team Catalog**:
A team-owned, versioned collection of shared Skills, Profiles, and optional policies. A member can use its shared originals or fork selected entries into their personal Registry, where the member controls subsequent changes.
_Avoid_: Shared Registry, team cache, global Registry

**Catalog Fork**:
A personal Registry version of a Team Catalog Skill that retains the Agent-visible Skill name and records the Catalog entry and version it came from. It does not receive automatic Catalog updates; later divergence, rebase, or return-to-Catalog choices are explicit.
_Avoid_: Override (does not retain provenance), duplicate, detached copy

**Catalog Policy**:
A versioned Team Catalog rule about project composition. An advisory policy is reported without blocking automation; a required policy causes `sm plan --check` to fail until the project satisfies it.
_Avoid_: Recommendation (does not state enforcement), mandatory profile, configuration

**Catalog Source**:
A Git repository that supplies a Team Catalog. Its URL, commit, and ref behavior are recorded as provenance; consumers may pin it to a tag or commit for reproducible project and CI evaluation.
_Avoid_: Hosted Catalog, Registry remote, policy server

**Catalog Pin**:
The resolved Team Catalog tag or commit recorded by a project as the version of its Curation Baseline. A newer Catalog revision becomes an explicit upgrade proposal and cannot alter the project's composition until confirmed.
_Avoid_: Live Catalog, automatic update, floating team baseline

**Source**:
A place to discover skills from: GitHub/GitLab shorthand or URL, local skill directory or collection, a valid local `SKILL.md` file, or skills.sh entry. A root `SKILL.md` makes a directory a single-Skill Source; otherwise Git and local directories use the same collection discovery rules. A single-file Source is materialized in the Registry as a standard Skill directory named from its frontmatter.
_Avoid_: Repo (too narrow), package

**Compatibility Baseline**:
The fixed external behavior reference used to judge unintentional differences in `sm`; the current baseline is `npx skills@1.5.20` after documented intentional divergences are excluded.
_Avoid_: Latest npx behavior, strict clone

**Compatibility Fix**:
A change that removes an unintentional difference from the Compatibility Baseline while preserving `sm`'s existing user-visible contract; a change to that contract requires explicit approval.
_Avoid_: Blind parity change, behavioral rewrite

**Functional Compatibility**:
Compatibility measured by command semantics, flags, filesystem effects, and exit outcomes, including interactive choices that change those outcomes; presentation-only wording and picker appearance are not part of the target.
_Avoid_: UI cloning, byte-for-byte terminal parity

**Compatibility Audit Record**:
A durable, itemized classification of all Compatibility Baseline behaviors as aligned, intentionally divergent, or fixed, used to establish and recheck completion.
_Avoid_: Handoff note, informal checklist

**Direct Install**:
The convenience path that registers skills from a Source and installs them into Agent skill directories in one user action. It feeds the same Registry-centered lifecycle as Register, Registry Install, and Profile Install.
_Avoid_: Add (add only registers), sync

**Registry Install**:
`sm install <name>`: install a uniquely named Skill from the Registry's stored original without re-cloning a Source. An unknown bare name errors with a Register hint and never falls back to network acquisition; explicit repository, URL, and filesystem syntax selects Direct Install. Defaults to Project Scope; `--global` opts into Global Scope. The legacy `--from-registry` flag is a deprecated alias.
_Avoid_: Local install, cache-install, registry mode

**Register**:
Put skill originals into the Registry without necessarily making them Installed Skills. `sm add <source>` defaults to the `global` category; Agent-specific categories require explicit targeting. A multi-Skill Source prompts for selection in a TTY and requires `--skill` or `--all` in non-interactive use. Registering from Git preserves Skill Origin even when the skill is extracted from a repository bundle or subdirectory. Re-registering the same Skill from the same Source refreshes it; replacing a same-named Skill from a different Source requires explicit force because every Link Install is affected.
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
A named bundle of Registry Skills and MCP definitions for a scenario. A Profile may be saved only when every referenced member exists and resolves uniquely in the Registry.
_Avoid_: Preset (ok synonym but Profile is canonical), template, wish list

**Profile Install**:
`sm install --profile <name>` (or `sm install` with no source): atomically install every Skill and MCP in a Profile. The entire Profile is preflighted; if any referenced item is unavailable, no links, MCP config, or project config are changed. Defaults to Project Scope; `--global` opts into Global Scope.
_Avoid_: Profile apply (use Profile Install), profile sync

**Curation Plan**:
An explainable, previewable set of proposed changes to a project's Installed Skills and MCP definitions, including the reason for every addition, removal, or update. A Curation Plan changes nothing until the user confirms it or explicitly requests non-interactive application.
_Avoid_: Auto-sync, recommendation list, drift report

**Curation Baseline**:
The ordered evidence used to assess a project's intended skill composition: an explicit project Profile or `.sm.json` first, Team Catalog policy requirements and prohibitions second, and project-environment inference only as non-binding advice.
_Avoid_: Desired state, auto-detected configuration, policy-only baseline

**Curation Evidence**:
The local, auditable facts that support a non-binding Curation Plan recommendation, including project manifests, framework configuration, directory markers, existing `.sm.json`, and Team Catalog rules. It excludes default code-content upload and model-based inference.
_Avoid_: Code analysis, telemetry, opaque recommendation

**Bootstrap Curation Plan**:
A Curation Plan for a project without an explicit Profile or `.sm.json`. It proposes evidence-backed Profiles or Skills but cannot create configuration, install content, or remove content until the user selects an explicit target.
_Avoid_: Automatic setup, inferred configuration, default profile

**Curation Command**:
The `sm plan` command that produces a Curation Plan for a project. It is read-only by default, supports an explicit application mode, and can emit a machine-readable representation for automation.
_Avoid_: Status command, auto-sync command, profile installer

**Curation-managed Link Install**:
A project-scope Link Install created by SkillsManager and known to belong to the project's Curation Baseline. It is the only Installed Skill entity that a confirmed Curation Plan may remove.
_Avoid_: Managed skill (too broad), installed skill (does not establish removal authority), tracked file

**Skill Origin**:
Provenance metadata on a Registry Skill: Source, resolved ref kind, optional requested ref, path inside the source, and resolved commit. A missing ref tracks the default branch; an explicit branch tracks that branch; a tag or commit is pinned. Ambiguous branch/tag names must be qualified as `refs/heads/...` or `refs/tags/...`.
_Avoid_: Git remote alone, lockfile

**Origin-backed Skill**:
A Registry skill that has Skill Origin and can be refreshed from the source cache. Every skill registered from an unpinned Git Source is Origin-backed, including extracted skills and repository subdirectories. Default and explicitly named branches advance during Registry Update; tags and commits remain pinned.
_Avoid_: Git-managed skill (that means a `.git` directory inside the skill path)

**Snapshot Skill**:
A Registry original copied from a local skill directory or local `SKILL.md`. It is independent of the original local path and never changes during Registry Update; re-register the local Source to replace the snapshot deliberately.
_Avoid_: Orphan Skill, linked local skill, Copy Install (that describes an installed entity)

**Registry Update**:
`sm update`: refresh every updatable original in the Registry. Skill-name arguments narrow it to named Registry Skills; `--project [--dir]` and `--global` narrow it to the Registry Skills referenced by those install scopes, and using both selects their union. The legacy `--registry` flag is a deprecated alias for the bare default. Tracking Git Skills are updated; pinned Skills and Snapshot Skills are healthy skips; Orphan Skills are errors. Because Link Installs point to Registry originals, one Registry Update propagates across every project and Agent that links them. Sources update independently: a failed source keeps its prior valid originals while other sources continue, and any failure makes the command exit nonzero with a summary.
_Avoid_: Installed-skill update, project update, update-all flag

**Registry List**:
`sm list`: show the personal Registry inventory. `--installed` switches to Installed Skills and composes with project, global, and Agent filters; the legacy `--registry` flag is a deprecated alias for the default view.
_Avoid_: Installed list (unless `--installed` is selected), catalog search

**Uninstall**:
Remove Installed Skill entities from selected projects or Agents without changing their Registry originals.
_Avoid_: Remove, delete, unregister

**Remove**:
Delete a Skill original from the Registry. Removal refuses while any known Installed Skill references the original; explicit force removes all known installs and lock entries first, reports inaccessible historical projects, and then deletes the original.
_Avoid_: Uninstall, unlink

**Orphan Skill**:
A legacy or damaged Registry skill that should have update provenance but has neither a managed Git repository nor valid Skill Origin. An intentional Snapshot Skill is not orphaned.
_Avoid_: Snapshot Skill, broken skill (too broad), stale (ambiguous)

**Link Install**:
The default install mode: the Agent skill directory entry is a symlink into the Registry original. Registry Update refreshes the original and every Link Install reflects it immediately.
_Avoid_: Symbolic install (say Link Install), soft install

**Copy Install**:
The alternative install mode (`--copy`): the Agent skill directory entry is an independent file copy of the Registry original, carrying its own Skill Origin so it can be refreshed in place. A Copy Install does NOT auto-reflect Registry updates; it must be refreshed by `sm update --in-place` acting on the copy itself.
_Avoid_: Duplicate (rejected synonym — use Copy Install), hardlink

**In-Place Update**:
`sm update --in-place`: refresh Copy Install entities in place from their own Skill Origin, without touching the Registry. No-ops on Link Installs (a symlink has no independent content to update — it already follows the Registry). If a Copy Install's source cache is missing, it errors and points the user to `sm update` (never clones remotely on this path).
_Avoid_: Local update (--local collides with the --project/--global scope flags), project update (ambiguous with --project scope)

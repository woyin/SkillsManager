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

**Register**:
Put skill originals into the Registry without necessarily making them Installed Skills.
_Avoid_: Install, add-as-install

**Scope**:
Where an Installed Skill lives: Project Scope (`./<agent>/skills`) or Global Scope (`~/<agent-skill-dir>`).
_Avoid_: Category (registry taxonomy), environment

**Project Scope**:
Install location under the current project root for each agent. Default scope for Direct Install.
_Avoid_: Local (ambiguous), workspace-only

**Global Scope**:
Install location under the user home agent skill dirs. Opt-in via an explicit global flag.
_Avoid_: User scope (unless matching a specific agent term), machine-wide

**Detected Agents**:
Agents present on this machine (e.g. binary on `PATH`) used as the default Direct Install targets when the user does not name agents.
_Avoid_: All agents, supported agents (those are the full catalog)

**Profile**:
A named bundle of skills and MCP configs for a scenario; layered onto a project. Secondary to Direct Install.
_Avoid_: Preset (ok synonym but Profile is canonical), template

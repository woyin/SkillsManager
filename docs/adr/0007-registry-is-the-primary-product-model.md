# Registry is the primary product model

The user-facing center of SkillsManager is a personal, cross-project Registry under `~/.sm`: users register skill originals once, reuse them by name or Profile across projects, and update the shared originals so Link Installs reflect the change. Direct Install remains a convenience path into that same lifecycle rather than the defining product model. This supersedes ADR-0001 and the parts of ADR-0003 that treat the Registry as an opt-in implementation detail; the exact default scope of list and update commands is decided separately.

We accept the additional Registry vocabulary because a cross-project, user-owned library is the intended distinction from tools whose canonical skill copies remain project- or agent-scoped. Hiding the Registry would also hide the reason Profile Install and shared updates are useful.

The primary curation entry point is therefore complete without storage vocabulary: `sm add <source>` registers into the all-Agent `global` category by default, while explicit Agent flags select narrower targets.

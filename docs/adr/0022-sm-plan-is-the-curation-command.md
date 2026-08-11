# `sm plan` is the Curation Command

SkillsManager will expose Curation Plans through a dedicated `sm plan` command. It previews changes by default, supports explicit atomic application, and offers JSON output for automation; `sm status` remains a concise health report rather than becoming an action-oriented interface.

Combining plan generation with `status` would blur whether a command merely observes or is intended to reconcile state. A separate command creates a stable surface for review, CI, and later editor or dashboard integrations without complicating the existing lifecycle commands.

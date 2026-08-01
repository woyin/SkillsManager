# All Git registrations preserve origin

Every skill registered from Git records Skill Origin, whether the Source is a whole single-skill repository, a repository subdirectory, or one selection extracted from a multi-skill repository. Registry Update advances every unpinned Git Source and rewrites its registered originals; Sources explicitly pinned to a tag or commit remain fixed until deliberately re-registered at another ref.

Keeping a whole repository clone only in the easy case while losing provenance for extracted skills makes update behavior depend on repository layout, which users should not need to understand. Persistent source caches and per-skill origin metadata provide one lifecycle for all Git shapes at the cost of retaining shared acquisition metadata under `~/.sm/data/sources`.

# New projects require an explicit Curation target

For a project without `.sm.json`, `sm plan` produces a Bootstrap Curation Plan that recommends Profiles or Skills from local evidence. Applying any change requires the user to choose an explicit target first; only then may SkillsManager create `.sm.json` and install the selected composition atomically.

Automatically materializing inferred configuration would turn a helpful suggestion into an opaque project decision and create an unreliable baseline for later reconciliation. Requiring one confirmation keeps first-use low-friction while giving future plans a durable, reproducible target.

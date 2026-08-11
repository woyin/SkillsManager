# Curation Plans require explicit application

SkillsManager will diagnose a project's skill composition and present a Curation Plan before changing it. The user must confirm the plan, or pass an explicit non-interactive apply option; the confirmed plan is preflighted and applied atomically using the same safety standard as Profile Install.

Silent reconciliation would let an inferred recommendation alter Agent behavior and potentially remove a project-specific exception. A diagnostic-only surface avoids that risk but leaves the routine maintenance burden unresolved. Requiring review makes recommendations useful while preserving user control and scriptability.

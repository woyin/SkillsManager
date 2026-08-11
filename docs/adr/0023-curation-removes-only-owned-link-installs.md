# Curation removes only owned Link Installs

When applying a Curation Plan, SkillsManager may remove only project-scope Curation-managed Link Installs that are explicitly outside the Curation Baseline. Manual installations, Copy Installs, and items whose ownership cannot be established remain visible as cleanup candidates but are never removed by the plan.

The current project configuration and lockfile record intent and provenance, not a blanket deletion grant. Extending reconciliation to every discovered directory entry would risk deleting project-specific work or independently managed content. Restricting removal to owned Link Installs gives plan application a safe, explainable boundary.

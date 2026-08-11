# Catalog Forks retain Skill name and provenance

A Catalog Fork keeps the original Agent-visible Skill name in the member's personal Registry and records the Team Catalog entry and version from which it was forked. A fork is never automatically overwritten by Catalog updates; SkillsManager instead reports divergence and offers explicit rebase, continued-fork, or return-to-Catalog actions.

Using a different installed name would break callers and Agent discovery expectations, while retaining two same-named originals would violate the Registry's global-name invariant and create ambiguous installs. One local name with durable upstream provenance preserves both deterministic installation and a recoverable collaboration history.

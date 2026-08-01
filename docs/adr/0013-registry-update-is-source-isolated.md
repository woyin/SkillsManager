# Registry Update isolates failures by Source

Registry Update processes independent Sources without making the entire Registry one transaction. If acquisition or validation fails for a Source, that Source's registered originals remain at their previous valid versions while other Sources continue; any such failure produces a nonzero final exit and an itemized summary.

One remote outage should not prevent unrelated skills from updating, but a zero exit after partial failure would make automation and users believe the personal library is fully current. Source-level rollback is the smallest transaction boundary that provides both useful progress and truthful status.

Pinned tag/commit Skills and intentional local Snapshot Skills are reported as healthy skips. A Skill that should have provenance but is orphaned is an error and contributes to the nonzero result.

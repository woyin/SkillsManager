# Curation uses an explicit layered baseline

Curation Plans evaluate project composition against a Curation Baseline: the project's explicit Profile or `.sm.json` takes precedence, Team Catalog policies express optional requirements or prohibitions beneath it, and repository-derived inference can only make non-binding recommendations. This makes a plan reproducible and lets users understand every proposed change.

Inferring the entire target composition from repository signals would make recommendations opaque and unstable. Treating Team Catalog policy as the sole authority would erase project intent. The layered model supports governance without making normal project configuration dependent on a central service.

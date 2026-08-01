# Cross-source name replacement requires force

Registering an existing Skill name from the same Source is a refresh. Registering that name from a different Source fails by default and requires an explicit `--force` replacement that warns all Link Installs will observe the new original. Register-only and Direct Install follow the same rule. This supersedes ADR-0004's decision to let Direct Install overwrite name clashes after only a warning.

A Registry name identifies one shared original across projects, so silently changing its Source has a much larger blast radius than overwriting a project-local installation. We accept one extra flag for intentional source replacement to prevent an unrelated install from changing existing projects invisibly.

# Skill names are globally unique in the Registry

A Skill's frontmatter `name` is its identity throughout the personal Registry, Registry Install, Profiles, and Registry Update. The Registry may contain only one original with a given name; category and Agent targeting are attributes rather than additional identity components. Distinct variants must use distinct names. This supersedes ADR-0005's support for storing the same name in multiple categories and resolving the ambiguity at install time.

We reject `(category, name)` identity because Profiles currently reference Skills by name and cross-project reuse should not depend on storage layout. Global uniqueness gives every name one deterministic original and update source, at the cost of requiring users to rename intentionally different variants that previously shared a name.

Registration therefore requires Agent Skills-compatible `name` (1–64 lowercase alphanumeric or single-hyphen characters, without leading/trailing hyphens) and `description` (1–1024 characters) frontmatter and never invents identity from the source directory name. Registry materialization normalizes the destination directory to the declared name. Other lint findings may remain warnings, but an original without its declared identity or trigger description cannot enter the Registry.

## Migration

Existing duplicate names are never deleted, renamed, or selected automatically. `sm doctor` reports every conflicting category, and operations that require a unique Skill identity—including name-based install, Profile Install, and update—fail with instructions to remove or re-register a variant explicitly. Normal operation resumes after the user leaves one original for that name.

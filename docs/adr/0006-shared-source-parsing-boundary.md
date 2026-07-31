# Shared Source Parsing Boundary

`registry` needs `lockfile.Manager` to apply source-lock discovery semantics, but `lockfile` previously depended on `registry` for source parsing. We place the shared pure parsing operations in `internal/sourceutil`; `registry` keeps its public wrappers and `lockfile` calls the shared package directly. This removes the dependency cycle without duplicating parsing rules or making the lockfile layer depend on registry lifecycle behavior.

## Considered Options

- **Keep `lockfile → registry` and hand-parse the source lock in registry**: rejected because it would duplicate lockfile parsing and risk format drift.
- **Move lockfile reading into registry**: rejected because the lockfile remains a reusable project-local persistence concern, not registry state.

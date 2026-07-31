# `sm` compatibility audit: `npx skills@1.5.20`

## Scope

This record tracks functional compatibility between `sm` and `npx
skills@1.5.20`. Functional compatibility covers command semantics, flags,
filesystem effects, exit outcomes, and interactive choices that change an
outcome. Presentation-only wording and picker appearance are out of scope.

Existing documented intentional divergences are excluded from required parity.
Any newly found difference that would change `sm`'s existing user-visible
contract requires explicit approval before implementation.

## Reference artifact

- npm package: `skills@1.5.20`
- SHA-1: `01898927e51692d85da779ec1eb8a032bb3b3065`
- SRI: `sha512-lPl5KzMfTW+qwHFwc8t6R+wAqmdmSHw1+HWbGdJ/FZYbWLdB34bAZNFWiencM5DVoRaKAgXArmfTWMlNAbl9Gg==`

Recreate the reference outside the worktree:

```sh
reference_dir=$(mktemp -d /tmp/sm-skills-reference.XXXXXX)
cd "$reference_dir"
npm pack skills@1.5.20
mkdir package
tar -xzf skills-1.5.20.tgz -C package
```

The audited bundle is `package/package/dist/cli.mjs`. Do not commit this
third-party build artifact.

## Completion rule

Every command and material helper behavior in the reference implementation
must be classified as one of:

- **Aligned** — `sm` has equivalent functional behavior.
- **Intentional divergence** — `sm` deliberately differs, with a rationale.
- **Fixed** — a previous difference removed by a cited `sm` change and test.

## Evidence standard

An **Aligned** classification requires a static code comparison and a
reproducible test reference. Existing coverage may be cited. A newly found or
fixed gap requires a focused regression test. The audit does not require a
full automated differential harness because approved divergences and
environment-dependent interaction would make it misleading and costly.

## Audit order

Audit user-visible commands first: `install`, `update`, `list`, `rm`, and
`use`; then `init`, `find`, `check`, and experimental entry points. For each
command, trace and classify the material helpers that can affect a functional
outcome.

## Status

In progress. The first completed finding is the source-lock discovery filter,
fixed in commit `65c202e`.

## Accepted intentional divergences

The following are pre-existing, approved divergences. They are recorded here
for audit completeness and are reconsidered only when current evidence
contradicts their rationale.

| Reference behavior | `sm` decision and rationale |
| --- | --- |
| GitHub API acquisition | Use generic `git clone`; it avoids GitHub-token and host coupling. |
| skills.sh Search and blob/tree install APIs | `sm browse` and generic clone cover the intended workflows without API-specific behavior. |
| Telemetry and `--metadata` | Omitted because they are telemetry-only. |
| `sanitizeName` directory rewriting | Omitted to preserve backward compatibility; name comparison is case-insensitive instead. |
| Strict frontmatter rejection | `sm` intentionally discovers leniently, falling back to the directory name. |
| Running-agent detection | `sm` detects available agent CLIs, a broader and more useful default. |
| `experimental_sync` | Omitted; it is npm-ecosystem-specific. |
| `experimental_install` | Covered by `sm install --from-lock`. |
| `ensureUniversalAgents` display behavior | Omitted because `sm`'s deduplication model has no functional counterpart. |
| Update deletion prompt | `sm` deliberately retains stale installs. |
| Existing-skill overwrite check | `sm` warns and overwrites. |
| `check` as update alias | `sm check` remains a standalone integrity scan. |
| `init` without arguments | `sm` creates project configuration; skill scaffolding remains available through `sm init <name>` or `sm init .`. |
| `--global` behavior in `rm` and `add` | It selects a registry category; default scope remains project plus global, while `--project` limits scope. |

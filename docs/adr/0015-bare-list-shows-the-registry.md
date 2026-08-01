# Bare list shows the Registry

`sm list` shows the personal Registry inventory by default. `--installed` switches to Installed Skills and composes with project, global, and Agent filters. The former `--registry` flag remains temporarily as a deprecated alias for the default. This supersedes the list-default portion of ADR-0003.

A Registry-centered product must make the user's reusable library visible without an opt-in flag. Installed entities remain operationally important, but they are deployments of Registry originals rather than the primary inventory.

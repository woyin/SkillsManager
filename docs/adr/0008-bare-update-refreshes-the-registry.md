# Bare update refreshes the entire Registry

`sm update` refreshes every updatable original in the personal Registry by default, so all Link Installs across projects observe the refreshed content without visiting each project. Narrow updates based on a project, installed skills, or named skills remain available only through explicit selection. This supersedes the update-default portion of ADR-0003 and makes the command match the Registry-centered product model established by ADR-0007.

We accept that a full Registry Update may contact sources unused by the current project: predictable one-command maintenance of the personal skill library is the intended value. Defaulting to only the current project's Installed Skills would leave other projects silently stale and make the shared Registry indistinguishable from an internal cache.

The command surface is: name arguments select named Registry Skills; `--project [--dir PATH]` selects Skills referenced by that project; `--global` selects Skills referenced by global Agent installs; both scope flags select their union. The former `--registry` flag remains temporarily as a deprecated alias for the new bare default.

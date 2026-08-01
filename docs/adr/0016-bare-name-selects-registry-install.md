# A bare install name selects Registry Install

`sm install <name>` installs that uniquely named Skill from the personal Registry. If the name is absent it fails with a Register hint and never searches or clones remotely. Explicit repository shorthand, URLs, and filesystem paths select Direct Install; a local relative name that could collide with a Registry name must use `./name`. The former `--from-registry` flag remains temporarily as a deprecated alias. This supersedes ADR-0005's requirement for an explicit Registry Install flag.

Requiring a mode flag for the primary reuse workflow makes the Registry look secondary. Source syntax supplies a deterministic boundary without introducing a network fallback, preserving predictable local installation while making the common command concise.

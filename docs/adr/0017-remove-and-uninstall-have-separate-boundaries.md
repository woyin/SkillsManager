# Remove and Uninstall have separate boundaries

`sm uninstall` removes Installed Skill entities from selected project or Agent scopes and never deletes Registry originals. `sm rm <name>` removes the uniquely named Registry original, but refuses while any known project or global installation references it and lists those references. `sm rm <name> --force` removes all known Link Installs and lock entries before deleting the original; inaccessible historical projects are reported explicitly. This supersedes ADR-0003's mixed `rm` behavior.

A cross-project Registry cannot safely infer that scanning only the current project and global Agent directories finds every consumer. Separating deployment removal from library deletion makes the destructive boundary visible and prevents ordinary cleanup from creating broken links elsewhere.

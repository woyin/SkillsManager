# Team Catalogs are Git-first

The first Team Catalog distribution mechanism is a Git Catalog Source. Members register a repository using existing Git URL and SSH authentication, and projects or CI may pin its revision with the established branch, tag, or commit semantics; a hosted service is a later option, not a first-release dependency.

Git already provides the access control, protected branches, review flow, commit signing, and immutable release references that shared skill governance requires. Reusing the Source and Origin lifecycle limits new operational surface while preserving a path to a service if the team later needs centralized discovery or administration.

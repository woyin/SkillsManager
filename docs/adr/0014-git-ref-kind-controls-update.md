# Git ref kind controls update behavior

Git Sources without an explicit ref track the default branch; explicit branch refs track that branch; tag and commit refs are pinned. Registration resolves and stores the ref kind rather than treating every non-empty ref as pinned. If a remote has a branch and tag with the same unqualified name, registration rejects it until the user supplies `refs/heads/...` or `refs/tags/...`.

Users commonly select a branch because they want updates from that line of development, while tags and commits express reproducibility. Preserving only the ref string loses that intent, so Skill Origin records the resolved kind despite the extra provenance schema and migration work.

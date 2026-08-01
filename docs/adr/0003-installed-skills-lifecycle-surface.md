# Installed Skills are the default list/update/rm surface

Status: superseded by ADR-0007, ADR-0008, ADR-0015, and ADR-0017

Bare `sm list` shows Installed Skills (project-first, with global as applicable). Bare `sm update` updates Registry sources for those currently Installed Skills. Bare `sm rm <skill>` uninstalls from the default install scope and removes the Registry original when nothing else references it. Registry inventory is opt-in (`--registry` or equivalent). Skill originals live in the Registry; Agent dirs hold symlinks by default (`--copy` optional).

We rejected agent-dirs-only truth (reimplements lifecycle), list defaulting to Registry (users think install failed), and update-all-registry-always (noisy and mismatched to "what is installed"). Direct default changes are accepted as intentional breakage of older sm semantics.

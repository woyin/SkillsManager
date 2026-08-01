# Profile Install is atomic

Profile Install preflights every referenced Skill and MCP before changing Agent directories, MCP configuration, or project configuration. If any Profile member is unavailable or invalid, the operation fails without installation side effects; otherwise the complete bundle is installed.

A partially installed Profile does not represent the named environment and is difficult to distinguish from success when missing members are only warnings. We accept failing the whole operation—and requiring the user to repair the Registry or Profile first—in exchange for Profiles being dependable, reusable project environments.

Profile create and update apply the same invariant before saving: every referenced Skill and MCP must already exist and resolve uniquely in the Registry. Invalid edits leave the previous Profile unchanged. Cross-machine restore orders Registry content before Profiles rather than persisting knowingly broken Profile definitions.

# Direct Install is the primary path; Registry is the lifecycle store

Day-to-day use is Direct Install: one command from a Source installs into Agent skill directories and records skill originals in the Registry. Register-only (`sm add`) remains for library/curation workflows. Registry is not the user's primary concept; list/update/rm default to Installed Skills and use Registry behind the scenes for originals, updates, and cleanup.

We rejected keeping the Registry as the mental model for daily use, because that fails the "command-behavior parity with npx skills without requiring registry literacy" bar. We also rejected dual primary paths and an npx-compat façade, which would split the command surface and double maintenance cost.

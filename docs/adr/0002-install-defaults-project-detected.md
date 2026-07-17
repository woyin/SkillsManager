# Direct Install defaults: project scope, detected agents, interactive multi-skill

`sm install <source>` defaults to Project Scope, targets Detected Agents, and when a Source has multiple skills with no skill filter: TTY multi-select, non-TTY (or `-y`/`--all`) installs all. Single-skill sources install immediately. If no Detected Agents exist, the command fails with guidance to install an agent or pass `--agent`. Defaults change immediately (no compat mode / major-version gate); previous behavior remains available via explicit flags (`--global`, `--agent`, `--skill`, `--all`).

We rejected default Global Scope (opposite of npx and weak for project reproducibility), defaulting to the full agent catalog or hard-coded claude+codex (wrong or noisy targets), and always requiring explicit skill selection (breaks one-command closure).

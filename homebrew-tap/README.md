# woyin/tap

Homebrew tap for [SkillsManager (sm)](https://github.com/woyin/skills-manager).

## Installation

```bash
brew tap woyin/tap
brew install sm
```

## What is SkillsManager?

A CLI tool for managing AI agent skills (Codex, Claude, Gemini, OpenCode, Hermes, OpenClaw) and MCP server configurations across multiple projects.

```bash
# Add a skill
sm add github.com/user/repo/skill cloudflare

# Install to a project
sm install --profile cloudflare

# Start web dashboard
sm web
```

## Updating

```bash
brew update
brew upgrade sm
```

## Uninstalling

```bash
brew uninstall sm
brew untap woyin/tap
```

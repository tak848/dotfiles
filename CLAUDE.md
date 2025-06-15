# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This is a personal dotfiles repository managed with chezmoi. The repository contains configuration templates that chezmoi applies to the user's home directory.

## Chezmoi File Naming Convention

- `dot_` prefix → `.` when applied (e.g., `dot_zshrc.tmpl` → `~/.zshrc`)
- `.tmpl` suffix → chezmoi template files that support templating

## Key Configuration Files

### Shell Environment (`dot_zshrc.tmpl`)
- Auto-installs essential tools on first run: Homebrew, direnv, aqua, fzf
- Configures Zinit for Zsh plugin management
- Sets up aqua for CLI tool version management
- Defines custom functions: `fbr` (git branch switcher), `fkill` (process killer)

### Package Management (`dot_config/aquaproj-aqua/aqua.yaml`)
Manages development tools via aqua:
- pnpm v10.12.1
- golang go1.24.4
- GitHub CLI v2.74.0
- Node.js v24.1.0

## Common Commands

```bash
# Apply dotfiles to system
chezmoi apply

# Edit a dotfile in the source directory
chezmoi edit ~/.zshrc

# See what changes would be applied
chezmoi diff

# Update aqua packages
aqua update

# After adding/updating packages in aqua.yaml, update checksums
aqua update-checksum
```

## Architecture Notes

The zshrc template implements a self-bootstrapping pattern:
1. Checks for and installs missing dependencies (Homebrew → direnv → aqua → fzf)
2. Configures PATH for aqua-managed binaries: `$(aqua root-dir)/bin`
3. Sets up NPM global directory to avoid permission issues: `$HOME/.local/share/npm-global`

When modifying configurations, always edit the template files in this repository, not the applied files in the home directory.

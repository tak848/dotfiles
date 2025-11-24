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


# AI-DLC and Spec-Driven Development

Kiro-style Spec Driven Development implementation on AI-DLC (AI Development Life Cycle)

## Project Context

### Paths
- Steering: `.kiro/steering/`
- Specs: `.kiro/specs/`

### Steering vs Specification

**Steering** (`.kiro/steering/`) - Guide AI with project-wide rules and context
**Specs** (`.kiro/specs/`) - Formalize development process for individual features

### Active Specifications
- Check `.kiro/specs/` for active specifications
- Use `/kiro:spec-status [feature-name]` to check progress

## Development Guidelines
- Think in English, generate responses in Japanese. All Markdown content written to project files (e.g., requirements.md, design.md, tasks.md, research.md, validation reports) MUST be written in the target language configured for this specification (see spec.json.language).

## Minimal Workflow
- Phase 0 (optional): `/kiro:steering`, `/kiro:steering-custom`
- Phase 1 (Specification):
  - `/kiro:spec-init "description"`
  - `/kiro:spec-requirements {feature}`
  - `/kiro:validate-gap {feature}` (optional: for existing codebase)
  - `/kiro:spec-design {feature} [-y]`
  - `/kiro:validate-design {feature}` (optional: design review)
  - `/kiro:spec-tasks {feature} [-y]`
- Phase 2 (Implementation): `/kiro:spec-impl {feature} [tasks]`
  - `/kiro:validate-impl {feature}` (optional: after implementation)
- Progress check: `/kiro:spec-status {feature}` (use anytime)

## Development Rules
- 3-phase approval workflow: Requirements → Design → Tasks → Implementation
- Human review required each phase; use `-y` only for intentional fast-track
- Keep steering current and verify alignment with `/kiro:spec-status`
- Follow the user's instructions precisely, and within that scope act autonomously: gather the necessary context and complete the requested work end-to-end in this run, asking questions only when essential information is missing or the instructions are critically ambiguous.

## Steering Configuration
- Load entire `.kiro/steering/` as project memory
- Default files: `product.md`, `tech.md`, `structure.md`
- Custom files are supported (managed via `/kiro:steering-custom`)

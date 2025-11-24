# Repository Guidelines

## Project Structure & Module Organization
- `dot_zsh/`: Zsh functions and helpers. Files use kebab-case (e.g., `git-worktree.zsh`).
- `dot_config/`: App configs managed via chezmoi (e.g., `aquaproj-aqua/`, `nvim/`, `mise/`).
- `dot_zshrc.tmpl`, `*.tmpl`: Chezmoi templates rendered per host.
- `aqua.yaml`, `dot_config/aquaproj-aqua/aqua.yaml`: Tool versions via aqua; checksums alongside.
- `dot_claude/`: Jsonnet sources and generated settings for local AI tooling.
- `.github/workflows/`: CI to validate aqua tools and chezmoi templates.
- `Taskfile.yaml`: Tasks to generate files from Jsonnet.

## Build, Test, and Development Commands
- `aqua install`: Install pinned CLI tools (run at repo root; many tasks assume PATH contains `$(aqua root-dir)/bin`).
- `task generate`: Generate all Jsonnet outputs under `dot_claude/`.
- `chezmoi diff` / `chezmoi apply`: Review and apply changes to your home directory.
- `chezmoi --source . --dry-run --verbose apply`: Full local validation without mutating files.
- `aqua update-checksum --prune`: Refresh checksums after editing any `aqua.yaml`.

## Coding Style & Naming Conventions
- Shell/Zsh: 4-space indent, safe `set -euo pipefail` where appropriate, quote variables.
- Function names: short lowercase (e.g., `gwt`, `gwr`, `gwc`).
- File names: kebab-case under `dot_zsh/functions/` (e.g., `git-commit-ai.zsh`).
- Templates: keep logic minimal in `*.tmpl` (Go templates); prefer readable conditionals over cleverness.

## Testing Guidelines
- Validate templates: `chezmoi --source . --dry-run --verbose apply` and `chezmoi diff` before pushing.
- Tooling sanity: `cd dot_config/aquaproj-aqua && aqua install --test`.
- No unit test framework here; keep changes small and reversible.

## Commit & Pull Request Guidelines
- Commit style: Conventional Commits (e.g., `feat:`, `fix:`, `chore(deps):`, `docs:`). Example: `chore(deps): update neovim to v0.11.4`.
- Include generated artifacts when relevant (e.g., `dot_claude/*.json`) and updated `aqua-checksums.json` when `aqua.yaml` changes.
- PRs: explain What/Why, note impacted paths (e.g., `dot_zsh/`, `dot_config/aquaproj-aqua/`), and paste a `chezmoi diff` snippet if user-facing.

## Security & Configuration Tips
- Do not commit secrets. Use local overrides: `~/.zshrc.local`, `~/.zsh/local/*.zsh`, `.envrc.local` (these are git-ignored).
- On new worktrees, run `direnv allow .` (if using direnv) and `aqua policy allow` when prompted.



# AI-DLC and Spec-Driven Development

Kiro-style Spec Driven Development implementation on AI-DLC (AI Development Life Cycle)

## Project Memory
Project memory keeps persistent guidance (steering, specs notes, component docs) so Codex honors your standards each run. Treat it as the long-lived source of truth for patterns, conventions, and decisions.

- Use `.kiro/steering/` for project-wide policies: architecture principles, naming schemes, security constraints, tech stack decisions, api standards, etc.
- Use local `AGENTS.md` files for feature or library context (e.g. `src/lib/payments/AGENTS.md`): describe domain assumptions, API contracts, or testing conventions specific to that folder. Codex auto-loads these when working in the matching path.
- Specs notes stay with each spec (under `.kiro/specs/`) to guide specification-level workflows.

## Project Context

### Paths
- Steering: `.kiro/steering/`
- Specs: `.kiro/specs/`

### Steering vs Specification

**Steering** (`.kiro/steering/`) - Guide AI with project-wide rules and context
**Specs** (`.kiro/specs/`) - Formalize development process for individual features

### Active Specifications
- Check `.kiro/specs/` for active specifications
- Use `/prompts:kiro-spec-status [feature-name]` to check progress

## Development Guidelines
- Think in English, generate responses in Japanese. All Markdown content written to project files (e.g., requirements.md, design.md, tasks.md, research.md, validation reports) MUST be written in the target language configured for this specification (see spec.json.language).

## Minimal Workflow
- Phase 0 (optional): `/prompts:kiro-steering`, `/prompts:kiro-steering-custom`
- Phase 1 (Specification):
  - `/prompts:kiro-spec-init "description"`
  - `/prompts:kiro-spec-requirements {feature}`
  - `/prompts:kiro-validate-gap {feature}` (optional: for existing codebase)
  - `/prompts:kiro-spec-design {feature} [-y]`
  - `/prompts:kiro-validate-design {feature}` (optional: design review)
  - `/prompts:kiro-spec-tasks {feature} [-y]`
- Phase 2 (Implementation): `/prompts:kiro-spec-impl {feature} [tasks]`
  - `/prompts:kiro-validate-impl {feature}` (optional: after implementation)
- Progress check: `/prompts:kiro-spec-status {feature}` (use anytime)

## Development Rules
- 3-phase approval workflow: Requirements → Design → Tasks → Implementation
- Human review required each phase; use `-y` only for intentional fast-track
- Keep steering current and verify alignment with `/prompts:kiro-spec-status`
- Follow the user's instructions precisely, and within that scope act autonomously: gather the necessary context and complete the requested work end-to-end in this run, asking questions only when essential information is missing or the instructions are critically ambiguous.

## Steering Configuration
- Load entire `.kiro/steering/` as project memory
- Default files: `product.md`, `tech.md`, `structure.md`
- Custom files are supported (managed via `/prompts:kiro-steering-custom`)

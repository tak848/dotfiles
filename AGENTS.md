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


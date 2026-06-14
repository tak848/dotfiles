# Global Guidelines

## Language

Always respond in Japanese(常に日本語で答えること).
GitHub に書き込む本文（PR description/body、issue/PR コメント、レビュー返信）も日本語で書くこと。ユーザーが明示的に別言語を指定した場合を除き、PR テンプレートの見出しが英語でも本文は日本語にする。

# Repository Guidelines

## Project Structure & Module Organization
- `dot_zsh/`: Zsh functions and helpers. Files use kebab-case (e.g., `git-worktree.zsh`).
- `dot_config/`: App configs managed via chezmoi (e.g., `aquaproj-aqua/`, `nvim/`, `mise/`).
- `dot_zshrc.tmpl`, `*.tmpl`: Chezmoi templates rendered per host.
- `aqua.yaml`, `dot_config/aquaproj-aqua/aqua.yaml`: Tool versions via aqua; checksums alongside.
- `dot_claude/`: Jsonnet sources and generated settings for local AI tooling.
- `.github/workflows/`: CI to validate aqua tools and chezmoi templates.
- `Taskfile.yaml`: Tasks to generate lockfiles, checksums, and bootstrap script.

## Build, Test, and Development Commands
- `aqua install`: Install pinned CLI tools (run at repo root; many tasks assume PATH contains `$(aqua root-dir)/bin`).
- `task`: Generate lockfiles, checksums, and bootstrap script.
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
- Include generated artifacts when relevant (e.g., `mise.lock`, `aqua-checksums.json`) and run `task` after changing tool configs.
- PRs: explain What/Why, note impacted paths (e.g., `dot_zsh/`, `dot_config/aquaproj-aqua/`), and paste a `chezmoi diff` snippet if user-facing.
- PR description/body は日本語で書くこと。テンプレート見出しが英語でも本文は日本語にし、英語本文をデフォルトにしない。
- **PR 作成時の注意**: GitHub MCP の `body` パラメータに改行を含める際、リテラル `\n` ではなく実際の改行文字を使うこと（リテラル `\n` はエスケープされて壊れる）。
- レビューコメントの指摘に対して修正を行った場合は、必ず該当コメントに reply すること。修正した commit へのリンク（`https://github.com/{owner}/{repo}/commit/{sha}` 形式）を含めること。
- issue/PR にコメント・返信する際は、本文末尾に `(by <agent name>)` を付与すること（例: `(by Codex)`, `(by Claude Code)`）。

## Security & Configuration Tips
- Do not commit secrets. Use local overrides: `~/.zshrc.local`, `~/.zsh/local/*.zsh`, `.envrc.local` (these are git-ignored).
- On new worktrees, run `direnv allow .` (if using direnv) and `aqua policy allow` when prompted.

## このリポジトリで作業するエージェントへの厳守事項
- **変更が一段落したら、確認を待たず commit → push → draft PR まで一気に完遂する**（commit で止めない）。PR は draft で作成する。
- `git commit` の前に必ず現在のブランチを確認する（`git branch --show-current`）。plan mode から戻った後や PR マージ後は main に戻っている可能性が高い。
- main に直接 commit しない。main から新しいブランチを切る。push の前にリモートブランチの状態を確認する（`git ls-remote` 等）。マージ済みブランチには push しない・不要なリモートブランチを散らかさない。
- ユーザーが別 repo / 機構を参照したら、default branch だけで「無い」と判断せず `git branch -a` / `git log --all` で他ブランチも確認してから答える。
- **要求スコープを厳守する。** ユーザーが「A の代替として B を作る」等と明示したら、その範囲だけに絞る。隣接レイヤー（前段・後段・類似機能）の改修を勝手に計画へ足さない。関連改善は本線の計画を立てた上で別途質問する。
- **環境を直接変更しない。** 変更は必ずこの repo のソース（`dot_` プレフィックス付きファイル等）を編集し、PR 経由で行う。`~/.local/share/chezmoi` 等の repo 外パスや、`~/.claude/` `~/.codex/` `~/.config/` 等のターゲットファイルを直接書き換えてはならない。環境への適用は `chezmoi update` に委ねる（remote main が single source of truth）。
- **chezmoi ソースを編集する。** `~/.codex/AGENTS.md` 等のターゲットではなく `dot_codex/AGENTS.md` 等のソースを編集する。ターゲットを直接編集しても `chezmoi update` で上書きされ、PR にも含められない。
- **ツール導入手段として Homebrew を提案しない**（`packages.yaml` への追加・`brew install` を選択肢に挙げない）。mise（aqua / github / go / npm backend）または aqua CLI で完結させる。
- mise にツールを追加する際、`mise search` / `mise registry` で見つからなくても [aqua-registry](https://github.com/aquaproj/aqua-registry/tree/main/pkgs) に定義があれば `"aqua:<registry path>" = "<version>"` で追加できる（Renovate 自動更新・checksum 検証に乗る）。`http` backend で URL を手書きするのは aqua-registry にも無い最終手段のみ。
- 環境変数（API key 等）の置き場所を勝手に特定ファイル（`.zshrc.local` 等）に指定しない。置き場所はユーザーに委ねる（エラーメッセージやコメントにも特定ファイル名を書かない）。
- `~/.codex/config.toml` 等の modify テンプレート（`dot_codex/modify_config.toml`）は、出力で再出力しないキーを `chezmoi apply` 時に削除する。ツールが書き込む既存キー（`projects` / `notice` / `hooks.state` 等）は保持ブロックに追加すること。



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

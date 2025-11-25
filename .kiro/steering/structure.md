# Project Structure

## Organization Philosophy

Chezmoi のファイル命名規則に従い、ホームディレクトリ構造をミラーリング。`dot_` プレフィックスで `.` 始まりのファイル・ディレクトリを表現。

## Directory Patterns

### Chezmoi ソースファイル
**Location**: `/` (リポジトリルート)
**Purpose**: chezmoi が管理する dotfiles のテンプレート
**Naming**:
- `dot_` → ホームの `.` (例: `dot_zshrc.tmpl` → `~/.zshrc`)
- `.tmpl` サフィックス → chezmoi テンプレート（変数展開可能）
- `once_` プレフィックス → 初回のみ適用（ローカル設定の雛形）

**Example**:
```
dot_zshrc.tmpl          → ~/.zshrc
dot_config/             → ~/.config/
dot_zsh/functions/      → ~/.zsh/functions/
```

### パッケージ管理設定
**Location**: `dot_config/aquaproj-aqua/`
**Purpose**: aqua による CLI ツールの宣言的管理
**Files**:
- `aqua.yaml`: パッケージリスト（バージョン固定）
- `aqua-checksums.json`: バイナリのチェックサム（自動生成）

### Zsh 関数
**Location**: `dot_zsh/functions/`
**Purpose**: シェル関数の整理（共有可能な関数）
**Pattern**: 機能ごとに独立した `.zsh` ファイル

**Example**:
- `git-worktree.zsh`: Git Worktree 管理関数（`gwt`, `gwc`, `gwr`）
- `git-commit-ai.zsh`: AI による Git コミットメッセージ生成

### AI ツール設定
**Location**: `dot_claude/`, `dot_gemini/`
**Purpose**: Claude Code / Gemini の設定管理
**Pattern**:
- Jsonnet ソース（`settings.jsonnet`）→ JSON 生成（`settings.json`）
- Task ランナーで自動変換: `task generate`

### ワークフロー
**Location**: `.github/workflows/`
**Purpose**: CI/CD の自動化
**Files**:
- `aqua-checksums.yaml`: aqua チェックサム更新の自動化
- `ci.yaml`: 構文チェック等

### タスク管理
**Location**: `Taskfile.yaml`
**Purpose**: Task ランナーによるコマンドの標準化
**Example**:
```bash
task generate  # Jsonnet → JSON 変換
```

## Naming Conventions

- **Dotfiles**: `dot_` プレフィックス + テンプレートは `.tmpl` サフィックス
- **Zsh 関数**: `kebab-case.zsh`（例: `git-worktree.zsh`）
- **設定ファイル**: `settings.jsonnet` / `settings.json`（生成物）

## Import Organization

### Zsh での読み込み
```zsh
# プラグイン管理: Zinit
source "${ZINIT_HOME}/zinit.zsh"

# 関数の自動読み込み: fpath
fpath=(~/.zsh/functions $fpath)
autoload -Uz git-worktree  # 関数名で自動ロード

# ローカル設定（存在する場合のみ）
[ -f ~/.zshrc.local ] && source ~/.zshrc.local
```

### Aqua でのパス設定
```zsh
export PATH="$(aqua root-dir)/bin:$PATH"
```

## Code Organization Principles

- **テンプレート vs 静的ファイル**: 環境依存の設定は `.tmpl` でテンプレート化
- **ローカル設定の分離**: `.local` サフィックスまたは `local/` ディレクトリで Git 管理外
- **宣言的管理**: ツールのバージョンは `aqua.yaml` で一元管理
- **自動化優先**: Jsonnet/Task ランナーで手動作業を排除

---
_Document patterns, not file trees. New files following patterns shouldn't require updates_

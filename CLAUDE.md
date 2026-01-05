# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Chezmoi で管理している個人用 dotfiles リポジトリ。macOS/Linux 対応。

## Commands

```bash
# 設定を適用
chezmoi apply

# 変更差分を確認
chezmoi diff

# 管理ファイル一覧
chezmoi managed

# mise ツールをインストール
mise install

# mise lockfile を更新
mise lock

# aqua ツールをインストール（mise 管理外のツール用）
aqua install

# aqua checksum を更新（ツール追加/更新後）
aqua update-checksum --prune
```

## Architecture

### ツール管理の構成

```
Homebrew
  └─ 基本パッケージ

~/.local/bin/mise (bootstrap スクリプト)
  └─ mise (統合ツール管理)
       ├─ ランタイム管理: go, node, pnpm (core backend)
       ├─ CLI ツール管理: fzf, ripgrep, starship, etc. (aqua backend)
       ├─ npm グローバルパッケージ
       └─ aqua CLI (github backend)
            └─ mise lock で checksum 取得不可のツール
                 └─ aws-cli, 1password/cli, zoxide
```

- **mise** (`dot_config/mise/`, `dot_local/bin/executable_mise`): ランタイム、CLI ツール、npm パッケージの統合管理。bootstrap 方式でインストール
- **aqua** (`dot_config/aquaproj-aqua/`): mise lock で checksum が取得できないツールを管理。aqua CLI 自体は mise でインストール

### Chezmoi ファイル命名規則

| プレフィックス | 展開先 | 例 |
|--------------|--------|-----|
| `dot_` | `~/.` | `dot_zshrc.tmpl` → `~/.zshrc` |
| `executable_` | 実行権限付与 | `executable_mise` → `mise` (+x) |
| `.tmpl` | テンプレート展開 | OS/アーキテクチャ分岐 |
| `run_once_before_*` | 初回のみ実行 | Homebrew インストール |
| `run_onchange_after_*` | ファイル変更時実行 | mise install |

### 自動更新ワークフロー（Renovate + GitHub Actions）

| ワークフロー | トリガー | 処理 |
|-------------|---------|------|
| `mise-lock.yaml` | `mise.toml` / `config.toml` 変更 | `mise lock` |
| `mise-bootstrap.yaml` | `.mise-bootstrap-version` 変更 | `mise generate bootstrap` |
| `aqua-checksums.yaml` | `aqua.yaml` 変更 | `aqua update-checksum --prune` |

### zsh 設定の読み込み順序

```
~/.zshenv      # PATH, 環境変数（非インタラクティブ含む）
~/.zprofile    # mise shims（IDE 連携用）
~/.zshrc       # インタラクティブ設定、プラグイン、エイリアス
~/.zshrc.local # マシン固有設定（Git 管理外）
```

## Conventions

- コミットメッセージは日本語、`feat:`, `fix:`, `chore:` などのプレフィックス必須
- Renovate PR への push には GitHub App Token が必要（GITHUB_TOKEN では不可）
- checksum/lockfile は手動編集しない（ワークフローで自動更新）

## AI-DLC / Spec-Driven Development

Kiro-style Spec Driven Development を使用可能。

- **Steering** (`.kiro/steering/`): プロジェクト全体のルールとコンテキスト
- **Specs** (`.kiro/specs/`): 個別機能の仕様策定

ワークフロー: `/kiro:spec-init` → `/kiro:spec-requirements` → `/kiro:spec-design` → `/kiro:spec-tasks` → `/kiro:spec-impl`

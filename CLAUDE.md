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

# aqua ツールをインストール（mise 管理外のツール用）
aqua install
```

## Taskfile（自動生成ファイル管理）

自動生成ファイル（mise.lock, aqua-checksums, mise bootstrap）を統一的に管理するための Task ランナー。
jsonnet からの JSON 生成は `chezmoi apply` 時に run_onchange スクリプトで実行される。

### 基本コマンド

```bash
# 全ての自動生成を実行（lockfiles + checksums + bootstrap）
task

# lockfiles/checksums/bootstrap のみ更新
task lock

# 個別タスク
task mise:lock      # mise lockfile のみ
task aqua:checksum  # aqua checksum のみ
task mise:bootstrap # bootstrap のみ

# CI用: 生成後に diff チェック（mise.lock, JSON 除外）
task check
```

### 設計方針

| 項目 | 説明 |
|------|------|
| `root: true` | サブディレクトリからも task コマンドを実行可能 |
| `method: checksum` | ファイル内容が変わった時のみ再実行（キャッシュ） |
| `-p PLATFORMS` | mise lock で全プラットフォームを明示的に指定 |

### mise.lock の既知の問題

mise.lock は実行環境（macOS/Linux）によって結果が異なる[既知の問題](https://github.com/jdx/mise/discussions/6942)がある。そのため：

- **CI の diff チェックからは除外**（`task check` は mise.lock を検証しない）
- **mise.lock の更新は Renovate ワークフロー（mise-lock.yaml）に任せる**

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

### 自動生成ファイル一覧

| ファイル | 生成元 | 生成コマンド |
|---------|--------|-------------|
| `mise.lock` | `.mise.toml` | `task mise:lock` |
| `dot_config/mise/mise.lock` | `dot_config/mise/config.toml` | `task mise:lock` |
| `dot_config/aquaproj-aqua/aqua-checksums.json` | `dot_config/aquaproj-aqua/aqua.yaml` | `task aqua:checksum` |
| `dot_local/bin/executable_mise` | `.mise-bootstrap-version` | `task mise:bootstrap` |

### chezmoi apply 時の自動実行スクリプト

`chezmoi apply` 時にターゲットディレクトリで実行されるスクリプト。リポジトリ内にはファイルを生成しない。

| スクリプト | トリガー | 処理 |
|-----------|---------|------|
| `run_once_before_00-install-essentials.sh.tmpl` | 初回のみ | Homebrew, zinit, cursor-agent インストール |
| `run_onchange_after_10-mise-install.sh.tmpl` | `config.toml` 変更時 | `mise install` |
| `run_onchange_after_20-aqua-install.sh.tmpl` | `aqua.yaml` 変更時 | `aqua install` |
| `run_onchange_after_30-install-packages.sh.tmpl` | `packages.yaml` 変更時 | `brew install`（macOS） |
| `run_onchange_after_40-generate-jsonnet.sh.tmpl` | jsonnet ファイル変更時 | jsonnet → JSON 生成（`~/.claude/`, `~/.gemini/`） |

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
| `ci.yaml` | push/PR | `task check` で自動生成ファイルの diff チェック |
| `mise-lock.yaml` | `mise.toml` / `config.toml` 変更 | `mise lock`（Renovate PR 時のみ） |
| `mise-bootstrap.yaml` | `.mise-bootstrap-version` 変更 | `mise generate bootstrap`（Renovate PR 時のみ） |
| `aqua-checksums.yaml` | `aqua.yaml` 変更 | `aqua update-checksum --prune`（Renovate PR 時のみ） |
| `lazy-lock.yaml` | nvim 設定変更 / 週次 cron | Lazy.nvim lockfile 更新（PR 作成 or Renovate PR へコミット） |

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
- 自動生成ファイルは手動編集しない（`task` または Renovate ワークフローで自動更新）

## AI-DLC / Spec-Driven Development

Kiro-style Spec Driven Development を使用可能。

- **Steering** (`.kiro/steering/`): プロジェクト全体のルールとコンテキスト
- **Specs** (`.kiro/specs/`): 個別機能の仕様策定

ワークフロー: `/kiro:spec-init` → `/kiro:spec-requirements` → `/kiro:spec-design` → `/kiro:spec-tasks` → `/kiro:spec-impl`

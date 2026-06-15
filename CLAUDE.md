# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Chezmoi で管理している個人用 dotfiles リポジトリ。macOS/Linux 対応。

**重要: このリポジトリの remote main が single source of truth。** ターゲットマシンでは `chezmoi update` でリモートから pull & apply する運用のみ。ローカルの chezmoi source directory とこのリポジトリは直接リンクしていないため、このリポジトリ内で `chezmoi cat` / `chezmoi diff` / `chezmoi apply` 等を実行しても意味がない。

## Commands

```bash
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
- **mise.lock の更新は GitHub Actions ワークフロー（`lockfiles-and-checksums.yaml`）に任せる**

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

| ファイル | 生成元 | 生成方法 |
|---------|--------|---------|
| `mise.lock` | `.mise.toml` | `task mise:lock` |
| `dot_config/mise/mise.lock` | `dot_config/mise/config.toml` | `task mise:lock` |
| `dot_config/aquaproj-aqua/aqua-checksums.json` | `dot_config/aquaproj-aqua/aqua.yaml` | `task aqua:checksum` |
| `dot_local/bin/executable_mise` | `.mise-bootstrap-version` | `task mise:bootstrap` |

### chezmoi apply 時の自動管理ファイル

| ターゲットファイル | 生成元 | 方式 |
|------------------|--------|------|
| `~/.claude/settings.json` | `dot_claude/settings.jsonnet` | jsonnet 全体生成（run_onchange） |
| `~/ccgate.libsonnet` | `ccgate.libsonnet` | Claude/Codex 共通の ccgate ルール |
| `~/.gemini/antigravity-cli/settings.json` | `dot_gemini/antigravity-cli/settings.jsonnet` | jsonnet 全体生成（run_onchange） |
| `~/.gemini/antigravity-cli/mcp_config.json` | `dot_gemini/antigravity-cli/mcp_config.jsonnet` | jsonnet 全体生成（run_onchange） |
| `~/.claude.json` | `modify_dot_claude.json` | chezmoi modify テンプレート（差分適用） |
| `~/.codex/config.toml` | `dot_codex/modify_config.toml` | chezmoi modify テンプレート（差分適用） |
| `~/.codex/ccgate.jsonnet` | `dot_codex/ccgate.jsonnet` | Codex PermissionRequest 補助判定ルール |

`~/.claude.json` と `~/.codex/config.toml` はツールが自動的に書き込むため、jsonnet で全体生成せず modify テンプレートで管理対象キーのみ差分適用する。

### Claude Code カスタムマーケットプレイス

dotfiles リポジトリ自体がカスタムマーケットプレイス (`tak848-plugins`) として機能する。MCP サーバーは `claude-plugins/` 配下にプラグインとして定義。

| ファイル | 役割 |
|---------|------|
| `.claude-plugin/marketplace.json` | マーケットプレイス定義 |
| `claude-plugins/{name}/.claude-plugin/plugin.json` | プラグインメタデータ |
| `claude-plugins/{name}/.mcp.json` | MCP サーバー設定 |
| `dot_claude/settings.jsonnet` の `extraKnownMarketplaces` | マーケットプレイス登録 |
| `dot_claude/settings.jsonnet` の `enabledPlugins` | プラグイン有効化 |

### chezmoi apply 時の自動実行スクリプト

`chezmoi apply` 時にターゲットディレクトリで実行されるスクリプト。リポジトリ内にはファイルを生成しない。

| スクリプト | トリガー | 処理 |
|-----------|---------|------|
| `run_once_before_00-install-essentials.sh.tmpl` | 初回のみ | Homebrew, zinit, cursor-agent インストール |
| `run_onchange_after_10-mise-install.sh.tmpl` | `config.toml` 変更時 | `mise install` |
| `run_onchange_after_20-aqua-install.sh.tmpl` | `aqua.yaml` 変更時 | `aqua install` |
| `run_onchange_after_30-install-packages.sh.tmpl` | `packages.yaml` 変更時 | `brew install`（macOS） |
| `run_onchange_after_40-generate-jsonnet.sh.tmpl` | jsonnet ファイル変更時 | jsonnet → JSON 生成（`~/.claude/settings.json`, `~/.gemini/antigravity-cli/{settings,mcp_config}.json`） |
| `run_onchange_after_50-claude-plugins.sh.tmpl` | プラグイン定義変更時 | `claude plugin marketplace update` + `install` |

### Chezmoi ファイル命名規則

| プレフィックス | 展開先 | 例 |
|--------------|--------|-----|
| `dot_` | `~/.` | `dot_zshrc.tmpl` → `~/.zshrc` |
| `executable_` | 実行権限付与 | `executable_mise` → `mise` (+x) |
| `.tmpl` | テンプレート展開 | OS/アーキテクチャ分岐 |
| `modify_` | 既存ファイルを差分適用 | `modify_dot_claude.json` → `~/.claude.json` |
| `run_once_before_*` | 初回のみ実行 | Homebrew インストール |
| `run_onchange_after_*` | ファイル変更時実行 | mise install |

### 自動更新ワークフロー（Renovate + GitHub Actions）

| ワークフロー | トリガー | 処理 |
|-------------|---------|------|
| `ci.yaml` | push | `task check` で自動生成ファイルの diff チェック |
| `lockfiles-and-checksums.yaml` | push（`.mise.toml` / `dot_config/mise/config.toml` / `dot_config/aquaproj-aqua/aqua.yaml` 変更時、`main` / `lazy-lock-update` ブランチを除く） | `mise lock`（ルート + `dot_config/mise/`）と `aqua update-checksum --prune` を Renovate PR ブランチへ commit |
| `mise-bootstrap.yaml` | push（`.mise-bootstrap-version` 変更時） | `mise generate bootstrap` |
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

### このリポジトリで作業するエージェントへの厳守事項

- **変更が一段落したら、確認を待たず commit → push → draft PR まで一気に完遂する**（commit で止めない）。PR は draft で作成する
- `git commit` の前に必ず現在のブランチを確認する（`git branch --show-current`）。plan mode から戻った後や PR マージ後は main に戻っている可能性が高い
- main に直接 commit しない。main から新しいブランチを切る。push の前にリモートブランチの状態を確認する（`git ls-remote` 等）。マージ済みブランチには push しない・不要なリモートブランチを散らかさない
- ユーザーが別 repo / 機構を参照したら、default branch だけで「無い」と判断せず `git branch -a` / `git log --all` で他ブランチも確認してから答える
- **要求スコープを厳守する。** ユーザーが「A の代替として B を作る」等と明示したら、その範囲だけに絞る。隣接レイヤー（前段・後段・類似機能）の改修を勝手に計画へ足さない。関連改善は本線の計画を立てた上で別途質問する
- **環境を直接変更しない。** 変更は必ずこの repo のソース（`dot_` プレフィックス付きファイル等）を編集し、PR 経由で行う。`~/.local/share/chezmoi` 等の repo 外パスや、`~/.claude/` `~/.codex/` `~/.config/` 等のターゲットファイルを直接書き換えてはならない。環境への適用は `chezmoi update` に委ねる（remote main が single source of truth）
- **chezmoi ソースを編集する。** `~/.claude/CLAUDE.md` 等のターゲットではなく `dot_claude/CLAUDE.md` 等のソースを編集する。ターゲットを直接編集しても `chezmoi update` で上書きされ、PR にも含められない
- **ツール導入手段として Homebrew を提案しない**（`packages.yaml` への追加・`brew install` を選択肢に挙げない）。mise（aqua / github / go / npm backend）または aqua CLI で完結させる
- mise にツールを追加する際、`mise search` / `mise registry` で見つからなくても [aqua-registry](https://github.com/aquaproj/aqua-registry/tree/main/pkgs) に定義があれば `"aqua:<registry path>" = "<version>"` で追加できる（Renovate 自動更新・checksum 検証に乗る）。`http` backend で URL を手書きするのは aqua-registry にも無い最終手段のみ
- 環境変数（API key 等）の置き場所を勝手に特定ファイル（`.zshrc.local` 等）に指定しない。置き場所はユーザーに委ねる（エラーメッセージやコメントにも特定ファイル名を書かない）
- `~/.codex/config.toml` 等の modify テンプレート（`dot_codex/modify_config.toml`）は、出力で再出力しないキーを `chezmoi apply` 時に削除する。ツールが書き込む既存キー（`projects` / `notice` / `hooks.state` 等）は保持ブロックに追加すること

## AI-DLC / Spec-Driven Development

Kiro-style Spec Driven Development を使用可能。

- **Steering** (`.kiro/steering/`): プロジェクト全体のルールとコンテキスト
- **Specs** (`.kiro/specs/`): 個別機能の仕様策定

ワークフロー: `/kiro:spec-init` → `/kiro:spec-requirements` → `/kiro:spec-design` → `/kiro:spec-tasks` → `/kiro:spec-impl`

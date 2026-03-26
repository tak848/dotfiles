# 調査ログ & 設計判断

---
**目的**: 発見フェーズで得た事実、調査結果、トレードオフを記録し、`design.md` の判断根拠を残す。
---

## Summary
- **Feature**: `lazyvim-chezmoi-config`
- **Discovery Scope**: Extension / Complex Integration（既存の chezmoi 管理に LazyVim を統合）
- **Key Findings**:
  - LazyVim は公式スターター構成（`init.lua`, `lua/config/*`, `lua/plugins/*`）を前提としており、Neovim は **0.11.2 以上**が必要。リポジトリの aqua ピン（v0.11.5）で満たす。
  - LazyVim のプラグイン管理は `lazy.nvim` に依存し、再現性は `lazy-lock.json` のコミットで担保するのが推奨。
  - 現状 `dot_config/nvim/` は `init.vim.tmpl` のみ。LazyVim 導入には Lua エントリポイントへの移行と、chezmoi テンプレートでの OS 分岐を組み込んだスターターの vendoring が最小摩擦。

## Research Log

### LazyVim の前提・互換性
- **Context**: 要件 5.4（Neovim バージョン互換）および 1.2（初回起動時の自動インストール）を満たすため、最新仕様を確認。
- **Sources Consulted**: LazyVim 公式ドキュメント / GitHub README。
- **Findings**:
  - LazyVim は Neovim 0.11.2+ を要求（2025 年時点の最新要件）。
  - 公式スターター（`LazyVim/starter`）をベースにすると、`init.lua` で `lazy.nvim` と LazyVim 本体をロードする標準構成になる。
- **Implications**:
  - `dot_config/aquaproj-aqua/aqua.yaml` の Neovim ピン v0.11.5 を維持/更新する設計にする。
  - Neovim 初回起動フローは LazyVim 標準の同期に委ね、chezmoi 側での追加ブートストラップは不要。

### lazy.nvim と `lazy-lock.json`
- **Context**: 要件 2.4（ロックファイル管理）と冪等性の確保。
- **Sources Consulted**: `lazy.nvim` 公式ドキュメント。
- **Findings**:
  - `lazy-lock.json` はプラグインのコミット SHA/バージョンを固定し、同一環境の再現に利用される。
  - ロックは `:Lazy update` / LazyVim の更新操作で更新され、更新後にファイルをコミットする運用が一般的。
- **Implications**:
  - ロックファイルは repo 管理下に置き、chezmoi apply で常に同一内容を配置する。
  - 更新責務は「Neovim 側の更新操作 → `lazy-lock.json` 反映 → Git で追従」とする。

### chezmoi 統合方式
- **Context**: 要件 1.x / 2.x / 4.x の統合と、既存 dotfiles パターンの尊重。
- **Sources Consulted**: 既存リポジトリ構造（`dot_config/nvim/`）、chezmoi テンプレート利用実績。
- **Findings**:
  - `dot_config/nvim/` 配下は `.tmpl` で環境依存を吸収するのが既存パターン。
  - 現状 Neovim 設定は VimScript のみで、Lua への移行が必要。
- **Implications**:
  - LazyVim スターターを **ファイルとして vendoring** し、必要箇所のみ `.tmpl` 化して OS 分岐・パス抽象化を実現する。
  - ユーザーのローカルカスタムは chezmoi 管理外のパス（例: `lua/plugins/local/`）に置く方針で保持する。

## Architecture Pattern Evaluation

| Option | Description | Strengths | Risks / Limitations | Notes |
|--------|-------------|-----------|---------------------|-------|
| Vendored Starter | `LazyVim/starter` を repo に取り込み、chezmoi で同期 | オフライン/冪等/レビュー可能、既存運用に合う | スターター更新は手動で追従が必要 | 要件 1,2,4,5 に最も整合 |
| Apply 時に git clone | chezmoi hook 等で `LazyVim/starter` を都度 clone | upstream 追従が容易 | ネットワーク必須、冪等性や失敗時復旧が複雑 | 要件 2（安全な反復適用）に弱い |
| Git submodule | スターターを submodule として参照 | upstream 追従と差分管理が容易 | submodule 運用の追加コスト | dotfiles 既存運用と不一致 |

## Design Decisions

### Decision: Lua エントリポイントへ移行しスターターに寄せる
- **Context**: LazyVim の標準構成は `init.lua` 前提。既存は `init.vim.tmpl` のみ。
- **Alternatives Considered**:
  1. `init.vim` に Lua を読み込ませる互換レイヤーを追加
  2. `init.lua` を新規作成しスターターへ移行
- **Selected Approach**: `init.lua(.tmpl)` を採用し、既存の基本設定は `lua/config/options.lua` 等へ移植。
- **Rationale**: upstream との乖離を最小化し、将来の更新が容易。
- **Trade-offs**: 初回導入時に設定の移行作業が発生。
- **Follow-up**: 既存設定のうち継承すべき項目を実装時に精査。

### Decision: ローカル専用カスタム領域を chezmoi 管理外にする
- **Context**: 要件 2.2 / 4.4。
- **Alternatives Considered**:
  1. repo 内に `lua/plugins/local/` を含める（管理対象）
  2. `lua/plugins/local/` を管理外にし、存在すれば自動ロードする
- **Selected Approach**: 管理外（ユーザー自由領域）とし、LazyVim のプラグインロードで自動的に拾える構造にする。
- **Rationale**: `chezmoi apply` の反復でもユーザー変更が消えない。
- **Trade-offs**: カスタムの内容は Git 追跡されない。
- **Follow-up**: 読み込み優先順位と衝突回避を実装時に確認。

### Decision: `lazy-lock.json` をリポジトリで固定管理する
- **Context**: 要件 2.4 と環境再現性。
- **Alternatives Considered**:
  1. ロックファイルを追跡せず、都度最新へ同期
  2. ロックを追跡し、更新時のみ手動で反映
- **Selected Approach**: 追跡して固定。更新は Neovim 側操作で行い、結果をコミット。
- **Rationale**: セットアップの再現性とロールバック容易性を確保。
- **Trade-offs**: 更新頻度に応じたメンテが必要。
- **Follow-up**: 更新手順をドキュメント化。

## Risks & Mitigations
- **upstream 更新追従の負荷** — LazyVim の更新は定期的にスターター差分を取り込み、必要なら `lazy-lock.json` を更新する運用にする。
- **既存 `init.vim` 設定との挙動差** — 重要設定のみ Lua 側へ移行し、差分が出た場合は Non-Goal を超える変更として切り分ける。
- **OS 依存設定の衝突** — chezmoi テンプレートで OS 判定し、Lua 側は共通設定に限定する。

## References
- LazyVim docs: Installation / Starter structure / Requirements
  - https://www.lazyvim.org/
  - https://github.com/LazyVim/LazyVim
  - https://github.com/LazyVim/starter
- lazy.nvim docs (lockfile)
  - https://lazy.folke.io/

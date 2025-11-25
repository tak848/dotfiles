# Product Overview

個人用の dotfiles リポジトリで、chezmoi を使用してシェル環境とツール群を統合管理します。

## Core Capabilities

- **自動ブートストラップ**: 初回起動時に開発環境を自動セットアップ
- **宣言的パッケージ管理**: aqua による CLI ツールのバージョン固定管理
- **テンプレートベース設定**: chezmoi テンプレートによる環境別の自動設定適用
- **Git Worktree 統合**: ブランチ管理を効率化する専用関数群
- **ローカル設定分離**: Git 管理外のマシン固有設定をサポート

## Target Use Cases

- 新しいマシンでの開発環境の迅速なセットアップ
- 複数マシン間での一貫した開発環境の維持
- CLI ツールのバージョン管理と再現性の確保
- チーム共有しない個人設定の安全な管理

## Value Proposition

chezmoi + aqua + Zsh の組み合わせにより、宣言的かつ再現可能な開発環境を実現。テンプレート機能で環境差を吸収し、ローカル設定の分離でセキュリティを担保します。

---
_Focus on patterns and purpose, not exhaustive feature lists_

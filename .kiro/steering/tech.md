# Technology Stack

## Architecture

シェル環境をテンプレート化し、宣言的な設定ファイル（YAML/Jsonnet）でツールとパッケージを管理するアプローチ。

## Core Technologies

- **Shell**: Zsh（Emacs モード、履歴共有）
- **Dotfile Manager**: chezmoi（テンプレート、暗号化サポート）
- **Package Manager**:
  - aqua（CLI ツールのバージョン固定）
  - Homebrew（システムレベルパッケージ）
- **Plugin Manager**: Zinit（Zsh プラグイン管理）

## Key Tools

### 開発ツール（aqua 管理）
- **Node.js**: v24.11.1
- **pnpm**: v10.23.0
- **Go**: go1.25.4
- **Python**: uv（パッケージマネージャー）
- **Neovim**: v0.11.5

### CLI ユーティリティ
- **fzf**: インタラクティブフィルタ（ブランチ選択等で使用）
- **starship**: プロンプトテーマ
- **direnv**: ディレクトリ別環境変数管理
- **ripgrep**: 高速 grep 代替
- **jq/yq**: JSON/YAML 処理

## Development Standards

### ファイル構成
- テンプレートファイル: `.tmpl` サフィックス
- Jsonnet による設定生成: Task ランナーで自動変換
- チェックサム管理: aqua-checksums.json でバイナリの整合性検証

### ツールバージョン管理
- aqua.yaml でバージョンを明示的に固定
- チェックサム必須（darwin/linux）
- `aqua update-checksum` でチェックサム更新

### Chezmoi 規約
- `dot_` プレフィックス → ホームディレクトリの `.` に変換
- 一度限りのファイル: `once_` プレフィックス（ローカル設定テンプレート）
- `modify_` スクリプトで動的な設定変更

## Development Environment

### Required Tools
- chezmoi（dotfile 適用）
- aqua（CLI ツール管理）
- Homebrew（macOS の場合）

### Common Commands
```bash
# Dotfiles 適用
chezmoi apply

# Dotfiles 編集
chezmoi edit ~/.zshrc

# 差分確認
chezmoi diff

# Aqua パッケージ更新
aqua update
aqua update-checksum

# Jsonnet から JSON 生成
task generate
```

## Key Technical Decisions

- **aqua + chezmoi 連携**: aqua で CLI ツールを固定し、chezmoi でシェル設定と統合。環境の再現性を最大化。
- **Jsonnet による設定生成**: Claude Code/Gemini の設定を Jsonnet で管理し、JSON を生成。変更の追跡が容易。
- **ローカル設定の分離**: `~/.zshrc.local` や `~/.zsh/local/` で Git 管理外の個人設定をサポート。
- **自動ブートストラップ**: .zshrc 内で依存ツールの存在チェックと自動インストール。

---
_Document standards and patterns, not every dependency_

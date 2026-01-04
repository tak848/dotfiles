# dotfiles

chezmoi で管理している個人用 dotfiles リポジトリです。macOS / Linux 対応。

## セットアップ

### 事前条件（macOS）

macOS では、Xcode Command Line Tools が必要です：

```bash
xcode-select --install
```

インストール完了を待ってから、次のステップに進んでください。

### 初回インストール

```bash
sh -c "$(curl -fsLS get.chezmoi.io)" -- init --apply tak848
```

### 既存環境での更新

```bash
chezmoi update
```

## ツール管理

### 構成

```
Homebrew
  └─ aqua (CLI ツール管理)
       └─ go, fzf, ripgrep, starship, direnv, etc.

~/.local/bin/mise (bootstrap スクリプト)
  └─ mise (ランタイム管理)
       └─ node, pnpm, npm グローバルパッケージ
```

### 今後の予定

- aqua → mise への統合（CLI ツールも mise で管理）
- Homebrew の宣言的管理（Brewfile など）

## 主な機能

### 自動セットアップ

初回起動時に以下を自動インストール：

- Homebrew
- aqua（CLI ツールバージョン管理）
- mise（ランタイム管理、bootstrap 方式）
- zinit（zsh プラグインマネージャー）

### Git Worktree 管理関数

- `gwt` - worktree 間の移動
- `gwc` - 新規 worktree の作成
- `gwr` - worktree の削除

### ローカル設定（Git 管理外）

- `~/.zshrc.local` - マシン固有の環境変数やエイリアス
- `~/.zsh/local/*.zsh` - ローカル関数

## ディレクトリ構造

```
~/
├── .local/bin/
│   └── mise              # mise bootstrap スクリプト
├── .config/
│   ├── aquaproj-aqua/    # aqua 設定
│   │   ├── aqua.yaml
│   │   └── aqua-checksums.json
│   └── mise/             # mise 設定
│       ├── config.toml
│       └── mise.lock
├── .zsh/
│   ├── functions/        # 共有関数（Git 管理）
│   └── local/            # ローカル関数（Git 管理外）
├── .zshenv               # PATH, 環境変数
├── .zprofile             # mise shims（IDE 連携）
├── .zshrc                # インタラクティブ設定
└── .zshrc.local          # マシン固有設定（Git 管理外）
```

## カスタマイズ

### ツールの追加

**aqua（CLI ツール）:**

```bash
# dot_config/aquaproj-aqua/aqua.yaml に追加後
aqua update-checksum --prune
```

**mise（ランタイム / npm パッケージ）:**

```bash
# dot_config/mise/config.toml に追加後
mise lock
mise install
```

## トラブルシューティング

```bash
# 管理ファイル一覧
chezmoi managed

# 変更差分を確認
chezmoi diff

# 設定をリセット
chezmoi init --apply tak848
```

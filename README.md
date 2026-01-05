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
# プロンプトで profile を聞かれたら "public" を選択
```

CI では環境変数で指定：

```bash
CHEZMOI_PROFILE=public sh -c "$(curl -fsLS get.chezmoi.io)" -- init --apply tak848
```

### プライベート設定の有効化

初回インストール後、プライベート設定（API キー、gitconfig 等）を有効にする場合：

```bash
# 1. GitHub 認証（gh は mise 経由でインストール済み）
gh auth login
gh auth setup-git

# 2. プロファイルを変更
chezmoi init --data=false
# プロンプトで "personal" または "work" を選択

# 3. 設定を適用（private リポジトリから取得）
chezmoi apply
```

### 既存環境での更新

```bash
chezmoi update
```

## ツール管理

### 構成

```
Homebrew
  └─ 基本パッケージ

~/.local/bin/mise (bootstrap スクリプト)
  └─ mise (統合ツール管理)
       ├─ ランタイム: go, node, pnpm
       ├─ CLI ツール: fzf, ripgrep, starship, direnv, etc.
       ├─ npm グローバルパッケージ
       └─ aqua CLI
            └─ mise lock で checksum 取得不可: aws-cli, 1password/cli, zoxide
```

### 今後の予定

- Homebrew の宣言的管理（Brewfile など）

## 主な機能

### 自動セットアップ

初回起動時に以下を自動インストール：

- Homebrew
- mise（統合ツール管理、bootstrap 方式）
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
│   ├── mise/             # mise 設定
│   │   ├── config.toml
│   │   └── mise.lock
│   └── aquaproj-aqua/    # aqua 設定（mise 管理外ツール用）
│       ├── aqua.yaml
│       └── aqua-checksums.json
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

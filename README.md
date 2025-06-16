# dotfiles

Chezmoiで管理している個人用dotfilesリポジトリです。

## セットアップ

### 初回インストール

```bash
# Chezmoiをインストールして、このリポジトリを適用
sh -c "$(curl -fsLS get.chezmoi.io)" -- init --apply tak848
```

### 既存環境での更新

```bash
# リポジトリの最新版を取得して適用
chezmoi update
```

## 主な機能

### 自動セットアップ
初回起動時に以下のツールを自動インストール：
- Homebrew（macOS用パッケージマネージャー）
- direnv（環境変数管理）via aqua
- aqua（CLIツールバージョン管理）
- fzf（インタラクティブフィルタ）via aqua

### Git Worktree管理関数
- `gwt` - worktree間の移動
- `gwc` - 新規worktreeの作成（ブランチ選択/作成）
- `gwr` - worktreeの削除

### 環境別設定
Chezmoiテンプレート機能により、環境に応じた設定を自動適用：
- macOS/Linux別のパス設定
- インストール済みツールの検出と設定

### ローカル設定
Git管理外のローカル設定をサポート：
- `~/.zshrc.local` - マシン固有の環境変数やエイリアス
- `~/.zsh/local/*.zsh` - ローカル関数やスクリプト

これらのファイルは初回のみテンプレートが作成され、以降は変更されません。

## ディレクトリ構造

```
~/
├── .config/
│   └── aquaproj-aqua/    # aquaの設定
│       ├── aqua.yaml     # パッケージリスト
│       └── aqua-checksums.json
├── .zsh/
│   ├── functions/        # 共有関数（Git管理）
│   │   └── git-worktree.zsh
│   └── local/           # ローカル関数（Git管理外）
│       └── example.zsh
├── .zshrc               # Chezmoiが生成
└── .zshrc.local        # ローカル設定（Git管理外）
```

## カスタマイズ

### ローカル設定の追加
`~/.zshrc.local`や`~/.zsh/local/`に独自の設定を追加できます：

```bash
# ~/.zshrc.local の例
export COMPANY_PROXY="http://proxy.company.com:8080"
alias work="cd ~/work"

# ~/.zsh/local/my-functions.zsh の例
function deploy-staging() {
    echo "Deploying to staging..."
    # デプロイ処理
}
```

### 新しいツールの追加
`dot_config/aquaproj-aqua/aqua.yaml`に追加：

```yaml
- name: example/tool@v1.0.0
```

その後：
```bash
# チェックサムを更新
aqua update-checksum
# 変更をコミット・プッシュ
git add -A && git commit -m "Add example tool" && git push
```

## トラブルシューティング

### Chezmoiで管理されているファイルを確認
```bash
chezmoi managed
```

### 変更内容を確認してから適用
```bash
chezmoi diff
chezmoi apply
```

### 設定をリセット
```bash
chezmoi init --apply tak848
```

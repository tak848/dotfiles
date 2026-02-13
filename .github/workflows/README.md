# GitHub Actions ワークフロー

このディレクトリには、dotfilesリポジトリの自動化のためのGitHub Actionsワークフローが含まれています。

## ワークフロー一覧

### ci.yaml
- **目的**: Pull RequestやmainブランチへのプッシュでCIテストを実行
- **トリガー**: Pull Request、mainブランチへのプッシュ
- **動作**:
  - Ubuntu/macOSでのマトリックステスト
  - mise/aqua によるツールのインストール確認
  - Jsonnet 構文チェック
  - `task check` で自動生成ファイルの diff チェック（Ubuntu のみ、mise.lock 除外）
  - chezmoiテンプレートの検証

### mise-lock.yaml
- **目的**: Renovate/Dependabot が mise 設定を更新した際に、自動的に mise.lock を更新
- **トリガー**: `.mise.toml` / `dot_config/mise/config.toml` への変更を含む Pull Request（bot のみ）
- **動作**:
  - mise lock で全プラットフォームの lockfile を再生成
  - 変更があればコミットしてプッシュ

### mise-bootstrap.yaml
- **目的**: Renovate/Dependabot が mise バージョンを更新した際に、bootstrap スクリプトを自動更新
- **トリガー**: `.mise-bootstrap-version` への変更を含む Pull Request（bot のみ）
- **動作**:
  - `mise generate bootstrap` で bootstrap スクリプトを再生成
  - 変更があればコミットしてプッシュ

### aqua-checksums.yaml
- **目的**: Renovate/Dependabot が aqua.yaml を更新した際に、自動的に aqua-checksums.json を更新
- **トリガー**: `dot_config/aquaproj-aqua/aqua.yaml` への変更を含む Pull Request（bot のみ）
- **動作**:
  - `aqua update-checksum --prune` で checksum を更新
  - 変更があればコミットしてプッシュ

### lazy-lock.yaml
- **目的**: Neovim Lazy.nvim の lockfile を自動更新
- **トリガー**:
  - 毎週日曜 0:00 UTC（cron）/ 手動実行 → PR を作成
  - `dot_config/nvim/**` 等への変更を含む Pull Request → PR にコミット
- **動作**:
  - chezmoi apply で nvim 設定をデプロイ
  - `nvim --headless "+Lazy! sync"` でプラグイン同期
  - lazy-lock.json を更新

## 必要な Secrets / Variables

| 名前 | 種類 | 用途 |
|------|------|------|
| `APP_ID` | Repository Variable (`vars`) | GitHub App のアプリケーション ID |
| `APP_PRIVATE_KEY` | Repository Secret (`secrets`) | GitHub App の秘密鍵 |

これらは、ワークフローが PR にコミットをプッシュするために必要です（`GITHUB_TOKEN` では他のワークフローをトリガーできないため）。

## GitHub App の設定方法

1. [GitHub Apps](https://github.com/settings/apps)で新しい App を作成
2. 必要な権限:
   - Contents: Write
   - Pull requests: Write
   - Metadata: Read
3. リポジトリに App をインストール
4. App ID を Variables に、秘密鍵を Secrets に登録

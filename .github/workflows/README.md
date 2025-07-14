# GitHub Actions ワークフロー

このディレクトリには、dotfilesリポジトリの自動化のためのGitHub Actionsワークフローが含まれています。

## ワークフロー一覧

### aqua-checksums.yaml
- **目的**: RenovateやDependabotがaqua.yamlを更新した際に、自動的にaqua-checksums.jsonを更新
- **トリガー**: aqua.yamlへの変更を含むPull Request（ルートと`dot_config/aquaproj-aqua/`の両方）
- **動作**: 
  - 両方のディレクトリでaqua update-checksumを実行
  - 変更があればコミットしてプッシュ

### ci.yaml
- **目的**: Pull RequestやmainブランチへのプッシュでCIテストを実行
- **トリガー**: Pull Request、mainブランチへのプッシュ
- **動作**:
  - Ubuntu/macOSでのマトリックステスト
  - 両方のディレクトリでaquaによるツールのインストール確認
  - chezmoiテンプレートの検証

### maintenance.yaml
- **目的**: 定期的なメンテナンスタスクの実行
- **トリガー**: 毎週月曜日0:00 UTC、手動実行
- **動作**:
  - 両方のディレクトリでaqua-registryの更新チェック
  - 両方のディレクトリで未使用のchecksumのクリーンアップ

## 必要なSecrets

以下のSecretsを設定する必要があります：

- `APP_ID`: GitHub Appのアプリケーション ID
- `APP_PRIVATE_KEY`: GitHub Appの秘密鍵

これらは、ワークフローがPRにコミットをプッシュするために必要です。

## GitHub Appの設定方法

1. [GitHub Apps](https://github.com/settings/apps)で新しいAppを作成
2. 必要な権限:
   - Contents: Write
   - Pull requests: Write
   - Metadata: Read
3. リポジトリにAppをインストール
4. App IDと秘密鍵をSecretsに登録
---
description: Add a new tool to mise config.toml
allowed-tools: Bash, Read, Edit
argument-hint: <tool-name>
---

# mise ツール追加

## 概要
`dot_config/mise/config.toml`に新規ツールを追加する。

## 実行ステップ

### 1. ツール検索
```bash
mise search $ARGUMENTS
```
- 検索結果から正しいツール名を特定（例: `rga` → `ripgrep-all`）
- 候補が複数ある場合はユーザーに確認
- 以降のステップでは検索で特定した**正式なツール名**を使用

### 2. Backend確認
```bash
mise registry <検索で特定したツール名>
```
例: `mise registry ripgrep-all`

複数のbackendが返される場合の選択ロジック：

1. **core:** があれば → core を使用（ランタイムの公式backend）
2. **github:** があれば → github で試行 → lockfile生成後checksum確認 → なければ aqua: にフォールバック
3. **aqua:** があれば → aqua を使用（checksum対応保証）
4. **その他**（cargo:, npm:, go:）→ 該当backendを使用

### 3. バージョン確認
```bash
mise ls-remote <backend>:<tool> | tail -5
```
最新バージョンを取得。backend を明示的に指定すること。
例: `mise ls-remote aqua:phiresky/ripgrep-all | tail -5`

### 4. config.toml編集
`dot_config/mise/config.toml`を読み、適切なセクションに追加：

| Backend | セクション | 形式 |
|---------|-----------|------|
| core | ランタイム | `tool = "version"` |
| aqua | CLI ツール（aqua backend） | `"aqua:org/repo" = "version"` |
| github | aqua CLI（github backend） | `"github:org/repo" = "version"` |
| npm | npm グローバルパッケージ | `"npm:package" = "version"` |
| go | Language Server 等 | `"go:package" = "version"` |
| cargo | Rust ツール | `"cargo:crate" = "version"` |

既存ツールとの重複をチェックすること。

### 5. Lockfile生成
```bash
task mise:lock
```

### 6. Checksum確認（github backendの場合）
lockfile（`dot_config/mise/mise.lock`）を確認し、追加したツールにchecksumがあるか確認。
checksumがなければ、aqua backendで再試行。

## 出力
- 追加したツール名とバージョン
- 使用したbackend
- 変更したファイル一覧

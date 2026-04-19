# CLAUDE config

## MCP 設定の管理

MCP サーバーは **Claude Code プラグイン** または **`modify_dot_claude.json` の直接定義** で管理する。

### プラグイン（カスタムマーケットプレイス）

dotfiles リポジトリ自体がカスタムマーケットプレイス (`tak848-plugins`) として機能する。
認証不要な MCP サーバーは `claude-plugins/` 配下に独立したプラグインとして定義。

- `.claude-plugin/marketplace.json` → マーケットプレイス定義
- `claude-plugins/{name}/` → 各プラグイン（`.claude-plugin/plugin.json` + `.mcp.json`）
- `settings.jsonnet` の `extraKnownMarketplaces` → マーケットプレイスの宣言的登録
- `settings.jsonnet` の `enabledPlugins` → プラグインの有効化

**注意**: Bearer token 認証が必要な MCP サーバー（Devin 等）はプラグインの `.mcp.json` で環境変数展開が機能しないため（[claude-code#9427](https://github.com/anthropics/claude-code/issues/9427)）、`modify_dot_claude.json` で直接定義する。

### 設定ファイル

- `modify_dot_claude.json` → `~/.claude.json` の `mcpServers`（Devin）、`remoteControlAtStartup` 設定、旧 MCP エントリの削除
- `settings.jsonnet` → `~/.claude/settings.json` を全体生成（Claude Code は書き込まない）
- `ccgate.jsonnet` → `~/.claude/ccgate.jsonnet` の PermissionRequest 補助判定ルール（[tak848/ccgate](https://github.com/tak848/ccgate)）
- `permission-rules.libsonnet` → `permissions.deny` と `PreToolUse` 用の deny ルールのマスタを管理
- `auto-mode.libsonnet` → `settings.json` の `autoMode` ブロック（Auto Mode Classifier 用 allow / soft_deny / environment）

## Hook 補助判定の設定（ccgate）

- 共有既定値: `dot_claude/ccgate.jsonnet`
- deny ルールのマスタ: `dot_claude/permission-rules.libsonnet`
- 配置先: `~/.claude/ccgate.jsonnet`
- プロジェクト単位 override: `ccgate.local.jsonnet` または `.claude/ccgate.local.jsonnet`
- API key: 環境変数 `CC_AUTOMODE_ANTHROPIC_API_KEY`

`ccgate.local.jsonnet` は Git 管理しない。必要な場合だけ `provider.model` や `allow` / `deny` / `environment` を追加する。読み込み順は `~/.claude/ccgate.jsonnet` → プロジェクト local override。

## Auto Mode Classifier の設定（autoMode）

Max / Team / Enterprise / API プランで動作する Auto Mode Classifier 用ポリシー。Auto Mode 中は確認プロンプトが発火せず `PermissionRequest` hook（ccgate）が呼ばれないため、ccgate 相当のガードを Classifier 層に言語化するのが目的。

- 設定ファイル: `dot_claude/auto-mode.libsonnet`
- 展開先: `~/.claude/settings.json` の top-level `autoMode` ブロック（`settings.jsonnet` から import）
- Classifier が読み込む場所: `~/.claude/settings.json` / `.claude/settings.local.json` / managed settings のみ。プロジェクト共有の `.claude/settings.json` は **読まれない**

### 運用ルール

- ベースは `claude auto-mode defaults` の出力を `auto-mode.libsonnet` に転記している。
- `autoMode.allow` / `soft_deny` はデフォルトと **置き換わる（マージされない）** 仕様のため、デフォルト全量を本ファイルで保持する。
- デフォルトを改変する場合は「元を `//` でコメントアウト + 直下に改変版を追加」形式にする。diff で意図が読めるようにする。
- 独自追加（ccgate 由来 / 環境汚染ガード 等）は各配列末尾のセクションに置く。
- 日本語コメントは人間向け。Classifier は英語原文のみ読む。

### CC バージョン更新時のデフォルト再取得

自動同期はしない。Claude Code のバージョン更新時に以下を手動で行う:

```bash
claude auto-mode defaults > /tmp/new-defaults.json
# auto-mode.libsonnet の --- defaults --- セクションと diff を取り、
# 新規項目のみ追記する。コメントアウト済み行や改変版を上書きしない。
```

### 検証コマンド

```bash
claude auto-mode config   # Classifier が実際に使う effective 設定を確認
claude auto-mode critique # カスタムルールの AI レビュー（曖昧性・冗長性・false positive の指摘）
```

### ccgate との責務分離

両者は重複する目的を持つが発火タイミングが異なるので並行保持する:

| モード | 評価経路 |
|--------|----------|
| Auto Mode | Classifier が `autoMode.*` を参照（ccgate は呼ばれない） |
| default / acceptEdits / plan | `PermissionRequest` hook が発火し ccgate が稼働 |

ccgate を機械的に 1:1 移植するのではなく、autoMode はデフォルト + 独自カスタムで独立構成する。ccgate にあった判定はカバーするが、デフォルトで十分カバー済みの項目（curl|bash / out-of-repo rm 等）は二重化しない。

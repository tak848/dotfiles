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

## Hook 補助判定の設定（ccgate）

- 共有既定値: `dot_claude/ccgate.jsonnet`
- deny ルールのマスタ: `dot_claude/permission-rules.libsonnet`
- 配置先: `~/.claude/ccgate.jsonnet`
- プロジェクト単位 override: `ccgate.local.jsonnet` または `.claude/ccgate.local.jsonnet`
- API key: 環境変数 `CC_AUTOMODE_ANTHROPIC_API_KEY`

`ccgate.local.jsonnet` は Git 管理しない。必要な場合だけ `provider.model` や `allow` / `deny` / `environment` を追加する。読み込み順は `~/.claude/ccgate.jsonnet` → プロジェクト local override。

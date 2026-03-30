# CLAUDE config

## MCP 設定の管理

`~/.claude.json` は Claude Code が自動的に書き込むファイル（projects, tipsHistory 等）のため、
chezmoi の modify テンプレート（`modify_dot_claude.json`）で `mcpServers` のみ差分適用する。

- `modify_dot_claude.json` → `~/.claude.json` の `mcpServers.github` を管理
- `settings.jsonnet` → `~/.claude/settings.json` を全体生成（Claude Code は書き込まない）
- `ccgate.jsonnet` → `~/.claude/ccgate.jsonnet` の PermissionRequest 補助判定ルール（[tak848/ccgate](https://github.com/tak848/ccgate)）
- `permission-rules.libsonnet` → `permissions.deny` と `PreToolUse` 用の deny ルールのマスタを管理

GitHub MCP の Bearer token は環境変数 `GH_TOKEN` で供給する。

## Hook 補助判定の設定（ccgate）

- 共有既定値: `dot_claude/ccgate.jsonnet`
- deny ルールのマスタ: `dot_claude/permission-rules.libsonnet`
- 配置先: `~/.claude/ccgate.jsonnet`
- プロジェクト単位 override: `ccgate.local.jsonnet` または `.claude/ccgate.local.jsonnet`
- API key: 環境変数 `CC_AUTOMODE_ANTHROPIC_API_KEY`

`ccgate.local.jsonnet` は Git 管理しない。必要な場合だけ `provider.model` や `allow` / `deny` / `environment` を追加する。読み込み順は `~/.claude/ccgate.jsonnet` → プロジェクト local override。

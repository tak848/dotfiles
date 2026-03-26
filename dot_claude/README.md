# CLAUDE config

## MCP 設定の管理

`~/.claude.json` は Claude Code が自動的に書き込むファイル（projects, tipsHistory 等）のため、
chezmoi の modify テンプレート（`modify_dot_claude.json`）で `mcpServers` のみ差分適用する。

- `modify_dot_claude.json` → `~/.claude.json` の `mcpServers.github` を管理
- `settings.jsonnet` → `~/.claude/settings.json` を全体生成（Claude Code は書き込まない）
- `permission-gate.jsonnet` → `~/.claude/permission-gate.jsonnet` の hook 補助判定ルールを管理

GitHub MCP の Bearer token は環境変数 `GH_TOKEN` で供給する。

## Hook 補助判定の設定

- 共有既定値: `dot_claude/permission-gate.jsonnet`
- JSON Schema: `dot_claude/permission-gate.schema.json`
- 配置先: `~/.claude/permission-gate.jsonnet`
- 個人用 override: `~/.claude/permission-gate.local.jsonnet`
- API key: 環境変数 `CC_AUTOMODE_ANTHROPIC_API_KEY`

`permission-gate.local.jsonnet` は Git 管理しない。必要な場合だけ `provider.model` や `allow` / `soft_deny` / `environment` / `pre_tool_deny` を追加する。

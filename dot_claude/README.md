# CLAUDE config

## MCP 設定の管理

`~/.claude.json` は Claude Code が自動的に書き込むファイル（projects, tipsHistory 等）のため、
chezmoi の modify テンプレート（`modify_dot_claude.json`）で `mcpServers` のみ差分適用する。

- `modify_dot_claude.json` → `~/.claude.json` の `mcpServers.github` を管理
- `settings.jsonnet` → `~/.claude/settings.json` を全体生成（Claude Code は書き込まない）

GitHub MCP の Bearer token は環境変数 `GH_TOKEN` で供給する（direnv の `.envrc.local` で設定）。

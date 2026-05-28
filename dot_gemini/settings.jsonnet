// agy (Antigravity CLI) の CLI 専用設定
// 出力先: ~/.gemini/antigravity-cli/settings.json
// 参考: https://antigravity.google/docs/cli-overview
// 旧 Gemini CLI 用キー (ide.hasSeenNudge, security.auth, ui.*, general.disableAutoUpdate,
// contextFileName 等) は agy スキーマとの互換性が確認できないため捨てる。
// MCP サーバー定義は dot_gemini/mcp.jsonnet で別管理。
{
  notifications: true,
  trustedWorkspaces: [],
}

// agy (Antigravity CLI) 用の MCP サーバー設定
// 出力先: ~/.gemini/antigravity-cli/mcp_config.json
// IDE と共有したい場合は ~/.gemini/config/mcp_config.json も読まれるが、
// 本リポジトリでは CLI 専用パスに固定する。
// 旧 Gemini CLI の `url` キーは agy では `serverUrl` を使う
{
  mcpServers: {
    context7: {
      command: 'pnpm',
      args: ['dlx', '@upstash/context7-mcp'],
      env: {
        CONTEXT7_API_KEY: '${CONTEXT7_API_KEY}',
        MCP_TIMEOUT: '120000',
      },
    },
    devin: {
      serverUrl: 'https://mcp.devin.ai/sse',
      headers: {
        Authorization: 'Bearer ${DEVIN_API_KEY}',
      },
    },
    deepwiki: {
      serverUrl: 'https://mcp.deepwiki.com/sse',
      headers: {
        Authorization: 'Bearer ${DEVIN_API_KEY}',
      },
    },
  },
}

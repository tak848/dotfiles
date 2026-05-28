// agy (Antigravity CLI / IDE) 共有の MCP サーバー設定
// 出力先: ~/.gemini/config/mcp_config.json
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

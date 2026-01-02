{
  ide: {
    hasSeenNudge: true,
  },
  security: {
    auth: {
      selectedType: 'oauth-personal',
    },
  },
  ui: {
    theme: 'GitHub',
    showStatusInTitle: true,
    footer: {
      hideContextPercentage: false,
    },
    showMemoryUsage: true,
    showLineNumbers: true,
    showCitations: true,
  },
  general: {
    disableAutoUpdate: true,
  },
  contextFileName: ['CLAUDE.md', 'GEMINI.md'],
  mcpServers: {
    // gopls: {
    //   type: 'stdio',
    //   command: 'gopls',
    //   args: [
    //     'mcp',
    //   ],
    //   env: {
    //     MCP_TIMEOUT: '120000',
    //   },
    // },
    context7: {
      command: 'pnpm',
      args: ['dlx', '@upstash/context7-mcp'],
      env: {
        CONTEXT7_API_KEY: '${CONTEXT7_API_KEY}',
        MCP_TIMEOUT: '120000',
      },
    },
    devin: {
      url: 'https://mcp.devin.ai/sse',
      headers: {
        Authorization: 'Bearer ${DEVIN_API_KEY}',
      },
    },
    deepwiki: {
      url: 'https://mcp.deepwiki.com/sse',
      headers: {
        Authorization: 'Bearer ${DEVIN_API_KEY}',
      },
    },
  },
}

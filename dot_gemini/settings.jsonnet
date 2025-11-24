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
    gopls: {
      type: 'stdio',
      command: 'gopls',
      args: [
        'mcp',
      ],
      env: {
        MCP_TIMEOUT: '120000',
      },
    },
  },
}

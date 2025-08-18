{
  mcpServers: {
    playwright: {
      type: 'stdio',
      command: 'npx',
      args: [
        '@playwright/mcp@latest',
      ],
      env: {},
    },
    // playwrightの方が使いやすい感覚だったのでコメントアウト
    // puppeteer: {
    //   type: 'stdio',
    //   command: 'npx',
    //   args: [
    //     '@modelcontextprotocol/server-puppeteer',
    //   ],
    //   env: {},
    // },

    'gemini-cli': {
      type: 'stdio',
      command: 'npx',
      args: [
        '@choplin/mcp-gemini-cli',
        '--allow-npx',
      ],
      env: {},
    },


    o3: {
      command: 'npx',
      args: ['o3-search-mcp'],
      env: {
        // OPENAI_API_KEY: 'your-api-key',
        SEARCH_CONTEXT_SIZE: 'medium',
        REASONING_EFFORT: 'medium',
      },
    },
    'serena-global': {
      type: 'stdio',
      command: 'uvx',
      args: [
        '--from',
        'git+https://github.com/oraios/serena',
        'serena',
        'start-mcp-server',
        '--context',
        'ide-assistant',
        '--project',
        '${PWD}',
      ],
      env: {},
    },

  },
}

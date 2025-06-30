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
  },
}

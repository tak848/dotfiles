{
  permissions: {
    defaultMode: 'plan',
    allow: [
      'List(*)',
      'Bash(ls:*)',
      'Bash(cp:*)',
      'Bash(mkdir:*)',
      'Bash(curl:*)',
      'Bash(touch:*)',
      'Bash(lsof:*)',
      'Bash(awk:*)',
      'Bash(sed:*)',
      'Bash(wget:*)',
      'Bash(ps:*)',
      'Bash(top:*)',
      'Bash(which:*)',
      'Bash(find:*)',
      'Bash(grep:*)',
      'Bash(rg:*)',
      'Fetch(https://*)',
      'WebFetch(https://*)',
      'Bash(pnpm:*)',
      'Bash(make:*)',
      'Bash(sed:*)',
      'Bash(true)',
      'Bash(echo:*)',
      // rmは都度確認
      // "Bash(rm:*)",
      'Bash(go test:*)',
      'Bash(go build:*)',
      'Bash(go fmt:*)',
      'Bash(go doc:*)',
      'Bash(gofmt:*)',
      'Bash(gh pr list:*)',
      'Bash(gh pr view:*)',
      'Bash(gh pr diff:*)',
      'Bash(gh issue create:*)',
      'Bash(gh issue edit:*)',
      'Bash(git checkout:*)',
      'Bash(git add:*)',
      'Bash(git push:*)',
      // checkoutは都度確認
      // "Bash(git checkout:*)",
      // switchは都度確認
      // "Bash(git switch:*)",
      'Bash(git remote set-url:*)',
      'Bash(git pull:*)',
      'Bash(git reset:*)',
      'Bash(git cherry-pick:*)',
      // commitは都度確認
      // "Bash(git commit:*)",
      // docker composeは都度確認
      // "Bash(docker compose:*)",
      'Bash(docker compose ps:*)',
      //   'mcp__puppeteer__puppeteer_screenshot',
      //   'mcp__puppeteer__puppeteer_navigate',
      //   'mcp__puppeteer__puppeteer_click',
      //   'mcp__puppeteer__puppeteer_type',
      //   'mcp__puppeteer__puppeteer_evaluate',
      //   'mcp__puppeteer__puppeteer_fill',
      'mcp__playwright__browser_navigate',
      'mcp__playwright__browser_type',
      'mcp__playwright__browser_click',
      'mcp__playwright__browser_wait_for',
      'mcp__playwright__browser_take_screenshot',
      'mcp__playwright__browser_press_key',
      'mcp__playwright__browser_console_messages',
      // ブラウザの終了は、自動で行いはしてもらうが確認を必要とする
      //   'mcp__playwright__browser_close',
      'mcp__playwright__browser_tab_list',
      'mcp__playwright__browser_resize',

      'mcp__o3__o3-search',
    ],
    deny: [],
  },
  model: 'opus',

  toolPermissions: {
    // Puppeteer
    mcp__puppeteer: 'session',
    mcp__puppeteer__puppeteer_navigate: 'allow',
    mcp__puppeteer__puppeteer_screenshot: 'allow',
    mcp__puppeteer__puppeteer_click: 'session',
    mcp__puppeteer__puppeteer_type: 'session',
    mcp__puppeteer__puppeteer_evaluate: 'session',

    // Playwright
    mcp__playwright: 'session',
    mcp__playwright__playwright_navigate: 'allow',
    mcp__playwright__playwright_screenshot: 'allow',
    mcp__playwright__playwright_click: 'session',
    mcp__playwright__playwright_fill: 'session',
    mcp__playwright__playwright_select: 'session',
    mcp__playwright__playwright_evaluate: 'session',
  },
  env: {
    USE_BUILTIN_RIPGREP: 1,
  },
  hooks: {
    PostToolUse: [
      {
        matcher: 'Write|Edit|MultiEdit',
        hooks: [
          {
            type: 'command',
            command: "jq -r '.tool_input.file_path | select(endswith(\".js\") or endswith(\".ts\") or endswith(\".jsx\") or endswith(\".tsx\"))' | xargs -r npx prettier --write",
          },
          {
            type: 'command',
            command: "jq -r '.tool_input.file_path | select(endswith(\".go\"))' | xargs -r gofmt -w",
          },
        ],
      },
      //   {
      //     matcher: '',
      //     hooks: [
      //       {
      //         type: 'command',
      //         command: 'uv run ~/.claude/post_tool_use.py',
      //       },
      //     ],
      //   },
    ],
    PreToolUse: [
      //   {
      //     matcher: '',
      //     hooks: [
      //       {
      //         type: 'command',
      //         command: 'uv run ~/.claude/pre_tool_use.py',
      //       },
      //     ],
      //   },
    ],
    Notification: [
      {
        matcher: '',
        hooks: [
          {
            type: 'command',
            command: 'uv run ~/.claude/notification.py',
          },
        ],
      },
    ],
    Stop: [
      {
        matcher: '',
        hooks: [
          {
            type: 'command',
            command: 'uv run ~/.claude/stop.py',
          },
        ],
      },
    ],
  },
}

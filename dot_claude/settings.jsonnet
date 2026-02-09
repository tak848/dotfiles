{
  language: 'japanese',
  includeCoAuthoredBy: false,
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
      // 'WebFetch(https://*)',
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
      'Bash(gh pr checks:*)',
      'Bash(gh run view:*)',
      'Bash(git switch:*)',
      'Bash(git restore:*)',
      'Bash(git add:*)',
      'Bash(git commit:*)',
      'Bash(git push:*)',
      // checkoutは都度確認
      // "Bash(git checkout:*)",
      'Bash(git remote set-url:*)',
      'Bash(git pull:*)',
      'Bash(git fetch:*)',
      'Bash(git reset:*)',
      'Bash(git cherry-pick:*)',
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
      'mcp__playwright__browser_snapshot',
      // ブラウザの終了は、自動で行いはしてもらうが確認を必要とする
      //   'mcp__playwright__browser_close',
      'mcp__playwright__browser_tab_list',
      'mcp__playwright__browser_resize',

      'mcp__o3__o3-search',
      'mcp__gemini-cli__googleSearch',
      // serena-global
      'mcp__serena-global__get_symbols_overview',
      'mcp__serena-global__find_symbol',
      'mcp__serena-global__find_referencing_symbols',
      'mcp__serena-global__list_dir',
      'mcp__serena-global__find_file',
      'mcp__serena-global__search_for_pattern',
      'mcp__serena-global__list_memories',
      'mcp__serena-global__write_memory',
      'mcp__serena-global__read_memory',
      'mcp__serena-global__delete_memory',
      'mcp__serena-global__check_onboarding_performed',
      'mcp__serena-global__onboarding',
      'mcp__serena-global__think_about_collected_information',
      'mcp__serena-global__think_about_task_adherence',
      'mcp__serena-global__think_about_whether_you_are_done',

      'WebSearch',
      'mcp__context7__resolve-library-id',
      'mcp__context7__get-library-docs',
      'mcp__context7__query-docs',
      'mcp__deepwiki__read_wiki_structure',
      'mcp__deepwiki__read_wiki_contents',
      'mcp__deepwiki__ask_question',
    ],
    deny: [
      'Bash(git -C*)',
      'Bash(git commit*--amend*)',
      'Bash(git commit*--no-gpg-sign*)',
      'Bash(git commit*-S false*)',
      'Bash(git reset*--hard*)',
      'Bash(go generate*)',
      'Read(.envrc.local)',
      'Bash(npx:*)',
    ],
  },
  // model: 'claude-opus-4-1-20250805',
  model: 'claude-opus-4-6',
  // model: 'opus',

  // 無効らしい
  // toolPermissions: {
  //   // Puppeteer
  //   mcp__puppeteer: 'session',
  //   mcp__puppeteer__puppeteer_navigate: 'allow',
  //   mcp__puppeteer__puppeteer_screenshot: 'allow',
  //   mcp__puppeteer__puppeteer_click: 'session',
  //   mcp__puppeteer__puppeteer_type: 'session',
  //   mcp__puppeteer__puppeteer_evaluate: 'session',

  //   // Playwright
  //   mcp__playwright: 'session',
  //   mcp__playwright__playwright_navigate: 'allow',
  //   mcp__playwright__playwright_screenshot: 'allow',
  //   mcp__playwright__playwright_click: 'session',
  //   mcp__playwright__playwright_fill: 'session',
  //   mcp__playwright__playwright_select: 'session',
  //   mcp__playwright__playwright_evaluate: 'session',
  // },

  // https://docs.anthropic.com/en/docs/claude-code/settings#environment-variables
  env: {
    USE_BUILTIN_RIPGREP: '1',
    BASH_DEFAULT_TIMEOUT_MS: '600000',
    DISABLE_AUTOUPDATER: '1',
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1',
    MCP_TIMEOUT: '600000',
    MCP_TOOL_TIMEOUT: '600000',
    MAX_MCP_OUTPUT_TOKENS: '100000',  // default: 25000
    MAX_THINKING_TOKENS: '31199',

    CLAUDE_CODE_DISABLE_FEEDBACK_SURVEY: '1',
    DISABLE_TELEMETRY: '1',
    DISABLE_ERROR_REPORTING: '1',
    DISABLE_NON_ESSENTIAL_MODEL_CALLS: '1',
    CLAUDE_BASH_MAINTAIN_PROJECT_WORKING_DIR: '1',
  },
  alwaysThinkingEnabled: true,  // https://github.com/anthropics/claude-code/issues/8780
  hooks: {
    PostToolUse: [
      {
        matcher: 'Write|Edit|MultiEdit',
        hooks: [
          // {
          //   type: 'command',
          //   command: "jq -r '.tool_input.file_path | select(endswith(\".js\") or endswith(\".ts\") or endswith(\".jsx\") or endswith(\".tsx\"))' | xargs -r npx prettier --write",
          // },
          {
            type: 'command',
            command: "jq -r '.tool_input.file_path | select(endswith(\".go\"))' | xargs -r gofmt -w",
          },
        ],
      },
      {
        matcher: '',
        hooks: [
          {
            type: 'command',
            command: 'uv run ~/.claude/post_tool_use.py',
          },
        ],
      },
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

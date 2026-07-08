local permissionRules = import 'permission-rules.libsonnet';
local autoModeRules = import 'auto-mode.libsonnet';

{
  // 【一時的】language 強制が思考まで日本語化する不具合の回避（claude-code#62123, #63875）。
  // 言語制御は CLAUDE.md の「言語」セクションに委譲。問題なければ削除/復帰を判断する。
  // language: 'japanese',
  plansDirectory: '.claude/plans',
  // ccusage が読む ~/.claude/projects/**/*.jsonl の自動削除を実質無効化（デフォルト30日 → 10年）。
  // 過去の利用量集計が日々消えないようにするため。0 は無効値なので大きい値を指定する。
  cleanupPeriodDays: 3650,
  // teammateMode は Agent Teams 用の設定。Agent Teams 無効化に伴い不要（下記 env 参照）。
  // teammateMode: 'tmux',
  includeCoAuthoredBy: false,
  // ツール出力・コマンド出力を省略せずフル表示する。表示が省略されて情報が欠けるのを避ける。
  // thinking の常時フル表示設定は存在しない（transcript viewer の Ctrl+E で全表示は可能）。
  verbose: true,
  // classic renderer（default）。かつて再描画のたびに scrollback を破壊するバグがあり
  // fullscreen に切り替えていたが、2.1.204 時点で崩れが解消したことを確認したため戻す。
  // fullscreen はマウスキャプチャにより素のドラッグで文字選択できない点がストレスだった。
  // ref: https://code.claude.com/docs/en/fullscreen
  tui: 'default',
  // auto-memory を全プロジェクトで一律無効化。false で読み書き・memory ディレクトリ生成を停止する。
  // 規約・コンテキストは CLAUDE.md / AGENTS.md / CLAUDE.local.md に集約する方針。
  // ref: https://code.claude.com/docs/en/memory#enable-or-disable-auto-memory
  autoMemoryEnabled: false,
  extraKnownMarketplaces: {
    'tak848-plugins': {
      source: {
        source: 'github',
        repo: 'tak848/dotfiles',
      },
    },
    'openai-codex': {
      source: {
        source: 'github',
        repo: 'openai/codex-plugin-cc',
      },
    },
  },
  enabledPlugins: {
    'github@claude-plugins-official': true,
    'context7@claude-plugins-official': true,
    // gopls-lsp は重いため一律有効化しない。必要なプロジェクトで個別に有効化する。
    // 'gopls-lsp@claude-plugins-official': true,
    // gopls-lazy は大規模 Go モノレポ向けの軽量 LSP プロキシ（mise: github:sivchari/gopls-lazy）。
    // 公式 gopls-lsp と同様、一律有効化せず必要なプロジェクトで個別に有効化する。
    // 'gopls-lazy@tak848-plugins': true,
    'aws-knowledge@tak848-plugins': true,
    'codex@openai-codex': true,
  },
  statusLine: {
    type: 'command',
    command: '~/.claude/bin/cc-statusline',
    padding: 2,
  },
  permissions: {
    defaultMode: 'plan',
    allow: [
      'List(*)',
      'Bash(ls *)',
      'Bash(cp *)',
      'Bash(mkdir *)',
      'Bash(curl *)',
      'Bash(touch *)',
      'Bash(mktemp *)',
      'Bash(codex exec *)',
      'Bash(codex-review-exec *)',
      'Read(.codex-reviews/*)',
      'Write(.codex-reviews/*)',
      'Bash(lsof *)',
      'Bash(awk *)',
      'Bash(sed *)',
      'Bash(wget *)',
      'Bash(ps *)',
      'Bash(top *)',
      'Bash(which *)',
      'Bash(uuidgen)',
      'Bash(find *)',
      'Bash(grep *)',
      'Bash(rg *)',
      'Fetch(https://*)',
      // 'WebFetch(https://*)',
      'Bash(pnpm *)',
      'Bash(make *)',
      'Bash(true)',
      'Bash(echo *)',
      'Bash(printf *)',
      'Bash(head *)',
      'Bash(wc *)',
      'Bash(diff *)',
      'Bash(file *)',
      'Bash(date *)',
      'Bash(mise where *)',
      'Bash(mise registry *)',
      'Bash(mise settings *)',
      'Bash(mise doctor *)',
      'Bash(mise latest *)',
      'Bash(mise search *)',
      'Bash(mise --version)',
      'Bash(mise ls *)',
      'Bash(mise current *)',
      'Bash(mise plugins ls *)',
      // rmは都度確認
      // "Bash(rm *)",
      'Bash(go test *)',
      'Bash(go build *)',
      'Bash(go fmt *)',
      'Bash(go doc *)',
      'Bash(go vet *)',
      'Bash(gofmt *)',
      'Bash(gh pr list *)',
      'Bash(gh pr view *)',
      'Bash(gh pr diff *)',
      'Bash(gh pr status *)',
      'Bash(gh repo view *)',
      'Bash(gh issue list *)',
      'Bash(gh issue view *)',
      'Bash(gh issue create *)',
      'Bash(gh issue edit *)',
      'Bash(gh search *)',
      'Bash(gh release view *)',
      'Bash(gh pr checks *)',
      'Bash(gh run view *)',
      'Bash(git status *)',
      'Bash(git switch *)',
      'Bash(git restore *)',
      'Bash(git add *)',
      'Bash(git commit *)',
      'Bash(git push *)',
      'Bash(git log *)',
      'Bash(git show *)',
      'Bash(git describe *)',
      // checkoutは都度確認
      // "Bash(git checkout *)",
      'Bash(git remote set-url *)',
      'Bash(git pull *)',
      'Bash(git fetch *)',
      'Bash(git reset *)',
      'Bash(git cherry-pick *)',
      // docker composeは都度確認
      // "Bash(docker compose *)",
      'Bash(docker compose ps *)',
      //   'mcp__puppeteer__puppeteer_screenshot',
      //   'mcp__puppeteer__puppeteer_navigate',
      //   'mcp__puppeteer__puppeteer_click',
      //   'mcp__puppeteer__puppeteer_type',
      //   'mcp__puppeteer__puppeteer_evaluate',
      //   'mcp__puppeteer__puppeteer_fill',
      // 'mcp__playwright__browser_navigate',
      // 'mcp__playwright__browser_type',
      // 'mcp__playwright__browser_click',
      // 'mcp__playwright__browser_wait_for',
      // 'mcp__playwright__browser_take_screenshot',
      // 'mcp__playwright__browser_press_key',
      // 'mcp__playwright__browser_console_messages',
      // 'mcp__playwright__browser_snapshot',
      // // ブラウザの終了は、自動で行いはしてもらうが確認を必要とする
      // //   'mcp__playwright__browser_close',
      // 'mcp__playwright__browser_tab_list',
      // 'mcp__playwright__browser_resize',

      'mcp__o3__o3-search',
      // 'mcp__gemini-cli__googleSearch',  // gemini-cli は廃止し agy へ移行

      'WebSearch',
      'WebFetch(domain:docs.anthropic.com)',
      'WebFetch(domain:docs.devin.ai)',
      'WebFetch(domain:devin.ai)',
      'WebFetch(domain:developers.openai.com)',
      'mcp__plugin_context7_context7__resolve-library-id',
      'mcp__plugin_context7_context7__query-docs',
      'mcp__deepwiki__read_wiki_structure',
      'mcp__deepwiki__read_wiki_contents',
      'mcp__deepwiki__ask_question',
      // Devin MCP (直接定義 — プラグインでは env var 展開が未対応)
      'mcp__devin__ask_question',
      'mcp__devin__read_wiki_contents',
      // AWS Knowledge Plugin (tak848-plugins)
      'mcp__plugin_aws-knowledge_aws-knowledge__aws___search_documentation',
      'mcp__plugin_aws-knowledge_aws-knowledge__aws___read_documentation',
      'mcp__plugin_aws-knowledge_aws-knowledge__aws___recommend',
      'mcp__plugin_aws-knowledge_aws-knowledge__aws___list_regions',
      'mcp__plugin_aws-knowledge_aws-knowledge__aws___get_regional_availability',
      'mcp__plugin_aws-knowledge_aws-knowledge__aws___retrieve_agent_sop',
      // GitHub Plugin - read 系
      'mcp__plugin_github_github__get_me',
      'mcp__plugin_github_github__list_issues',
      'mcp__plugin_github_github__search_issues',
      'mcp__plugin_github_github__issue_read',
      'mcp__plugin_github_github__list_pull_requests',
      'mcp__plugin_github_github__search_pull_requests',
      'mcp__plugin_github_github__pull_request_read',
      'mcp__plugin_github_github__get_file_contents',
      'mcp__plugin_github_github__list_commits',
      'mcp__plugin_github_github__get_commit',
      'mcp__plugin_github_github__list_branches',
      'mcp__plugin_github_github__list_releases',
      'mcp__plugin_github_github__list_tags',
      'mcp__plugin_github_github__search_code',
      'mcp__plugin_github_github__get_repository_tree',
      'mcp__plugin_github_github__list_notifications',
      'mcp__plugin_github_github__get_notification_details',
      // GitHub Plugin - write 系（コメント・Issue のみ自動許可）
      'mcp__plugin_github_github__add_issue_comment',
      'mcp__plugin_github_github__issue_write',
      'mcp__plugin_github_github__add_reply_to_pull_request_comment',
      // GitHub Plugin - PR write 系（都度確認のためコメントアウト）
      // 'mcp__plugin_github_github__create_pull_request',
      // 'mcp__plugin_github_github__update_pull_request',
    ],
    deny: permissionRules.permissionsDeny,
  },
  autoMode: {
    environment: autoModeRules.environment,
    allow: autoModeRules.allow,
    soft_deny: autoModeRules.soft_deny,
  },
  // plan mode 中に auto mode classifier が read-only tool を auto-allow し、
  // ccgate（PermissionRequest hook）をバイパスするのを止める。許可判定は ccgate に一本化する。
  // auto mode 自体は無効化しない（disableAutoMode は使わない）。Shift+Tab での手動利用は維持。
  // CC 2.1.186 で入った plan-mode read-only auto-allow 挙動への対処。
  // 未設定（デフォルト）は true 扱い（cBr() の `!== false` 判定）のため、明示的に false にする必要がある。
  useAutoModeDuringPlan: false,
  // model: 'claude-opus-4-1-20250805',
  // model: 'claude-opus-4-6',
  // model: 'claude-opus-4-6[1m]',
  // model: 'claude-opus-4-7[1m]',
  model: 'claude-opus-4-8[1m]',
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
    // remote-control の eligibility チェック等がブロックされるため無効化
    // CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1',
    MCP_TIMEOUT: '600000',
    MCP_TOOL_TIMEOUT: '600000',
    MAX_MCP_OUTPUT_TOKENS: '100000',  // default: 25000
    CLAUDE_CODE_AUTO_COMPACT_WINDOW: '500000',
    // adaptive thinking (effortLevel) が有効な場合、以下は不要
    // MAX_THINKING_TOKENS: '31199',
    // CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING: '1',  // これを設定すると MAX_THINKING_TOKENS に戻る

    // fullscreen rendering のマウスクリック機能（select メニューのクリック選択・click-to-expand 等）
    // を無効化する。ダイアログ表示直後の意図しないクリックで選択肢が勝手に確定する事故を防ぐ。
    // ホイールスクロールは維持される（CLAUDE_CODE_DISABLE_MOUSE と違いキャプチャ自体は残る）。
    // ref: https://code.claude.com/docs/en/fullscreen#keep-native-text-selection
    CLAUDE_CODE_DISABLE_MOUSE_CLICKS: '1',
    CLAUDE_CODE_DISABLE_FEEDBACK_SURVEY: '1',
    // DISABLE_TELEMETRY はフィーチャーフラグ(Statsig)の取得も停止してしまうため無効化
    // ref: https://zenn.dev/m0370/articles/d7e77adebd0ba8
    // DISABLE_TELEMETRY: '1',
    DISABLE_ERROR_REPORTING: '1',
    DISABLE_NON_ESSENTIAL_MODEL_CALLS: '1',
    CLAUDE_BASH_MAINTAIN_PROJECT_WORKING_DIR: '1',
    // 実験的 Agent Teams は無効化する。teammate（subagent）の permission request が
    // PermissionRequest hook を経由せず端末の手動プロンプトに直行し、ccgate による許可判定が
    // バイパスされる既知バグ（anthropics/claude-code#23983, open）があるため。
    // 許可制御を ccgate に一本化している運用と両立しない。upstream 修正後に再検討する。
    // CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS: '1',
  },
  effortLevel: 'xhigh',
  // alwaysThinkingEnabled は adaptive thinking (effortLevel) により不要
  // alwaysThinkingEnabled: true,  // https://github.com/anthropics/claude-code/issues/8780
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
            command: '~/.claude/bin/cc-post-tool-use',
          },
        ],
      },
    ],
    PreToolUse: permissionRules.preToolUseHooks + [
      {
        matcher: 'Write|Edit|MultiEdit',
        hooks: [
          {
            type: 'command',
            command: '~/.claude/bin/cc-check-mojibake',
          },
        ],
      },
    ],
    PermissionRequest: [
      {
        matcher: '',
        hooks: [
          {
            type: 'command',
            command: 'ccgate claude',
          },
        ],
      },
    ],
    Notification: [
      {
        matcher: '',
        hooks: [
          {
            type: 'command',
            command: '~/.claude/bin/cc-notification',
            async: true,
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
            command: '~/.claude/bin/cc-stop',
            async: true,
          },
        ],
      },
    ],
  },
}

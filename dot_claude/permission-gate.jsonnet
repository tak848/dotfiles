{
  ['$schema']: './permission-gate.schema.json',
  provider: {
    name: 'anthropic',
    model: 'claude-haiku-4-5',
    timeout_ms: 4000,
  },
  pre_tool_deny: [
    {
      matcher: 'Bash',
      pattern: '(^|\\s)git commit\\b.*\\s--amend(\\s|$)',
      reason: 'git commit --amend は禁止',
      system_message: 'amend で履歴を潰さず、新しい commit を積んでください。',
    },
    {
      matcher: 'Bash',
      pattern: '(^|\\s)python3?(\\s|$)',
      reason: 'python/python3 の直接実行は禁止',
      system_message: 'system の python を直接呼ばず、せめて uv run を使ってください。',
    },
    {
      matcher: 'Bash',
      pattern: '(^|\\s)npx(\\s|$)',
      reason: 'npx の直接実行は禁止',
      system_message: 'npx は許可していないので、既存のツールか別の安全な経路を使ってください。',
    },
  ],
  allow: [
    'Read-Only Operations: GET requests, read-only API calls, or queries that do not modify state.',
    'Local Operations: Read-only work inside the current repository or current worktree.',
  ],
  soft_deny: [
    'Git Destructive: Force pushing, deleting remote branches, or rewriting history.',
    'Irreversible Local Destruction: Deleting or overwriting files that existed before the session without explicit user direction.',
    'Sibling Checkout Confusion: When operating inside a worktree, reading a different sibling checkout instead of the current worktree should usually be denied or fall through for user confirmation.',
  ],
  environment: [
    '**Trusted repo**: The git repository the session started in.',
    '**Current worktree context**: Prefer the current worktree and its own files over sibling checkouts unless the user clearly asks otherwise.',
  ],
}

{
  ['$schema']: './permission-gate.schema.json',
  provider: {
    name: 'anthropic',
    model: 'claude-opus-4-6',
    timeout_ms: 4000,
  },
  trusted_paths: [
    '~/.claude',
    '~/.codex',
  ],
  pre_tool_deny: [
    {
      matcher: 'Bash',
      pattern: '(^|\\s)git rebase\\b.*\\s-i(\\s|$)',
      reason: 'interactive rebase は確認なしでは禁止',
      additional_context: '履歴整理が必要なら、目的と対象 commit を明示してユーザー確認を取ること。',
    },
  ],
  allow: [
    'Read-Only Operations: GET requests, read-only API calls, or queries that do not modify state.',
    'Local Operations: Read-only work inside the current repository or explicitly trusted directories.',
  ],
  soft_deny: [
    'Git Destructive: Force pushing, deleting remote branches, or rewriting history.',
    'Irreversible Local Destruction: Deleting or overwriting files that existed before the session without explicit user direction.',
    'Out-of-scope Paths: Reading or writing outside the current repository and trusted local config directories.',
  ],
  environment: [
    '**Trusted repo**: The git repository the session started in.',
    '**Trusted local config**: ~/.claude and ~/.codex for local tooling configuration.',
  ],
}

{
  ['$schema']: './permission-gate.schema.json',
  provider: {
    name: 'anthropic',
    model: 'claude-haiku-4-5',
    timeout_ms: 40000,
  },
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

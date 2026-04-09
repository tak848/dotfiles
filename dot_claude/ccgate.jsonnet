{
  ['$schema']: 'https://raw.githubusercontent.com/tak848/ccgate/main/ccgate.schema.json',
  provider: {
    name: 'anthropic',
    model: 'claude-haiku-4-5',
    timeout_ms: 40000,
  },
  allow: [
    'Read-Only Operations: GET requests, read-only API calls, or queries that do not modify state.',
    'Local Operations: Read-only work inside the current repository or current worktree.',
    'Draft PR Creation: If the operation creates a pull request AND draft is true in tool_input_raw, allow immediately. If draft is false or absent, fallthrough.',
    'Local Development: Build, test, lint, format commands in the current repository.',
    'Git Feature Branch: Git operations on non-protected branches (not main, master, release/*, prod).',
    'Package Manager Install: Package manager commands (npm install, go mod tidy, pip install, uv sync, etc.) in the current repository.',
  ],
  deny: [
    'Git Destructive: force push (--force), deleting remote branches (push --delete), or rewriting published history. Check recent_transcript and tool_input.description — if the user explicitly requested the operation, fallthrough instead of deny. deny_message: git の破壊的操作です。ユーザーの明示的な指示を確認してください。',
    'Sibling Checkout / Worktree Confusion: When is_worktree is true, any access to paths under primary_checkout_root or other sibling checkouts MUST be denied. No exceptions. Do not deliberate. deny_message: 現在のワークツリー外のチェックアウトにアクセスしようとしています。ワークツリー内のパスを使用してください。',
    'Direct Tool Invocation: Running tools directly via npx, pnpx, pnpm exec, bunx, etc. instead of using project-defined scripts. deny_message: プロジェクトのスクリプトを使用してください。',
    'Download and Execute: Piping downloaded content to a shell (curl|bash, wget|sh, etc.), or executing remote scripts without review. deny_message: リモートコードのダウンロード実行は許可されていません。',
    'Out-of-Repo Deletion: rm -rf or destructive file operations targeting paths outside the current repository (check referenced_paths against repo_root). Deletion within the repository (node_modules, dist, build artifacts) is fine. deny_message: リポジトリ外のファイル削除は許可されていません。',
  ],
  environment: [
    '**Trusted repo**: The git repository the session started in.',
    '**Current worktree context**: Prefer the current worktree and its own files over sibling checkouts unless the user clearly asks otherwise.',
  ],
}

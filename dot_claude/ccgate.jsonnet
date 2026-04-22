{
  ['$schema']: 'https://raw.githubusercontent.com/tak848/ccgate/main/ccgate.schema.json',
  provider: {
    name: 'anthropic',
    model: 'claude-haiku-4-5',
    timeout_ms: 10000,
  },
  log_max_size: 100 * 1024 * 1024,
  metrics_max_size: 100 * 1024 * 1024,
  allow: [
    'Read-Only Operations: GET requests, read-only API calls, or queries that do not modify state.',
    'Local Operations: Read-only work inside the current repository or current worktree.',
    'Library Source Read: Read-only inspection (Read/Glob/Grep, or Bash head/tail/sed -n/cat/less/file/stat/wc/awk) of package and module caches and of system temp. This includes: Go module cache (~/go/pkg/mod, $GOPATH/pkg/mod, ~/Library/Caches/go-build), node package manager stores (pnpm/npm/yarn caches), Python package cache, cargo registry (~/.cargo/registry), Homebrew prefix (/opt/homebrew, /usr/local), ~/.cache/*, /usr/include, system temp (/tmp, /var/folders), and installed tool binaries. These cannot contain user private source repos. This rule is READ-ONLY only — any write, delete, or execute to these paths MUST fallthrough.',
    'Draft PR Creation: If the operation creates a pull request AND draft is true in tool_input_raw, allow immediately. If draft is false or absent, fallthrough.',
    'Local Development: Build, test, lint, format commands in the current repository.',
    'Git Feature Branch: Git operations on non-protected branches (not main, master, release/*, prod).',
    'Package Manager Install: Package manager install/sync commands in the current repository (e.g. pnpm install, go mod tidy, uv sync).',
  ],
  deny: [
    'Git Destructive: force push (--force), deleting remote branches (push --delete), or rewriting published history. Check recent_transcript and tool_input.description — if the user explicitly requested the operation, fallthrough instead of deny. deny_message: Destructive git operation. Confirm explicit user instruction.',
    'Sibling Checkout Access (ABSOLUTE): When is_worktree is true, ANY access (read or write, via any tool, no exceptions) to paths under primary_checkout_root or under other sibling worktree checkouts of the SAME repository MUST be denied. This rule is strictly about same-repo cross-checkout confusion — it does NOT apply to package/module caches, /tmp, or unrelated repositories (those are handled by other rules). deny_message: Accessing another checkout of the same repository is forbidden. Stay within this worktree.',
    'Unrelated Repository Access: Reading or writing files inside a DIFFERENT git repository the user has NOT explicitly authorized via --add-dir. A path under ~/repos/*, ~/src/*, ~/work/*, ~/code/*, ~/ghq/* (or similar source-hosting roots) that is NOT under repo_root AND NOT a package/module cache should be denied — these look like user source repos. If the path is clearly a package/module cache or /tmp, this rule does NOT apply and the operation should fall through to Library Source Read. When genuinely uncertain whether a path is a private repo or a cache, fallthrough (not deny). deny_message: Reading outside authorized repositories. Ask the user to --add-dir this path if intentional.',
    'Direct Tool Invocation: Running tools directly via one-shot package runners (npx, pnpx, pnpm exec, bunx, etc.) instead of using project-defined scripts. deny_message: Direct tool invocation not allowed. Use project-defined scripts.',
    'Download and Execute: Piping downloaded content to a shell (curl|bash, wget|sh, etc.), or executing remote scripts without review. deny_message: Download-and-execute not allowed.',
    'Out-of-Repo Deletion: rm -rf or destructive file operations targeting paths outside the current repository (check referenced_paths against repo_root). Deletion within the repository (node_modules, dist, build artifacts) is fine. deny_message: Deletion outside repository not allowed.',
  ],
  environment: [
    '**Trusted repo**: The git repository the session started in.',
    '**Current worktree context**: Prefer the current worktree and its own files over sibling checkouts unless the user clearly asks otherwise.',
  ],
}

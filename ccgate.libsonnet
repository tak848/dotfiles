local base = {
  provider: {
    name: 'anthropic',
    model: 'claude-haiku-4-5',
    timeout_ms: 10000,
  },

  log_max_size: 100 * 1024 * 1024,
  metrics_max_size: 100 * 1024 * 1024,
};

local config(opts) = base {
  allow: opts.extraAllowBefore + [
    'Read-Only Operations: ' + opts.readOnlyOperations + ' that do not modify state.',
    'Local Operations: Read-only work inside the current repository or current worktree.',
    'Library Source Read: Read-only inspection of package/module caches, installed tool caches, and system temp. This includes Go module cache, node package manager stores, Python package cache, cargo registry, Homebrew prefix, ~/.cache/*, /usr/include, /tmp, /var/folders, and installed tool binaries. This rule is READ-ONLY only; any write, delete, or execute to these paths MUST fall through.',
    'Draft PR Creation: If the operation creates a pull request AND draft is true in ' + opts.draftField + ', allow immediately. If draft is false or absent, fall through.',
    'Local Development: Build, test, lint, format commands in the current repository.',
    'Git Feature Branch: Git operations on non-protected branches (not main, master, release/*, prod). ' + opts.gitBranchContext,
    'Package Manager Install: Package manager install/sync commands in the current repository.',
  ],

  deny: [
    'Git Destructive: force push (--force), deleting remote branches (push --delete), or rewriting published history. ' + opts.intentContext + ' deny_message: Destructive git operation. Confirm explicit user instruction.',
    'Sibling Checkout Access (ABSOLUTE): When is_worktree is true, ANY access to paths under primary_checkout_root or under other sibling worktree checkouts of the SAME repository MUST be denied. This does NOT apply to package/module caches, system temp, or unrelated repositories. deny_message: Accessing another checkout of the same repository is forbidden. Stay within this worktree.',
    'Unrelated Repository Access: Reading or writing files inside a DIFFERENT git repository the user has NOT explicitly authorized via --add-dir is forbidden. A path under ~/repos/*, ~/src/*, ~/work/*, ~/code/*, ~/ghq/*, or similar source-hosting roots that is NOT under repo_root and NOT a package/module cache should be denied. If the path is clearly a package/module cache or system temp, this rule does NOT apply. When genuinely uncertain whether a path is a private repo or a cache, fall through. deny_message: Reading outside authorized repositories. Ask the user to --add-dir this path if intentional.',
    'Direct Tool Invocation: Running tools directly via one-shot package runners (npx, pnpx, pnx, pnpm dlx, bunx, etc.) or bypassing package.json scripts with `pnpm exec`. Use package.json scripts via `pnpm run <script>` or `pnpm <script>` instead. deny_message: Direct package runner or script bypass not allowed. Use package.json scripts.',
    'Download and Execute: Piping downloaded content to a shell (curl|bash, wget|sh, etc.), or executing remote scripts without review. deny_message: Download-and-execute not allowed.',
    'Out-of-Repo Deletion: rm -rf or destructive file operations targeting paths outside the current repository. Deletion within the repository for build artifacts is fine. deny_message: Deletion outside repository not allowed.',
    'Redundant cd to Current Directory: A command whose first action is to cd via an ABSOLUTE path into the directory that is already context.cwd, and then run the real work (e.g. `cd /abs/path && <cmd>` or `cd /abs/path; <cmd>` where /abs/path resolves to cwd). The leading cd is pointless because the shell already runs in cwd — wherever that is. Scope: this rule ONLY matches when the cd target resolves to cwd itself. A cd to any OTHER directory is out of scope for THIS rule and is NOT thereby allowed — it remains subject to the other rules (Sibling Checkout Access and Unrelated Repository Access still forbid cross-worktree and unauthorized-repository access). deny_message: Unnecessary cd — you are already in this directory. Drop the leading `cd <abs-path> &&` and run the command directly.',
  ],

  environment: [
    'Trusted repo: the git repository the session started in is the trust boundary.',
    'Current worktree context: prefer the current worktree and its own files over sibling checkouts unless the user clearly asks otherwise.',
  ] + opts.extraEnvironment,
};

{
  claude: config({
    extraAllowBefore: [],
    readOnlyOperations: 'Read, Glob, Grep, Bash inspection commands, GET requests, read-only API calls, or queries',
    draftField: 'tool_input_raw',
    gitBranchContext: 'For switch -c / checkout -b, the target branch is in the command; context.branch_name is the pre-command branch.',
    intentContext: 'Check recent_transcript and tool_input.description; if the user explicitly requested the operation, fall through instead of deny.',
    extraEnvironment: [],
  }) + {
    ['$schema']: 'https://raw.githubusercontent.com/tak848/ccgate/main/schemas/claude.schema.json',
  },

  codex: config({
    extraAllowBefore: [
      'Local Writes: apply_patch hunks and file edits whose target paths are all under cwd / repo_root, as long as the operation does not match a deny rule.',
    ],
    readOnlyOperations: 'Bash inspection commands',
    draftField: 'tool_input',
    gitBranchContext: 'For switch -c / checkout -b, the target branch is in the command; context.branch_name is the pre-command branch.',
    intentContext: 'Codex HookInput does not carry recent_transcript; if explicit user intent is ambiguous, fall through instead of deny.',
    extraEnvironment: [
      'Codex HookInput does not carry a recent_transcript field. Decide from tool_name + tool_input + cwd; if intent is ambiguous, return fallthrough.',
    ],
  }) + {
    ['$schema']: 'https://raw.githubusercontent.com/tak848/ccgate/main/schemas/codex.schema.json',
  },
}

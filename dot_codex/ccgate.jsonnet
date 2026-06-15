{
  ['$schema']: 'https://raw.githubusercontent.com/tak848/ccgate/main/schemas/codex.schema.json',

  provider: {
    name: 'anthropic',
    model: 'claude-haiku-4-5',
    timeout_ms: 10000,
  },

  log_max_size: 100 * 1024 * 1024,
  metrics_max_size: 100 * 1024 * 1024,

  allow: [
    'Read-Only Operations: Bash inspection commands, read-only API calls, or queries that do not modify state.',
    'Local Operations: Read-only work inside the current repository or current worktree.',
    'Local Writes: apply_patch hunks and file edits whose target paths are all under cwd / repo_root, as long as the operation does not match a deny rule.',
    'Library Source Read: Read-only inspection of package/module caches, installed tool caches, and system temp. This includes Go module cache, node package manager stores, Python package cache, cargo registry, Homebrew prefix, ~/.cache/*, /usr/include, /tmp, /var/folders, and installed tool binaries. This rule is READ-ONLY only; any write, delete, or execute to these paths MUST fall through.',
    'Draft PR Creation: If the operation creates a pull request AND draft is true in tool_input, allow immediately. If draft is false or absent, fall through.',
    'Local Development: Build, test, lint, format commands in the current repository.',
    'Git Feature Branch: Git operations on non-protected branches (not main, master, release/*, prod). For switch -c / checkout -b, the target branch is in the command; context.branch_name is the pre-command branch.',
    'Package Manager Install: Package manager install/sync commands in the current repository with no global flags.',
    'Trusted MCP Read/Scoped Write: MCP tools whose server is explicitly trusted by the user and whose side effects are confined to the user-authorized scope.',
  ],

  deny: [
    'Git Destructive: force push (--force, --force-with-lease), deleting remote branches (push --delete), deleting protected branches, or rewriting published history. If the user explicitly requested the operation, fall through instead of deny. deny_message: Destructive git operation. Confirm explicit user instruction.',
    'Sibling Checkout Access (ABSOLUTE): When is_worktree is true, ANY access to paths under primary_checkout_root or under other sibling worktree checkouts of the SAME repository MUST be denied. This does NOT apply to package/module caches, system temp, or unrelated repositories. deny_message: Accessing another checkout of the same repository is forbidden. Stay within this worktree.',
    'Unrelated Repository Access: Reading or writing files inside a DIFFERENT git repository the user has NOT explicitly authorized via --add-dir is forbidden. A path under ~/repos/*, ~/src/*, ~/work/*, ~/code/*, ~/ghq/*, or similar source-hosting roots that is NOT under repo_root and NOT a package/module cache should be denied. If the path is clearly a package/module cache or system temp, this rule does NOT apply. When genuinely uncertain whether a path is a private repo or a cache, fall through. deny_message: Reading outside authorized repositories. Ask the user to --add-dir this path if intentional.',
    'Direct Tool Invocation: Running tools directly via one-shot package runners (npx, pnpx, pnpm exec, bunx, etc.) instead of using project-defined scripts. deny_message: Direct tool invocation not allowed. Use project-defined scripts.',
    'Download and Execute: Piping downloaded content to a shell (curl|bash, wget|sh, etc.), eval "$(curl ...)", or executing remote scripts without review. deny_message: Download-and-execute not allowed.',
    'Out-of-Repo Deletion: rm -rf, mv, or destructive file operations targeting paths outside the current repository. Deletion within the repository for build artifacts is fine. deny_message: Deletion outside repository not allowed.',
    'Privilege Escalation: sudo or other privilege escalation from the hook context. deny_message: Privilege escalation is not allowed from the hook context.',
    'Unrestricted Network Out: nc, ssh, scp, ftp, or similar network tools to non-allowlisted hosts. deny_message: Network-out tools are blocked from the hook context.',
    'Destructive MCP Tools: MCP tools that advertise destructive side effects (delete, drop, force-push, send-message, post-comment, close, merge, etc.) without an explicit allow rule. deny_message: Destructive MCP tool not allowed without an explicit project-local rule.',
    'Homebrew Forbidden: Any brew invocation (install, upgrade, uninstall, tap, bundle, etc.) is forbidden. Tool management in this environment is delegated to mise and aqua. Installing Homebrew itself via brew.sh / install.sh is also forbidden. deny_message: Homebrew is not allowed here; use mise or aqua.',
  ],

  environment: [
    'Tool surface: Codex hooks fire for Bash, apply_patch, MCP tool calls, and other tool kinds. Classify by tool_name + tool_input shape rather than assuming a single surface.',
    'Trusted repo: the git repository the session started in is the trust boundary.',
    'Path scope: when tool_input targets paths outside cwd / repo_root, treat it as out-of-repo and lean toward deny unless it is clearly read-only and benign.',
    'Preferred tool management: use mise or aqua instead of Homebrew for CLI/tool installation.',
    'Current worktree context: prefer the current worktree and its own files over sibling checkouts unless the user clearly asks otherwise.',
    'Codex HookInput does not carry a recent_transcript field. Decide from tool_name + tool_input + cwd; if intent is ambiguous, return fallthrough.',
  ],
}

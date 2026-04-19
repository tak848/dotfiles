// ccgate (PermissionRequest hook で動く LLM 判定ゲート) の設定。
//
// 役割: Claude Code が確認プロンプトを出す前の段階で、静的 allow/deny では裁けない
// 柔軟判断が要るケースに対して LLM が allow / deny / fallthrough を返す。
// 詳細: https://github.com/tak848/ccgate
//
// 書式: allow / deny は自然言語（英語）の prose リスト。LLM は英語原文を読むので、
// 日本語コメントは人間向け（レビュー・編集時の可読性用）。
//
// 運用ルール:
//   - deny 項目は末尾に `deny_message: ...` を含める。拒否時に Claude に返す説明文。
//   - ルール改変時は autoMode 側（dot_claude/auto-mode.libsonnet）との整合も検討する。
//     autoMode と ccgate は独立のポリシーセットだが、同じ曖昧性は両方で問題になるので
//     critique 等で明確化した文面は揃えた方がメンテしやすい。
{
  ['$schema']: 'https://raw.githubusercontent.com/tak848/ccgate/main/ccgate.schema.json',
  provider: {
    name: 'anthropic',
    model: 'claude-haiku-4-5',
    timeout_ms: 10000,
  },
  allow: [
    // 読み取り専用操作（GET / read-only API / state 変更なしの query）を許可。
    'Read-Only Operations: GET requests, read-only API calls, or queries that do not modify state.',

    // 現在の repo / worktree 内の read-only 操作を許可。
    'Local Operations: Read-only work inside the current repository or current worktree.',

    // パッケージ/モジュールキャッシュ・システム temp への read-only ファイルアクセスを許可。
    // Go module cache / pnpm|npm|yarn store / Python cache / cargo registry / Homebrew / ~/.cache / /tmp 等。
    // 読み取り専用のみ。書き込み・削除・インストール binary の実行は fallthrough。
    'Library Source Read: Read-only inspection (Read/Glob/Grep, or Bash head/tail/sed -n/cat/less/file/stat/wc/awk) of package and module caches and of system temp. This includes: Go module cache (~/go/pkg/mod, $GOPATH/pkg/mod, ~/Library/Caches/go-build), node package manager stores (pnpm/npm/yarn caches), Python package cache, cargo registry (~/.cargo/registry), Homebrew prefix (/opt/homebrew, /usr/local), ~/.cache/*, /usr/include, system temp (/tmp, /var/folders), and installed tool binaries. These cannot contain user private source repos. This rule is READ-ONLY file access only — any write, delete, or execute (including running an installed binary) MUST fallthrough.',

    // draft=true の PR 作成は即時許可。非 draft 化 / body-title 変更 / 既存 PR branch への push 等は
    // 非 draft PR 作成と同等扱いで user confirmation が必要 → fallthrough。
    'Draft PR Creation: If the operation creates a pull request AND draft is true in tool_input_raw, allow immediately. If draft is false or absent, fallthrough. Marking an existing draft PR as ready-for-review, converting non-draft → draft, updating the title/body of an existing PR, or pushing commits to a PR branch the agent did not create in this session all require user confirmation — fallthrough.',

    // project の task runner 経由で「明らかに build/test/lint/format」の script を実行するのを許可。
    // manifest に script があるから allow、ではなく script の意味が build/test/lint/format であること。
    // deploy / migration / prod 対象 / 共有 system への write は fallthrough（user intent 必要）。
    'Local Development: Project-declared build/test/lint/format scripts invoked via the project\'s own task runner — e.g. `pnpm run <script>`, `uv run <script>`, `cargo test`, `go test`, `make <target>` — where the target is visibly a build/test/lint/format step and does NOT chain into deploys, migrations, or network writes to shared systems. Arbitrary scripts (e.g. `make deploy`, `pnpm run build:prod`, `pytest tests/integration/ --prod-db`, `./scripts/seed-staging.sh`) MUST fallthrough even if declared in a manifest; deploy / migration / prod-target scripts require explicit user intent.',

    // 保護 branch 以外の branch に対する git 操作を許可。保護 branch は default branch（main/master）と
    // release/deploy/env 系の長寿命 branch（releases/* / hotfix/* / deploy/* / env/* / prod / stg 等）。
    'Git Feature Branch: Git operations on non-protected branches. Protected branches include the default branch (main, master) and long-lived deployment branches (release/*, releases/*, hotfix/*, deploy/*, env/*, prod, production, stg, staging, or similarly named branches whose semantics indicate deployment / environment / release targeting).',

    // current repo 内で manifest に既に宣言済みの依存を install/sync する操作を許可。
    // `pnpm install` / `uv sync` / `cargo build` / `bundle install` 等。
    // `go mod tidy` は session で import 追加していない場合のみ OK（正規化目的）、
    // session で import 追加済みなら新規依存導入になるので fallthrough。
    // agent が任意のパッケージ名を指定する install（`pnpm add foo` / `uv add bar` / `go get <pkg>` 等）は fallthrough。
    'Package Manager Install: Package manager install/sync commands in the current repository that DO NOT install agent-chosen package names — e.g. `pnpm install`, `uv sync`, `cargo build`, `bundle install`. `go mod tidy` is allowed as a normalisation step when the agent has not added new imports in this session; if new imports were added, `go mod tidy` introduces new dependencies and MUST fallthrough. Agent-chosen installations (`pnpm add foo`, `uv add bar`, `go get <pkg>`) MUST fallthrough.',
  ],
  deny: [
    // 強制 push・リモートブランチ削除・publish 済み履歴書き換えを禁止。
    // ただし recent_transcript / tool_input.description で user が明示的に指示した場合は fallthrough。
    'Git Destructive: force push (--force), deleting remote branches (push --delete), or rewriting published history. Check recent_transcript and tool_input.description — if the user explicitly requested the operation, fallthrough instead of deny. deny_message: Destructive git operation. Confirm explicit user instruction.',

    // worktree 境界保護。is_worktree=true のとき primary_checkout_root や他 sibling worktree への
    // アクセスを tool 問わず全面禁止。ABSOLUTE = in-session の user authorization では解除不可。
    // 必要なら user が session を抜けて --add-dir 付きで再起動する。
    // package/module cache / /tmp / 別 repo は対象外で別ルールが管理する。
    'Sibling Checkout Access (ABSOLUTE): When is_worktree is true, ANY access (read or write, via any tool, no exceptions) to paths under primary_checkout_root or under other sibling worktree checkouts of the SAME repository MUST be denied. ABSOLUTE means this is NOT cleared by in-session user authorization — if sibling-worktree access is genuinely needed, the user must restart the session with --add-dir targeting that path. This rule is strictly about same-repo cross-checkout confusion — it does NOT apply to package/module caches, /tmp, or unrelated repositories (those are handled by other rules). deny_message: Accessing another checkout of the same repository is forbidden. Stay within this worktree.',

    // --add-dir で明示認可されていない別 git repo への read/write を禁止。
    // ~/repos/* / ~/projects/* / ~/src/* / ~/work/* / ~/code/* / ~/ghq/* 等（あるいは repo_root 外で
    // 別の git repo 配下にあるパス）は private source repo 扱い。
    // `cd` でそこへ移動した後に触る形のすり抜けも対象（動作対象のパスで判定する）。
    // package/module cache や /tmp は対象外（Library Source Read に fallthrough）。
    // 判別がつかないケースは deny ではなく fallthrough。
    'Unrelated Repository Access: Reading or writing files inside a DIFFERENT git repository the user has NOT explicitly authorized via --add-dir. A path under ~/repos/*, ~/projects/*, ~/src/*, ~/work/*, ~/code/*, ~/ghq/* (or any path outside repo_root that is itself inside a git repository) should be denied when it is NOT a package/module cache — these look like user source repos. This applies even when reached via `cd` earlier in the session; detection is against the actual path touched by each action, not the current working directory alone. If the path is clearly a package/module cache or /tmp, this rule does NOT apply and the operation should fall through to Library Source Read. When genuinely uncertain whether a path is a private repo or a cache, fallthrough (not deny). deny_message: Reading outside authorized repositories. Ask the user to --add-dir this path if intentional.',

    // 単発 / ad-hoc package runner で declared manifest 外のコードを fetch 実行するのを禁止。
    // npx / pnpx / pnpm dlx / bunx / uvx / uv tool run / go run <remote-path> 等が対象。
    // `pnpm exec <bin>` も devDependency であっても禁止。`pnpm run <script>` に wrap すること。
    'Direct Tool Invocation: Running tools via one-shot or ad-hoc package runners that fetch and execute code outside the declared manifest — e.g. `npx`, `pnpx`, `pnpm dlx`, `bunx`, `uvx`, `uv tool run`, or `go run <remote-path>` such as `go run github.com/foo/bar@latest`. Rationale: one-shot runners fetch and execute code not pinned by the repo\'s manifest. `pnpm exec <bin>` is also denied even when the binary is a declared devDependency — wrap it in a `pnpm run <script>` entry instead. deny_message: Direct tool invocation not allowed. Use project-defined scripts.',

    // curl|bash / wget|sh 等、ダウンロードしたコンテンツを shell に流す操作、
    // またはレビュー無しのリモートスクリプト実行を禁止。
    'Download and Execute: Piping downloaded content to a shell (curl|bash, wget|sh, etc.), or executing remote scripts without review. deny_message: Download-and-execute not allowed.',

    // current repo 外を対象とする rm -rf 等の破壊的ファイル操作を禁止（referenced_paths と repo_root を照合）。
    // repo 内の node_modules / dist / build artifact 削除は OK。
    'Out-of-Repo Deletion: rm -rf or destructive file operations targeting paths outside the current repository (check referenced_paths against repo_root). Deletion within the repository (node_modules, dist, build artifacts) is fine. deny_message: Deletion outside repository not allowed.',
  ],
  environment: [
    // session 開始したリポジトリを trusted として扱う。
    '**Trusted repo**: The git repository the session started in.',

    // sibling worktree は "Sibling Checkout Access (ABSOLUTE)" deny と整合する強さで off-limits と宣言。
    // current worktree とそのファイルのみが作業対象で、primary checkout / sibling worktree への fallback は不可。
    '**Current worktree context**: Sibling worktrees of the same repository are off-limits by default — see the "Sibling Checkout Access (ABSOLUTE)" deny rule. The current worktree and its own files are the only in-scope area; do not fall back to primary checkout or sibling worktrees for reference material.',
  ],
}

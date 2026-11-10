local shellSingleQuote(s) =
  "'" + std.strReplace(s, "'", "'\"'\"'") + "'";

// reason を持つルールは、PreToolUse hook で reason を additionalContext として Claude に渡す。
// 拒否そのものは permissions.deny（permissionsDeny に同じ spec を登録済み）が行う。
//
// Bash のルールは hooks[].if で絞らず、cc-deny-hint（go/cmd/cc-deny-hint）がコマンドを単純コマンドに
// 分けて照合する。if はコマンド名より先まで指定したパターンだと、$()・バッククォート・$VAR を含む
// コマンドで常に hook を実行する（公式 docs の hooks "How if patterns match Bash commands"。仕様で
// あってリグレッションではない）。そのため無関係なコマンドにも全ルールの reason が注入されていた。
// Bash 以外のルールに reason を付けたときは、if で正しく絞れるので if と printf で返す。
local preToolResponse(reason) = {
  hookSpecificOutput: {
    hookEventName: 'PreToolUse',
    additionalContext: '[deny hint] このコマンドは permissions.deny にマッチする可能性があります。背景: ' + reason,
  },
};

local worktreeReason = 'worktree は git worktree コマンドではなく、EnterWorktree か subagent の isolation: worktree で作ってください（.worktreeinclude のコピーと後片付けが効きます）。ユーザーが作った worktree は消したり動かしたりしないでください。一覧（git worktree list）は使えます。';

local rules = [
  // 今のリポジトリで不要な -C を付けるのを止めるためのもの（#400）。理由を返さないと、別のリポジトリで
  // 読むだけの git まで黙って止められ、リポジトリをまたぐ作業そのものが禁止されていると読み違える。
  {
    matcher: 'Bash',
    spec: 'Bash(git -C*)',
    reason: 'git -C は使わないでください。今のリポジトリでは git をそのまま実行し、別のリポジトリでは cd <そのリポジトリ> && git … の形で実行してください。リポジトリをまたぐ作業そのものを禁止するルールではありません。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(git -c commit.gpgsign=false*)',
    reason: 'commit.gpgsign=false による GPGサインのスキップは禁止です。ユーザーが離席しているだけかもしれません。必要な場合はユーザーに確認を取ってから実行してください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(git commit*--amend*)',
    reason: 'git commit --amend は完全に禁止です。今後このオプションを使おうとしないでください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(git commit*--no-gpg-sign*)',
    reason: 'GPGサインのスキップは禁止です。ユーザーが離席しているだけかもしれません。サインなしで commit する必要があるなら、必ずユーザーに確認を取ってから実行してください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(git reset*--hard*)',
  },
  // AI が worktree を作るのは EnterWorktree と subagent の isolation: worktree だけにする。Claude Code が
  // git で作る worktree（<repo>/.claude/worktrees/<name>）には .worktreeinclude のコピーと後片付け（変更の
  // 無い subagent の worktree の削除、定期の sweep）が効くが、Bash の git worktree add で作ったものには効かない。
  // remove / move / prune は、ユーザーが gwc で作った worktree を消したり動かしたりしないよう止める。
  // git worktree list は通す。
  {
    matcher: 'Bash',
    spec: 'Bash(git worktree add*)',
    reason: worktreeReason,
  },
  {
    matcher: 'Bash',
    spec: 'Bash(git worktree remove*)',
    reason: worktreeReason,
  },
  {
    matcher: 'Bash',
    spec: 'Bash(git worktree move*)',
    reason: worktreeReason,
  },
  {
    matcher: 'Bash',
    spec: 'Bash(git worktree prune*)',
    reason: worktreeReason,
  },
  {
    matcher: 'Bash',
    spec: 'Bash(git merge*--squash*)',
    reason: 'git merge --squash は禁止です。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(go generate*)',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(go install*)',
    reason: 'go install は禁止です。ツール管理はプロジェクトの環境管理に従ってください。場合によっては .mise.local.toml を使って管理してください。',
  },
  {
    matcher: 'Read',
    spec: 'Read(.envrc.local)',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(npm*)',
    reason: 'npm は禁止です。pnpm を使用してください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(npx*)',
    reason: 'npx は registry から package を一時取得して実行できるため禁止です。必要な CLI は依存に宣言し、package.json の script として pnpm run <script> または pnpm <script> で実行してください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(pnpm dlx*)',
    reason: 'pnpm dlx は registry から package を一時取得して実行するため禁止です。必要な CLI は依存に宣言し、package.json の script として pnpm run <script> または pnpm <script> で実行してください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(pnpm exec*)',
    reason: 'pnpm exec は package.json の scripts を迂回して任意の依存 bin を直接実行できるため禁止です。必要なコマンドは package.json に script として定義し、pnpm run <script> または pnpm <script> で実行してください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(pnpx*)',
    reason: 'pnpx は pnpm dlx の alias であり、registry から package を一時取得して実行するため禁止です。必要な CLI は依存に宣言し、package.json の script として pnpm run <script> または pnpm <script> で実行してください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(pnx*)',
    reason: 'pnx は pnpm dlx の alias であり、registry から package を一時取得して実行するため禁止です。必要な CLI は依存に宣言し、package.json の script として pnpm run <script> または pnpm <script> で実行してください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(bunx*)',
    reason: 'bunx は registry から package を一時取得して実行できるため禁止です。必要な CLI は依存に宣言し、package.json の script として pnpm run <script> または pnpm <script> で実行してください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(pip *)',
    reason: 'pip は禁止です。uv を使用してください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(pip3 *)',
    reason: 'pip3 は禁止です。uv を使用してください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(pnpm --dir*)',
    reason: 'pnpm --dir は禁止です。--filter を使用してください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(pnpm -C*)',
    reason: 'pnpm -C は禁止です。--filter を使用してください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(python*)',
    reason: 'システムの python を直接使用することは禁止です。最低限 uv run または uvx を使ってください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(python3*)',
    reason: 'システムの python3 を直接使用することは禁止です。最低限 uv run または uvx を使ってください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(perl*)',
    reason: 'perl が最適なのはよく分かるけど、awk, sed, jq, シェルスクリプト等で代替できないか考えてみてください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(ruby*)',
    reason: 'ruby の直接実行は禁止です。awk, sed, jq, シェルスクリプト等で代替できないか、それでも足りなければ Go で書けないか考えてみてください。',
  },
];

local hinted = [rule for rule in rules if std.objectHas(rule, 'reason')];

// 'Bash(<pattern>)' から pattern を取り出す。
local bashPattern(spec) =
  assert std.startsWith(spec, 'Bash(') && std.endsWith(spec, ')') : 'unexpected Bash spec: ' + spec;
  std.substr(spec, std.length('Bash('), std.length(spec) - std.length('Bash()'));

{
  rules: rules,
  permissionsDeny: [rule.spec for rule in rules],
  preToolUseHooks: [
    {
      matcher: 'Bash',
      hooks: [
        {
          type: 'command',
          command: '~/.claude/bin/cc-deny-hint ' + shellSingleQuote(std.manifestJsonMinified([
            { pattern: bashPattern(rule.spec), reason: rule.reason }
            for rule in hinted
            if rule.matcher == 'Bash'
          ])),
          timeout: 5,
        },
      ],
    },
  ] + [
    {
      matcher: rule.matcher,
      hooks: [
        {
          type: 'command',
          ['if']: rule.spec,
          command: "printf '%s' " + shellSingleQuote(std.manifestJsonEx(preToolResponse(rule.reason), '')),
        },
      ],
    }
    for rule in hinted
    if rule.matcher != 'Bash'
  ],
}

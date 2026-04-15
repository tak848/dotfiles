local shellSingleQuote(s) =
  "'" + std.strReplace(s, "'", "'\"'\"'") + "'";

// Claude Code v2.1.105 以降、hooks[].if の評価が shell expansion ($(...), $$, ${VAR} 等) を
// 含むコマンドで fall-open するリグレッションがあり、deny を返すと暴発で大量の false positive
// deny が発生してコマンド実行が阻害される。
// そこで permissionDecision は指定せず additionalContext のみで reason を Claude に届ける暫定措置。
// 本物の deny 判定は permissions.deny 側 (permissionsDeny に同じ spec を登録済み) に完全委任する。
// 上流修正が入ったら permissionDecision: 'deny' 方式に戻す。
local preToolResponse(reason) = {
  hookSpecificOutput: {
    hookEventName: 'PreToolUse',
    additionalContext: '[deny hint] このコマンドは permissions.deny にマッチする可能性があります。背景: ' + reason,
  },
};

local rules = [
  {
    matcher: 'Bash',
    spec: 'Bash(git -C*)',
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
  {
    matcher: 'Bash',
    spec: 'Bash(git worktree*)',
    reason: 'git worktree の操作は禁止です。worktree の管理はユーザーが行います。',
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
    reason: 'npx は禁止です。プロジェクトのスクリプトを使用してください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(pnpx*)',
    reason: 'pnpx は禁止です。プロジェクトのスクリプトを使用してください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(bunx*)',
    reason: 'bunx は禁止です。プロジェクトのスクリプトを使用してください。',
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
    spec: 'Bash(pnpm exec*)',
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
];

{
  rules: rules,
  permissionsDeny: [rule.spec for rule in rules],
  preToolUseHooks: [
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
    for rule in rules
    if std.objectHas(rule, 'reason')
  ],
}

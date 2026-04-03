local shellSingleQuote(s) =
  "'" + std.strReplace(s, "'", "'\"'\"'") + "'";

local preToolResponse(reason) = {
  hookSpecificOutput: {
    hookEventName: 'PreToolUse',
    permissionDecision: 'deny',
    permissionDecisionReason: '[auto-rejected: pattern matched] ' + reason,
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
    spec: 'Bash(go generate*)',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(go install*)',
    reason: 'go install は禁止です。ツール管理はプロジェクトの環境管理に従ってください。',
  },
  {
    matcher: 'Read',
    spec: 'Read(.envrc.local)',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(npx*)',
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

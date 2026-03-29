local shellSingleQuote(s) =
  "'" + std.strReplace(s, "'", "'\"'\"'") + "'";

local preToolResponse(reason) = {
  hookSpecificOutput: {
    hookEventName: 'PreToolUse',
    permissionDecision: 'deny',
    permissionDecisionReason: reason,
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
    spec: 'Bash(git commit*-S false*)',
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
    spec: 'Bash(python*)',
    reason: 'システムの python を直接使用することは禁止です。最低限 uv run または uvx を使ってください。',
  },
  {
    matcher: 'Bash',
    spec: 'Bash(python3*)',
    reason: 'システムの python3 を直接使用することは禁止です。最低限 uv run または uvx を使ってください。',
  },
];

local preToolHooks = [
  {
    type: 'command',
    ['if']: rule.spec,
    command: "printf '%s' " + shellSingleQuote(std.manifestJsonEx(preToolResponse(rule.reason), '')),
  }
  for rule in rules
  if std.objectHas(rule, 'reason')
];

{
  rules: rules,
  permissionsDeny: [rule.spec for rule in rules],
  preToolUseHooks:
    if std.length(preToolHooks) == 0 then []
    else [
      {
        matcher: '',
        hooks: preToolHooks,
      },
    ],
}

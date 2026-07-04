#!/usr/bin/env bash
# Commit changes via GitHub GraphQL `createCommitOnBranch` so the commit is
# server-signed (Verified) and authored by the credential owner. With a GitHub
# App installation token in $GH_TOKEN, the commit is attributed to <app>[bot].
#
# Required env:
#   GH_TOKEN     installation token (gh CLI uses this automatically)
#   REPO         owner/name (e.g. tak848/dotfiles)
#   BRANCH       target branch name
#   FILES        space-separated paths (relative to repo root)
#   MESSAGE      commit headline
#
# Optional env:
#   REGENERATE_CMD  shell snippet run before each attempt (must (re)produce $FILES)
#   MAX_ATTEMPTS    integer, defaults to 3

set -euo pipefail

: "${GH_TOKEN:?GH_TOKEN required}"
: "${REPO:?REPO required}"
: "${BRANCH:?BRANCH required}"
: "${FILES:?FILES required}"
: "${MESSAGE:?MESSAGE required}"

REGENERATE_CMD="${REGENERATE_CMD:-}"
MAX_ATTEMPTS="${MAX_ATTEMPTS:-3}"

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

attempt=0
while :; do
  attempt=$((attempt + 1))

  if [ -n "$REGENERATE_CMD" ]; then
    eval "$REGENERATE_CMD"
  fi

  # core.fileMode=false: createCommitOnBranch は executable bit を扱えないため
  # tree mode が 100644 で固定される一方、再生成コマンド (mise generate
  # bootstrap -w 等) は fs に +x を付ける。fs vs index の mode 差を
  # 「変更あり」と誤認すると空 commit を打ち続けるので、ここでは無視する。
  # shellcheck disable=SC2086
  if git -c core.fileMode=false diff --quiet -- $FILES; then
    echo "No changes in: $FILES — nothing to commit."
    exit 0
  fi

  EXPECTED_OID=$(git rev-parse HEAD)

  # ファイル内容は jq の argv に載せない。base64 化した lockfile は単一引数の
  # 上限 (Linux の MAX_ARG_STRLEN = 128KB) を超え得て、--arg/--argjson が
  # "Argument list too long" (E2BIG) で失敗する。--rawfile / --slurpfile で
  # 一時ファイルから読ませ、GraphQL payload は gh に stdin (--input -) で渡す。
  adds_file="$WORKDIR/additions.json"
  input_file="$WORKDIR/input.json"
  payload_file="$WORKDIR/payload.json"

  {
    # shellcheck disable=SC2086
    for f in $FILES; do
      b64="$WORKDIR/content.b64"
      base64 < "$f" | tr -d '\n' >"$b64"
      jq -n --arg path "$f" --rawfile contents "$b64" \
        '{path: $path, contents: $contents}'
    done
  } | jq -s '.' >"$adds_file"

  jq -n \
    --arg repo "$REPO" \
    --arg branch "$BRANCH" \
    --arg oid "$EXPECTED_OID" \
    --arg msg "$MESSAGE" \
    --slurpfile adds "$adds_file" \
    '{
      branch: {repositoryNameWithOwner: $repo, branchName: $branch},
      expectedHeadOid: $oid,
      message: {headline: $msg},
      fileChanges: {additions: $adds[0]}
    }' >"$input_file"

  jq -n \
    --arg query 'mutation($input: CreateCommitOnBranchInput!) {
      createCommitOnBranch(input: $input) { commit { url oid } }
    }' \
    --slurpfile input "$input_file" \
    '{query: $query, variables: {input: $input[0]}}' >"$payload_file"

  if gh api graphql --input - <"$payload_file"; then
    exit 0
  fi

  if [ "$attempt" -ge "$MAX_ATTEMPTS" ]; then
    echo "createCommitOnBranch failed after ${MAX_ATTEMPTS} attempts." >&2
    exit 1
  fi

  echo "Mutation failed; refetching origin/$BRANCH and retrying..." >&2
  git fetch origin "$BRANCH"
  git reset --hard "origin/$BRANCH"
done

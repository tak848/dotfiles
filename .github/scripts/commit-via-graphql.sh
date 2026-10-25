#!/usr/bin/env bash
# Commit changes via GitHub GraphQL `createCommitOnBranch` so the commit is
# server-signed (Verified) and authored by the credential owner. With a GitHub
# App installation token in $GH_TOKEN, the commit is attributed to <app>[bot].
#
# $FILES must already be generated in the work tree. This script never
# regenerates them: workflows regenerate in an earlier step so that third-party
# code never runs while the App token exists.
#
# If the branch moved after checkout (another workflow committed, or someone
# pushed), the mutation is rejected because expectedHeadOid is stale. The script
# then fetches the new head and, if the new head has not changed $FILES since
# checkout, commits the generated files on top of it. If $FILES did change, it
# refuses to overwrite them and fails. A push that changes the inputs of the
# generation starts a new run of the workflow, which regenerates on the new head.
#
# Required env:
#   GH_TOKEN     installation token (gh CLI uses this automatically)
#   REPO         owner/name (e.g. tak848/dotfiles)
#   BRANCH       target branch name
#   FILES        space-separated paths (relative to repo root). A directory
#                covers every file under it, including new and deleted ones.
#   MESSAGE      commit headline
#
# Optional env:
#   MAX_ATTEMPTS    integer, defaults to 3

set -euo pipefail

: "${GH_TOKEN:?GH_TOKEN required}"
: "${REPO:?REPO required}"
: "${BRANCH:?BRANCH required}"
: "${FILES:?FILES required}"
: "${MESSAGE:?MESSAGE required}"

MAX_ATTEMPTS="${MAX_ATTEMPTS:-3}"

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

# core.fileMode=false: createCommitOnBranch は executable bit を扱えないため
# tree mode が 100644 で固定される一方、再生成コマンド (mise generate
# bootstrap -w 等) は fs に +x を付ける。fs vs index の mode 差を
# 「変更あり」と誤認すると空 commit を打ち続けるので、ここでは無視する。
# FILES にはディレクトリも渡せる（mise lockfile revision 2 の sidecar ディレクトリ）。
# 新規ファイル・削除されたファイルも拾うため diff ではなく status を使う。
# -uall で未追跡ディレクトリをファイル単位に展開し、-z で path をそのまま受け取る。
# index は触らないので、変更は work tree 側の列（2 文字目）にだけ出る。
collect_changes() {
  changed=()
  deleted=()
  local entry
  # shellcheck disable=SC2086
  while IFS= read -r -d '' entry; do
    if [ "${entry:1:1}" = "D" ]; then
      deleted+=("${entry:3}")
    else
      changed+=("${entry:3}")
    fi
  done < <(git -c core.fileMode=false status --porcelain=v1 -z -uall -- $FILES)
}

attempt=0
while :; do
  attempt=$((attempt + 1))

  collect_changes
  if [ "${#changed[@]}" -eq 0 ] && [ "${#deleted[@]}" -eq 0 ]; then
    echo "No changes in: $FILES — nothing to commit."
    exit 0
  fi

  EXPECTED_OID=$(git rev-parse HEAD)

  # ファイル内容は jq の argv に載せない。base64 化した lockfile は単一引数の
  # 上限 (Linux の MAX_ARG_STRLEN = 128KB) を超え得て、--arg/--argjson が
  # "Argument list too long" (E2BIG) で失敗する。--rawfile / --slurpfile で
  # 一時ファイルから読ませ、GraphQL payload は gh に stdin (--input -) で渡す。
  adds_file="$WORKDIR/additions.json"
  dels_file="$WORKDIR/deletions.json"
  input_file="$WORKDIR/input.json"
  payload_file="$WORKDIR/payload.json"

  {
    for f in "${changed[@]}"; do
      b64="$WORKDIR/content.b64"
      base64 < "$f" | tr -d '\n' >"$b64"
      jq -n --arg path "$f" --rawfile contents "$b64" \
        '{path: $path, contents: $contents}'
    done
  } | jq -s '.' >"$adds_file"

  {
    for f in "${deleted[@]}"; do
      jq -n --arg path "$f" '{path: $path}'
    done
  } | jq -s '.' >"$dels_file"

  jq -n \
    --arg repo "$REPO" \
    --arg branch "$BRANCH" \
    --arg oid "$EXPECTED_OID" \
    --arg msg "$MESSAGE" \
    --slurpfile adds "$adds_file" \
    --slurpfile dels "$dels_file" \
    '{
      branch: {repositoryNameWithOwner: $repo, branchName: $branch},
      expectedHeadOid: $oid,
      message: {headline: $msg},
      fileChanges: {additions: $adds[0], deletions: $dels[0]}
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

  # 先頭が進んだか確かめる。checkout は persist-credentials: false だが、公開リポジトリなので
  # token なしで取れる。生成物と比べるのに要るのは新しい先頭の tree だけなので depth=1 でよい
  git fetch --quiet --no-tags --depth=1 origin "refs/heads/$BRANCH"
  new_oid=$(git rev-parse FETCH_HEAD)
  if [ "$new_oid" = "$EXPECTED_OID" ]; then
    echo "createCommitOnBranch failed, but $BRANCH still points to $EXPECTED_OID. Not a conflict; not retrying." >&2
    exit 1
  fi

  # mixed reset: HEAD と index だけを新しい先頭に合わせ、work tree の生成物は残す。
  # 次の collect_changes は「生成物と新しい先頭の差分」になる
  # shellcheck disable=SC2086
  if git diff --quiet "$EXPECTED_OID" "$new_oid" -- $FILES; then
    echo "$BRANCH moved from $EXPECTED_OID to $new_oid without touching $FILES. Committing the generated files on top of it..." >&2
    git reset --quiet "$new_oid"
    continue
  fi

  # 新しい先頭が FILES を変えている。前回の試行が実は成功していた（応答だけ失われた）場合など、
  # 中身がすでに生成物と同じなら何もしない。違えば、他人の変更を上書きしないよう止める
  git reset --quiet "$new_oid"
  collect_changes
  if [ "${#changed[@]}" -eq 0 ] && [ "${#deleted[@]}" -eq 0 ]; then
    echo "$BRANCH moved to $new_oid, which already has the generated $FILES — nothing to commit."
    exit 0
  fi
  echo "$BRANCH moved from $EXPECTED_OID to $new_oid and the new head changed $FILES. Not overwriting them." >&2
  echo "Re-run this workflow to regenerate on the new head (checkout takes the branch head, not the pushed SHA)." >&2
  exit 1
done

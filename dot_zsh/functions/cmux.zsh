# cmux (https://github.com/manaflow-ai/cmux / com.cmuxterm.app) 連携

# cmux 環境（コマンドを実行しているワークスペースが特定できる）かどうか
_cmux_available() {
    [ -n "$CMUX_WORKSPACE_ID" ] && command -v cmux >/dev/null 2>&1
}

# 「コマンドを実行している」cmux ワークスペースをリネーム。
# 注: cmux current-workspace は「選択中（フォーカス中）」のものを返すため使わない。
#     コマンド実行元のワークスペースは環境変数 CMUX_WORKSPACE_ID で特定する。
# 呼び出し側で _cmux_available を確認してから呼ぶこと。
_cmux_rename_current_workspace() {
    local title="$1"
    cmux workspace rename "$CMUX_WORKSPACE_ID" --title "$title"
}

# lcm (Linear-CMux): Linear URL/identifier から現在の cmux ワークスペース名を設定する。
#
# 使用例:
#   lcm ENG-123                                       # identifier を直接指定
#   lcm https://linear.app/acme/issue/ENG-123/...     # Linear URL を指定
#
# 設定される名前: <repo>[<identifier>] <title>（例: dotfiles[ENG-123] Hogehoge）
#   - git リポジトリ内なら repo はワークツリールートの basename
#   - git リポジトリ外なら repo を省略して [<identifier>] <title> になる
# cmux 環境でなければエラー。Linear 情報取得には GWC_LINEAR_API_KEY が必要。
lcm() {
    if [ -z "$1" ]; then
        echo "使い方: lcm <Linear URL | identifier(例 ENG-123)>" >&2
        return 1
    fi
    if ! _cmux_available; then
        echo "エラー: lcm は cmux 環境でのみ使用できます。" >&2
        return 1
    fi

    # Linear issue 情報を取得（_linear_fetch_issue は git-worktree.zsh で定義）
    local issue
    issue=$(_linear_fetch_issue "$1") || return 1
    local identifier title
    identifier=$(echo "$issue" | jq -r '.identifier')
    title=$(echo "$issue" | jq -r '.title')

    # repo 名: git リポジトリ内ならワークツリールートの basename、そうでなければ省略
    local repo_prefix="" root_dir
    root_dir=$(git rev-parse --show-toplevel 2>/dev/null)
    [ -n "$root_dir" ] && repo_prefix="$(basename "$root_dir")"

    local ws_title
    if [ -n "$repo_prefix" ]; then
        ws_title="${repo_prefix}[${identifier}] ${title}"
    else
        ws_title="[${identifier}] ${title}"
    fi

    if _cmux_rename_current_workspace "$ws_title"; then
        echo "cmux workspace 名を設定: $ws_title"
    else
        echo "エラー: cmux workspace のリネームに失敗しました。" >&2
        return 1
    fi
}

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

# 「コマンドを実行している」cmux ワークスペースを閉じる。
# socket API 直叩きのため GUI の確認ダイアログは出ない。実行元シェルごと終了する。
# 呼び出し側で _cmux_available を確認してから呼ぶこと。
_cmux_close_current_workspace() {
    cmux workspace close "$CMUX_WORKSPACE_ID"
}

# lcm (Linear-CMux): Linear URL/identifier から現在の cmux ワークスペース名を設定する。
#
# 使用例:
#   lcm                                               # 引数なし: 現在のブランチ名から逆引き
#   lcm ENG-123                                       # identifier を直接指定
#   lcm eng-123                                       # 小文字でも可（大文字に正規化）
#   lcm https://linear.app/acme/issue/ENG-123/...     # Linear URL を指定
#
# 設定される名前: <repo>[<identifier>] <title>（例: dotfiles[ENG-123] Hogehoge）
#   - git リポジトリ内なら repo はリポジトリ名（worktree のディレクトリ名ではない）
#   - git リポジトリ外なら repo を省略して [<identifier>] <title> になる
# cmux 環境でなければエラー。Linear 情報取得には GWC_LINEAR_API_KEY が必要。
lcm() {
    if ! _cmux_available; then
        echo "エラー: lcm は cmux 環境でのみ使用できます。" >&2
        return 1
    fi

    # 引数なしの場合は現在のブランチ名から Linear ticket を逆引きする。
    # ブランチ名（例 tak848/eng-123-foo）の識別子抽出・大文字正規化は _linear_fetch_issue 側で行う。
    local ref="$1"
    if [ -z "$ref" ]; then
        ref=$(git rev-parse --abbrev-ref HEAD 2>/dev/null)
        if [ -z "$ref" ] || [ "$ref" = "HEAD" ]; then
            echo "エラー: 引数がなく、現在のブランチ名も取得できませんでした。" >&2
            echo "使い方: lcm [Linear URL | identifier(例 ENG-123)]" >&2
            return 1
        fi
        if ! echo "$ref" | grep -oiE '[A-Z][A-Z0-9]*-[0-9]+' >/dev/null; then
            echo "エラー: 現在のブランチ '$ref' から Linear identifier を抽出できませんでした。" >&2
            return 1
        fi
        echo "ブランチ '$ref' から Linear ticket を逆引きします。"
    fi

    # Linear issue 情報を取得（_linear_fetch_issue は git-worktree.zsh で定義）
    local issue
    issue=$(_linear_fetch_issue "$ref") || return 1
    local identifier title
    identifier=$(echo "$issue" | jq -r '.identifier')
    title=$(echo "$issue" | jq -r '.title')

    # repo 名: git リポジトリ内ならリポジトリ名（_git_repo_name）、そうでなければ省略
    local repo_prefix=""
    if git rev-parse --show-toplevel >/dev/null 2>&1; then
        repo_prefix="$(_git_repo_name)"
    fi

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

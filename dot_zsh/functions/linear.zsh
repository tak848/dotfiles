# Linear 連携（cmux に依存しない汎用コマンド）

# lop (Linear OPen): Linear issue をブラウザで開く。
#
# 使用例:
#   lop                                               # 引数なし: 現在のブランチ名から逆引き
#   lop ENG-123                                       # identifier を直接指定
#   lop eng-123                                       # 小文字でも可（大文字に正規化）
#   lop https://linear.app/acme/issue/ENG-123/...     # Linear URL を指定
#
# Linear 情報取得には GWC_LINEAR_API_KEY が必要。
# _linear_fetch_issue は git-worktree.zsh で定義。
lop() {
    # 引数なしの場合は現在のブランチ名から Linear ticket を逆引きする。
    # ブランチ名（例 tak848/eng-123-foo）の識別子抽出・大文字正規化は _linear_fetch_issue 側で行う。
    local ref="$1"
    if [ -z "$ref" ]; then
        ref=$(git rev-parse --abbrev-ref HEAD 2>/dev/null)
        if [ -z "$ref" ] || [ "$ref" = "HEAD" ]; then
            echo "エラー: 引数がなく、現在のブランチ名も取得できませんでした。" >&2
            echo "使い方: lop [Linear URL | identifier(例 ENG-123)]" >&2
            return 1
        fi
        if ! echo "$ref" | grep -oiE '[A-Z][A-Z0-9]*-[0-9]+' >/dev/null; then
            echo "エラー: 現在のブランチ '$ref' から Linear identifier を抽出できませんでした。" >&2
            return 1
        fi
        echo "ブランチ '$ref' から Linear ticket を逆引きします。"
    fi

    local issue
    issue=$(_linear_fetch_issue "$ref") || return 1
    local identifier title url
    identifier=$(echo "$issue" | jq -r '.identifier')
    title=$(echo "$issue" | jq -r '.title')
    url=$(echo "$issue" | jq -r '.url // empty')
    if [ -z "$url" ]; then
        echo "エラー: Linear ($identifier) の URL を取得できませんでした。" >&2
        return 1
    fi

    echo "Linear issue を開きます: [${identifier}] ${title}"
    echo "$url"
    if command -v open >/dev/null 2>&1; then
        open "$url"
    elif command -v xdg-open >/dev/null 2>&1; then
        xdg-open "$url"
    else
        echo "エラー: open / xdg-open が見つかりません。上記 URL を手動で開いてください。" >&2
        return 1
    fi
}

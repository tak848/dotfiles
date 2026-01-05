# ghq ラッパー: 引数なしで fzf 選択 → cd
ghq() {
    if ! command -v fzf >/dev/null 2>&1; then
        echo "エラー: fzf が必要です" >&2
        return 1
    fi

    if [ $# -eq 0 ]; then
        # 引数なし: fzf で選択して cd
        local repo_path=$(command ghq list | fzf --height 40% --reverse)
        if [ -n "$repo_path" ]; then
            cd "$(command ghq root)/$repo_path"
        fi
    else
        # 引数あり: 通常の ghq コマンド
        command ghq "$@"
    fi
}

# repo: ghq ラッパーのエイリアス（短縮形）
alias repo='ghq'

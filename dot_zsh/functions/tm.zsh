# tm: ghq リポジトリ → tmux session を作成/切り替え
tm() {
  if ! command -v tmux >/dev/null 2>&1; then
    echo "エラー: tmux が必要です" >&2
    return 1
  fi

  local change
  [[ -n "${TMUX:-}" ]] && change="switch-client" || change="attach-session"

  # 引数ありの場合: 名前指定で切り替え
  if [[ $# -eq 1 ]]; then
    tmux list-sessions -F "#{session_name}" 2>/dev/null |
      grep -qE "^${1}$" ||
      tmux new-session -d -s "$1"
    tmux $change -t "$1"
    return
  fi

  # 引数なし: ghq list + fzf でリポジトリ選択
  local repo_path
  repo_path=$(command ghq list | fzf --tmux center,80% --reverse --prompt="tmux session> ")
  [[ -z "$repo_path" ]] && return

  local repo_dir="$(command ghq root)/$repo_path"
  # セッション名: 最後の2コンポーネント (org/repo)、. と : を - に置換
  local session
  session=$(echo "$repo_path" | awk -F/ '{print $(NF-1)"/"$NF}' | tr '.,:' '---')

  tmux list-sessions -F "#{session_name}" 2>/dev/null |
    grep -qE "^${session}$" ||
    tmux new-session -d -c "$repo_dir" -s "$session"
  tmux $change -t "$session"
}

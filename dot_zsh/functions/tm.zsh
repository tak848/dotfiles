# tm: ghq リポジトリ → tmux session を作成/切り替え
#   tm       → カレントディレクトリが ghq 管理下ならそのセッション、そうでなければ fzf 選択
#   tm .     → カレントディレクトリでセッション作成（ghq 管理外でも可）
#   tm <name> → 指定名でカレントディレクトリにセッション作成/切り替え
tm() {
  if ! command -v tmux >/dev/null 2>&1; then
    echo "エラー: tmux が必要です" >&2
    return 1
  fi

  local change
  [[ -n "${TMUX:-}" ]] && change="switch-client" || change="attach-session"

  # カレントディレクトリから ghq 相対パスを取得するヘルパー
  _tm_ghq_relpath() {
    local ghq_root
    ghq_root=$(command ghq root 2>/dev/null) || return 1
    local cwd="${PWD}"
    if [[ "$cwd" == "$ghq_root"/* ]]; then
      echo "${cwd#$ghq_root/}"
      return 0
    fi
    return 1
  }

  # ghq 相対パスからセッション名を生成（. と : を - に置換）
  _tm_session_name() {
    echo "$1" | tr '.,:' '---'
  }

  # セッションが存在しなければ作成
  _tm_create_if_needed() {
    local session="$1" dir="$2"
    tmux list-sessions -F "#{session_name}" 2>/dev/null |
      grep -qE "^${session}$" ||
      tmux new-session -d -c "$dir" -s "$session"
  }

  # tm . → カレントディレクトリでセッション作成
  if [[ "${1:-}" == "." ]]; then
    local repo_path session
    repo_path=$(_tm_ghq_relpath)
    if [[ -n "$repo_path" ]]; then
      session=$(_tm_session_name "$repo_path")
    else
      # ghq 管理外: ディレクトリ名をセッション名に
      session=$(basename "$PWD" | tr '.,:' '---')
    fi
    _tm_create_if_needed "$session" "$PWD"
    tmux $change -t "$session"
    return
  fi

  # tm <name> → 指定名でカレントディレクトリにセッション作成
  if [[ $# -eq 1 ]]; then
    _tm_create_if_needed "$1" "$PWD"
    tmux $change -t "$1"
    return
  fi

  # tm (引数なし) → カレントディレクトリが ghq 管理下ならそのセッション
  local repo_path
  repo_path=$(_tm_ghq_relpath)
  if [[ -n "$repo_path" ]]; then
    local session
    session=$(_tm_session_name "$repo_path")
    _tm_create_if_needed "$session" "$PWD"
    tmux $change -t "$session"
    return
  fi

  # ghq 管理外 → fzf でリポジトリ選択
  repo_path=$(command ghq list | fzf --tmux center,80% --reverse --prompt="tmux session> ")
  [[ -z "$repo_path" ]] && return

  local repo_dir="$(command ghq root)/$repo_path"
  local session
  session=$(_tm_session_name "$repo_path")
  _tm_create_if_needed "$session" "$repo_dir"
  tmux $change -t "$session"
}

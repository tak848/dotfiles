# Git worktree間の移動を楽に
gwt() {
    # fzfコマンドの存在をチェック
    if ! command -v fzf >/dev/null 2>&1; then
        echo "エラー: この関数にはfzfが必要です。" >&2
        return 1
    fi

    # --- 1. 現在のワークツリーのルートと相対パスを取得 ---
    local root_dir
    root_dir=$(git rev-parse --show-toplevel 2>/dev/null)
    if [ -z "$root_dir" ]; then
        echo "エラー: Gitリポジトリではありません。" >&2
        return 1
    fi
    local current_dir=$(pwd)
    local relative_path=""
    if [[ "$current_dir" == "$root_dir"* ]]; then
        relative_path=${current_dir#$root_dir}
    fi

    # --- 2. 移動先のワークツリーをfzfで選択 ---
    # 現在のワークツリーを除いたリストを作成
    local other_worktrees=$(git worktree list | grep -v " ${root_dir} ")
    if [ -z "$other_worktrees" ]; then
        echo "移動可能な他のワークツリーがありません。"
        return 0
    fi

    # fzfで対話的に選択
    local selected_line=$(echo "$other_worktrees" | fzf --height=10 --prompt="Switch to which worktree?: ")

    # --- 3. 選択されたワークツリーに移動 ---
    if [ -n "$selected_line" ]; then
        # 選択された行からワークツリーのパスだけを抽出
        local selected_worktree_root=$(echo "$selected_line" | awk '{print $1}')

        # 最終的な移動先ディレクトリを決定
        local target_dir="$selected_worktree_root"
        # 記憶しておいた相対パスが、移動先のワークツリーにも存在すれば、そこを目的地にする
        if [[ -n "$relative_path" && -d "${selected_worktree_root}${relative_path}" ]]; then
            target_dir="${selected_worktree_root}${relative_path}"
        fi

        echo "Switching to: $target_dir"
        cd "$target_dir"
    else
        echo "キャンセルしました。"
    fi
}

# git worktree remove
gwr() {
    # メインのワークツリーは削除対象外にする
    local worktree_to_remove=$(git worktree list | awk 'NR>1 {print $1 " " $3}' | fzf --prompt="Select worktree to REMOVE: " | awk '{print $1}')

    if [ -n "$worktree_to_remove" ]; then
        # 最終確認
        read -q "REPLY?Really remove worktree at '$worktree_to_remove'? [y/N] "
        echo
        if [[ "$REPLY" =~ ^[Yy]$ ]]; then
            echo "Removing worktree: $worktree_to_remove"
            git worktree remove "$worktree_to_remove"
        else
            echo "Canceled."
        fi
    fi
}

# gwc: 既存ブランチ選択と新規ブランチ作成を兼ねる万能版
#
# 使用例:
#   gwc                              # 通常の使用
#   gwc --copy data.local.json       # 追加ファイルを指定
#   gwc --pr https://github.com/owner/repo/pull/123  # PR URL から worktree 作成
#   gwc --pr 123                     # PR 番号から worktree 作成（同じリポジトリ内）
#   gwc --pr 123 --copy data.json    # PR と追加ファイルを指定
#   export GWC_COPY_FILES=".env.test,config.local.json"  # 環境変数で事前設定
#
gwc() {
    # 元のディレクトリを保存
    local original_dir=$(pwd)

    local default_copy_files=(".envrc.local" ".env.local" "settings.local.json" "CLAUDE.local.md" ".mcp.json" ".serena" "config.toml", ".gemini/settings.json", ".mise.local.toml")
    local extra_copy_files=()
    local pr_ref=""
    local pr_mode=false

    # 環境変数 GWC_COPY_FILES から追加のコピー対象ファイルを取得
    if [ -n "$GWC_COPY_FILES" ]; then
        IFS=',' read -r -A env_copy_files <<< "$GWC_COPY_FILES"
        extra_copy_files+=("${env_copy_files[@]}")
    fi

    # コマンドラインオプションを解析
    while [[ $# -gt 0 ]]; do
        case "$1" in
        --copy)
            if [ -n "$2" ]; then
                extra_copy_files+=("$2")
                shift # --copy を消費
                shift # ファイル名を消費
            else
                echo "エラー: --copy オプションにはファイル名が必要です。" >&2
                return 1
            fi
            ;;
        --pr)
            if [ -n "$2" ]; then
                pr_ref="$2"
                shift # --pr を消費
                shift # PR URL/番号を消費
            else
                echo "エラー: --pr オプションには PR の URL または番号が必要です。" >&2
                return 1
            fi
            ;;
        *)
            echo "エラー: 不明なオプション '$1'" >&2
            return 1
            ;;
        esac
    done

    # 最終的なコピー対象リストを結合
    local copy_files=("${default_copy_files[@]}" "${extra_copy_files[@]}")

    if ! command -v fzf >/dev/null 2>&1; then
        echo "fzf is required"
        return 1
    fi

    # ★★★ 改善点1: 元のサブディレクトリを記憶 ★★★
    local root_dir=$(git rev-parse --show-toplevel)
    local current_dir=$(pwd)
    local relative_path=""
    # 現在地がリポジトリ内であれば、ルートからの相対パスを計算
    if [[ "$current_dir" == "$root_dir"* ]]; then
        relative_path=${current_dir#$root_dir}
    fi
    # ★★★ ここまで ★★★

    local project_name=$(basename "$root_dir")
    local worktree_path

    # --- PR モード: gh pr view でブランチ情報を取得 ---
    if [ -n "$pr_ref" ]; then
        # gh コマンドの存在確認
        if ! command -v gh >/dev/null 2>&1; then
            echo "エラー: --pr オプションには gh (GitHub CLI) が必要です。" >&2
            return 1
        fi

        # 現在のリポジトリの owner/repo を取得
        local current_repo
        current_repo=$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null)
        if [ $? -ne 0 ] || [ -z "$current_repo" ]; then
            echo "エラー: 現在のリポジトリ情報を取得できませんでした。" >&2
            return 1
        fi

        # gh pr view で PR 情報を取得（URL でも PR 番号でも対応）
        local pr_info
        pr_info=$(gh pr view "$pr_ref" --json headRefName,headRepository,headRepositoryOwner 2>/dev/null)
        if [ $? -ne 0 ] || [ -z "$pr_info" ]; then
            echo "エラー: PR '$pr_ref' から情報を取得できませんでした。" >&2
            return 1
        fi

        # PR の head リポジトリを取得して比較（大文字小文字を無視）
        # 注: headRepository.owner は存在しない。headRepositoryOwner が別フィールドとして存在
        local pr_repo_owner pr_repo_name pr_repo
        pr_repo_owner=$(echo "$pr_info" | jq -r '.headRepositoryOwner.login // empty')
        pr_repo_name=$(echo "$pr_info" | jq -r '.headRepository.name // empty')
        if [ -z "$pr_repo_owner" ] || [ -z "$pr_repo_name" ]; then
            echo "エラー: PR のリポジトリ情報を取得できませんでした。" >&2
            return 1
        fi
        pr_repo="${pr_repo_owner}/${pr_repo_name}"

        # 大文字小文字を無視して比較
        if [ "${current_repo:l}" != "${pr_repo:l}" ]; then
            echo "エラー: PR のリポジトリ ($pr_repo) が現在のリポジトリ ($current_repo) と一致しません。" >&2
            return 1
        fi

        local pr_branch
        pr_branch=$(echo "$pr_info" | jq -r '.headRefName')

        echo "PR からブランチ '$pr_branch' を取得しました。"

        # リモートを fetch して最新状態に
        echo "リモートからブランチを取得中..."
        git fetch origin "$pr_branch"

        # worktree を作成
        local dir_name="${project_name}-$(echo "$pr_branch" | sed 's/\//-/g')"
        worktree_path="${root_dir}/../${dir_name}"

        echo "ブランチ '$pr_branch' の worktree を作成中..."
        git worktree add -b "$pr_branch" "$worktree_path" "origin/$pr_branch"

        pr_mode=true
    fi

    # --- PR モードでない場合は通常の fzf 選択 ---
    if [ "$pr_mode" = "false" ]; then

    # --- 1. 構造化されたブランチ情報を生成 ---
    local all_branches_meta=$(
        (git for-each-ref --format='[L] %(refname:short)%09%(refname)%09local' 'refs/heads') &&
            (git for-each-ref --format='[R] %(refname:short)%09%(refname)%09remote' 'refs/remotes' | grep -v '/HEAD$')
    )
    local used_refs=$(git worktree list --porcelain | grep '^branch ' | awk '{print $2}')
    local available_branches_meta=$(grep -v -F -f <(echo "$used_refs") <(echo "$all_branches_meta"))

    # --- 2. fzfでユーザーに選択させる ---
    local header="[Enter] 選択 | [Option+Enter] 新規作成"
    local preview_command="git log --oneline --color=always -10 {2} 2>/dev/null"
    local selection=$(echo "$available_branches_meta" | fzf --with-nth=1 --delimiter=$'\t' --height=20 \
        --prompt="Select Branch > " --header="$header" --preview="$preview_command" --preview-window=right:50% \
        --bind "alt-enter:print-query")

    if [ $? -ne 0 ]; then
        echo "Canceled."
        return 1
    fi
    if [ -z "$selection" ]; then
        echo "Selection was empty. Canceled."
        return 1
    fi

    # --- 3. ユーザーの選択に応じて処理を分岐 ---
    if echo "$selection" | grep -q -F $'\t'; then
        local display_name=$(echo "$selection" | awk -F$'\t' '{print $1}')
        local full_ref=$(echo "$selection" | awk -F$'\t' '{print $2}')
        local branch_type=$(echo "$selection" | awk -F$'\t' '{print $3}')
        local local_branch_name

        if [ "$branch_type" = "local" ]; then
            local_branch_name=$(echo "$full_ref" | sed 's:^refs/heads/::')
            local dir_name="${project_name}-$(echo "$local_branch_name" | sed 's/\//-/g')"
            worktree_path="${root_dir}/../${dir_name}"
            git worktree add "$worktree_path" "$local_branch_name"
        elif [ "$branch_type" = "remote" ]; then
            local_branch_name=$(echo "$full_ref" | sed 's:^refs/remotes/[^/]\+/::')
            local dir_name="${project_name}-$(echo "$local_branch_name" | sed 's/\//-/g')"
            worktree_path="${root_dir}/../${dir_name}"
            echo "Creating new local branch '$local_branch_name' to track '$display_name'..."
            git worktree add -b "$local_branch_name" "$worktree_path" "$full_ref"
        fi
    else
        local branch="$selection"
        local dir_name="${project_name}-$(echo "$branch" | sed 's/\//-/g')"
        worktree_path="${root_dir}/../${dir_name}"
        echo "'$branch' is a new branch."
        local all_branches_raw=$(echo "$all_branches_meta" | awk -F$'\t' '{print $1}')
        local default_base_display_name=""
        if echo "$all_branches_raw" | grep -xqF "[L] develop"; then
            default_base_display_name="[L] develop"
        elif echo "$all_branches_raw" | grep -xqF "[L] main"; then
            default_base_display_name="[L] main"
        elif echo "$all_branches_raw" | grep -xqF "[L] master"; then
            default_base_display_name="[L] master"
        fi
        local sorted_base_branch_list
        if [ -n "$default_base_display_name" ]; then
            sorted_base_branch_list=$( (
                echo "$default_base_display_name"
                echo "$all_branches_raw" | grep -v -x -F "$default_base_display_name"
            ))
        else
            sorted_base_branch_list=$all_branches_raw
        fi
        local base_selection=$(echo "$sorted_base_branch_list" | fzf --height=10 --prompt="Create from which base branch?: ")
        if [ $? -ne 0 ] || [ -z "$base_selection" ]; then
            echo "Canceled."
            return 1
        fi
        local base_ref=$(echo "$all_branches_meta" | grep -F "$base_selection" | head -n 1 | awk -F$'\t' '{print $2}')
        git worktree add -b "$branch" "$worktree_path" "$base_ref"
    fi

    fi  # PR モードでない場合の終了

    # Cursor で開くかどうかのフラグ
    local open_with_cursor=false

    # cursor コマンドが存在する場合のみ質問
    if command -v cursor >/dev/null 2>&1; then
        echo
        read -q "REPLY?作成後、Cursor で開きますか？ [y/N] "
        echo
        if [[ "$REPLY" =~ ^[Yy]$ ]]; then
            open_with_cursor=true
        fi
    fi

    # --- 4. セットアップコマンドの実行 ---
    if [ $? -eq 0 ]; then
        local find_name_args=()
        if [ ${#copy_files[@]} -gt 0 ]; then
            local first=true
            for file in "${copy_files[@]}"; do
                if ! $first; then find_name_args+=("-o"); fi
                if [[ "$file" == */* ]]; then
                    find_name_args+=("-path" "*/$file")
                else
                    find_name_args+=("-name" "$file")
                fi
                first=false
            done
        fi

        if [ ${#find_name_args[@]} -gt 0 ]; then
            echo "\nSearching for and copying local config files: ${copy_files[*]}"
            local find_grouped_args=(\( "${find_name_args[@]}" \))
            find "$root_dir" -path "$root_dir/.git" -prune -o -path "$root_dir/node_modules" -prune -o "${find_grouped_args[@]}" -print | while read -r src_path; do
                local rel_path=${src_path#$root_dir}
                local dest_path="${worktree_path}${rel_path}"
                
                if [ -d "$src_path" ]; then
                    # ディレクトリの場合は中身を再帰的にコピー（既存ディレクトリにも対応）
                    mkdir -p "$dest_path"
                    if cp -r "$src_path"/. "$dest_path"/; then
                        echo "  Copied directory .${rel_path}"
                    fi
                else
                    # ファイルの場合は通常のコピー
                    mkdir -p "$(dirname "$dest_path")"
                    if cp "$src_path" "$dest_path"; then
                        echo "  Copied .${rel_path}"
                    fi
                fi
            done
        fi

        # 最終的な移動先ディレクトリを決定
        local target_dir="$worktree_path"
        if [[ -n "$relative_path" && -d "${worktree_path}${relative_path}" ]]; then
            target_dir="${worktree_path}${relative_path}"
        fi

        echo "\nChanging to target directory and running setup..."

        # cd する前に mise trust を実行（direnv ロード前に trust を完了させる）
        if command -v mise >/dev/null 2>&1; then
            mise trust "$worktree_path" 2>/dev/null || true
        fi

        cd "$target_dir" && {
            if command -v direnv >/dev/null 2>&1 && [ -f ".envrc" ]; then direnv allow .; fi
            if command -v pnpm >/dev/null 2>&1 && [ -f "package.json" ] && ! [ -f "package-lock.json" ] && ! [ -f "yarn.lock" ] && ! [ -f "bun.lockb" ]; then pnpm i; fi
        }

        # 最後に Cursor で開く（最初に選択していた場合）
        if $open_with_cursor; then
            echo "\nCursor で開いています..."
            cursor .
            echo "元のディレクトリに戻ります: $original_dir"
            cd "$original_dir"
        fi
        # ★★★ ここまで ★★★
    else
        echo "Worktree creation failed. Skipping setup."
    fi
}

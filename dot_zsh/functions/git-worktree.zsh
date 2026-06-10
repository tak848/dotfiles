# Linear issue 情報（identifier / title / branchName）を JSON で stdout へ出力する。
# 引数: Linear URL or identifier（例 ENG-123）。
# 必要: 環境変数 GWC_LINEAR_API_KEY, jq, curl。
# gwc（Linear モード）と lcm（cmux.zsh）で共用。
_linear_fetch_issue() {
    local ref="$1"
    if [ -z "$GWC_LINEAR_API_KEY" ]; then
        echo "エラー: 環境変数 GWC_LINEAR_API_KEY が必要です。" >&2
        return 1
    fi
    if ! command -v jq >/dev/null 2>&1; then
        echo "エラー: jq が必要です。" >&2
        return 1
    fi

    # URL / identifier / ブランチ名のいずれからでも ENG-123 形式の identifier を抽出。
    # 小文字（eng-123 や ブランチ名 tak/eng-123-foo）でも拾えるよう大文字小文字を無視し、
    # Linear API が要求する大文字 identifier に正規化する。
    local identifier
    identifier=$(echo "$ref" | grep -oiE '[A-Z][A-Z0-9]*-[0-9]+' | head -n 1 | tr '[:lower:]' '[:upper:]')
    if [ -z "$identifier" ]; then
        echo "エラー: '$ref' から Linear の identifier (例 ENG-123) を抽出できませんでした。" >&2
        return 1
    fi

    # GraphQL で issue 情報を取得（issue(id:) は ENG-123 形式の identifier を受け付ける）
    local query='query($id:String!){issue(id:$id){identifier title branchName url}}'
    local payload resp
    payload=$(jq -nc --arg q "$query" --arg id "$identifier" '{query:$q, variables:{id:$id}}')
    resp=$(curl -s -X POST https://api.linear.app/graphql \
        -H "Authorization: $GWC_LINEAR_API_KEY" \
        -H "Content-Type: application/json" \
        --data "$payload")

    if [ -z "$(echo "$resp" | jq -r '.data.issue.identifier // empty')" ]; then
        echo "エラー: Linear ($identifier) から情報を取得できませんでした。" >&2
        echo "$resp" | jq -r '.errors[]?.message // empty' >&2
        return 1
    fi
    echo "$resp" | jq -c '.data.issue'
}

# リポジトリ名（worktree のディレクトリ名ではなく本来のリポジトリ名）を返す。
# origin のリモート URL から導出し、なければワークツリールートの basename にフォールバック。
# 例: git@github.com:tak848/dotfiles.git / https://github.com/tak848/dotfiles.git → dotfiles
_git_repo_name() {
    local url
    url=$(git remote get-url origin 2>/dev/null)
    if [ -n "$url" ]; then
        basename "${url%.git}"
    else
        local root
        root=$(git rev-parse --show-toplevel 2>/dev/null)
        [ -n "$root" ] && basename "$root"
    fi
}

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
    # "." 指定時は選択なしで「今いる worktree」を削除する
    if [ "$1" = "." ]; then
        local current_root main_root
        current_root=$(git rev-parse --show-toplevel 2>/dev/null)
        if [ -z "$current_root" ]; then
            echo "エラー: Gitリポジトリではありません。" >&2
            return 1
        fi
        # git worktree list の先頭がメインのワークツリー
        main_root=$(git worktree list | awk 'NR==1 {print $1}')
        if [ "$current_root" = "$main_root" ]; then
            echo "エラー: メインのワークツリーは削除できません。" >&2
            return 1
        fi
        # 現在の worktree 内からだと git が「自身を削除」とみなして失敗するため、
        # git -C でメイン側から remove する。これによりシェルの cd は一切行わない。
        echo "Removing worktree: $current_root"
        if ! git -C "$main_root" worktree remove "$current_root" 2>/dev/null; then
            echo "Remove failed (uncommitted changes etc.)."
            read -q "REPLY?--force? [y/N] "
            echo
            if [[ "$REPLY" =~ ^[Yy]$ ]]; then
                git -C "$main_root" worktree remove --force "$current_root" || return 1
            else
                echo "Skipped."
                return
            fi
        fi
        # 削除に成功すると今いるディレクトリは消えている。移動はしないので手動で cd すること。
        echo "Removed. Current directory no longer exists; cd to another worktree manually (e.g. gwt)."
        return
    fi

    # メインのワークツリーは削除対象外にする
    # 各 worktree の最終コミット日時を表示
    local worktrees_to_remove=$(git worktree list | awk 'NR>1 {print $1 " " $3}' | while read -r wt_path wt_branch; do
        local last_commit=""
        if [ -d "$wt_path" ]; then
            last_commit=$(git -C "$wt_path" log -1 --format="%ci" 2>/dev/null | cut -d' ' -f1,2 | cut -d':' -f1,2 | tr ' ' 'T')
        fi
        echo "${last_commit:-unknown}  ${wt_path}  ${wt_branch}"
    done | fzf --multi --prompt="Select worktree(s) to REMOVE (Tab to multi-select): ")

    if [ -z "$worktrees_to_remove" ]; then
        return
    fi

    # パイプ (echo | while) だとループがサブシェルになり use_force が
    # 次のイテレーションに引き継がれないため、ヒアストリングで親シェル実行する。
    # その際 read -q がヒアストリングを消費しないよう端末 (/dev/tty) から読む。
    local use_force=false
    while read -r line; do
        local wt_path=$(echo "$line" | awk '{print $2}')
        echo "Removing worktree: $wt_path"
        if $use_force; then
            git worktree remove --force "$wt_path"
        elif ! git worktree remove "$wt_path" 2>/dev/null; then
            echo "Remove failed (uncommitted changes etc.)."
            read -q "REPLY?--force for this and all remaining? [y/N] " </dev/tty
            echo
            if [[ "$REPLY" =~ ^[Yy]$ ]]; then
                use_force=true
                git worktree remove --force "$wt_path"
            else
                echo "Skipped."
            fi
        fi
    done <<< "$worktrees_to_remove"
}

# gwc: 既存ブランチ選択と新規ブランチ作成を兼ねる万能版
#
# 使用例:
#   gwc                              # 通常の使用（fzf でブランチ選択 / 新規作成）
#   gwc --copy data.local.json       # 追加ファイルを指定
#   gwc --pr https://github.com/owner/repo/pull/123  # PR URL から worktree 作成
#   gwc --pr 123                     # PR 番号から worktree 作成（同じリポジトリ内）
#   gwc https://github.com/owner/repo/pull/123  # 位置引数でも PR を判定
#   gwc 123                          # 位置引数の数字は PR 番号として判定
#   gwc --pr 123 --copy data.json    # PR と追加ファイルを指定
#   gwc --linear https://linear.app/acme/issue/ENG-123/...  # Linear チケットから作成
#   gwc https://linear.app/acme/issue/ENG-123/...  # 位置引数でも Linear を判定
#   gwc ENG-123                      # Linear の identifier を位置引数で指定
#   gwc ENG-123 --base develop       # base ブランチを上書き（Linear モード、既定は main）
#   gwc --cursor                     # 作成後に Cursor で開く
#   gwc ENG-123 --cc                 # 作成後に claude を初期プロンプトで起動（GWC_CLAUDE_CODE_INITIAL_PROMPT）
#   gwc ENG-123 --co                 # 作成後に codex を初期プロンプトで起動（GWC_CODEX_CLI_INITIAL_PROMPT）
#   gwc ENG-123 --cc "追加の指示"     # 初期プロンプト + 改行2つ + 追加プロンプトで起動（ref より後ろに置くこと）
#   export GWC_COPY_FILES=".env.test,config.local.json"  # 環境変数で事前設定
#   export GWC_PNPM_EXTRA_DIRS="apps/foo,apps/bar"  # root 以外で pnpm install するディレクトリ（worktree root からの相対パス、カンマ区切り）
#   export GWC_LINEAR_API_KEY="lin_api_..."  # Linear モードに必要（環境変数として設定）
#   export GWC_CLAUDE_CODE_INITIAL_PROMPT="..."  # --cc で claude に渡す初期プロンプト（未設定なら素の起動 or 追加プロンプトのみ）
#   export GWC_CODEX_CLI_INITIAL_PROMPT="..."    # --co で codex に渡す初期プロンプト（同上）
#
# cmux 環境（CMUX_WORKSPACE_ID あり）で Linear/PR モードを使うと、実行元の cmux
# ワークスペース名を自動設定する（Linear: "<repo>[<ID>] <title>" / PR: "<repo>[#<番号>] <title>"）。
# cmux 環境でなければ何もしない。単体で名前だけ付け直したい場合は lcm（cmux.zsh）を使う。
#
gwc() {
    # 元のディレクトリを保存
    local original_dir=$(pwd)
    local worktree_add_status=1

    local default_copy_files=(".envrc.local" ".env.local" "settings.local.json" "CLAUDE.local.md" ".mcp.json" ".codex/config.toml" ".mise.local.toml")
    local extra_copy_files=()
    local pr_ref=""
    local linear_ref=""
    local base_override=""
    local positional_ref=""
    local skip_fzf=false
    local open_with_cursor=false
    local cmux_title=""  # cmux 環境ならワークスペース名に設定する文字列（Linear/PR モードでセット）
    local launch_agent=""  # --cc → claude / --co → codex。worktree 作成後に初期プロンプトで起動
    local agent_extra=""   # --cc / --co の後ろに渡された追加プロンプト

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
        --linear)
            if [ -n "$2" ]; then
                linear_ref="$2"
                shift # --linear を消費
                shift # Linear URL/identifier を消費
            else
                echo "エラー: --linear オプションには Linear の URL または identifier (例 ENG-123) が必要です。" >&2
                return 1
            fi
            ;;
        --base)
            if [ -n "$2" ]; then
                base_override="$2"
                shift # --base を消費
                shift # ブランチ名を消費
            else
                echo "エラー: --base オプションにはブランチ名が必要です。" >&2
                return 1
            fi
            ;;
        --cursor)
            open_with_cursor=true
            shift
            ;;
        --cc | --co)
            # --cc → claude / --co → codex。worktree 作成後に初期プロンプトで起動する。
            # 直後にオプションでないトークンがあれば追加プロンプトとして取り込む
            # （例: gwc ENG-123 --cc "追加の指示"）。ref と紛れないよう --cc/--co は ref の後ろに置くこと。
            if [ -n "$launch_agent" ]; then
                echo "エラー: --cc と --co は同時に指定できません。" >&2
                return 1
            fi
            if [ "$1" = "--cc" ]; then
                launch_agent="claude"
            else
                launch_agent="codex"
            fi
            shift # --cc / --co を消費
            if [ -n "$1" ] && [[ "$1" != -* ]]; then
                agent_extra="$1"
                shift # 追加プロンプトを消費
            fi
            ;;
        --*)
            echo "エラー: 不明なオプション '$1'" >&2
            return 1
            ;;
        *)
            if [ -n "$positional_ref" ]; then
                echo "エラー: ref は 1 つだけ指定できます ('$positional_ref' と '$1')。" >&2
                return 1
            fi
            positional_ref="$1"
            shift
            ;;
        esac
    done

    # --- ref の解決と種別判定 ---
    # 明示フラグ (--pr / --linear) と位置引数の重複・複数指定をチェック
    local explicit_ref_count=0
    [ -n "$pr_ref" ] && explicit_ref_count=$((explicit_ref_count + 1))
    [ -n "$linear_ref" ] && explicit_ref_count=$((explicit_ref_count + 1))
    [ -n "$positional_ref" ] && explicit_ref_count=$((explicit_ref_count + 1))
    if [ "$explicit_ref_count" -gt 1 ]; then
        echo "エラー: --pr / --linear / 位置引数の ref はいずれか 1 つだけ指定できます。" >&2
        return 1
    fi

    # 位置引数が来た場合は内容から PR / Linear を自動判定
    if [ -n "$positional_ref" ]; then
        if [[ "$positional_ref" == *linear.app* ]] || [[ "$positional_ref" =~ ^[A-Z][A-Z0-9]*-[0-9]+$ ]]; then
            linear_ref="$positional_ref"
        elif { [[ "$positional_ref" == *github.com* ]] && [[ "$positional_ref" == */pull/* ]]; } || [[ "$positional_ref" =~ ^[0-9]+$ ]]; then
            pr_ref="$positional_ref"
        else
            echo "エラー: 位置引数 '$positional_ref' を PR / Linear のいずれとも判定できませんでした。" >&2
            echo "  PR: github.com の PR URL または番号 / Linear: linear.app の URL または ENG-123 形式" >&2
            return 1
        fi
    fi

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
        pr_info=$(gh pr view "$pr_ref" --json number,title,headRefName,headRepository,headRepositoryOwner 2>/dev/null)
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

        # cmux ワークスペース名用に PR 番号とタイトルも取得
        local pr_number pr_title
        pr_number=$(echo "$pr_info" | jq -r '.number')
        pr_title=$(echo "$pr_info" | jq -r '.title')
        # repo は worktree ディレクトリ名ではなくリポジトリ名
        cmux_title="$(_git_repo_name)[#${pr_number}] ${pr_title}"

        echo "PR からブランチ '$pr_branch' を取得しました。"

        # リモートを fetch して最新状態に
        echo "リモートからブランチを取得中..."
        git fetch origin "$pr_branch"

        # worktree を作成
        local dir_name="${project_name}-$(echo "$pr_branch" | sed 's/\//-/g')"
        worktree_path="${root_dir}/../${dir_name}"

        echo "ブランチ '$pr_branch' の worktree を作成中..."
        git worktree add -b "$pr_branch" "$worktree_path" "origin/$pr_branch"
        worktree_add_status=$?

        skip_fzf=true
    fi

    # --- Linear モード: Linear GraphQL API でブランチ名を取得 ---
    if [ -n "$linear_ref" ]; then
        # Linear issue 情報を取得（identifier / title / branchName）。
        # 必要なツール・API キーのチェックは _linear_fetch_issue 側で行う。
        local linear_issue
        linear_issue=$(_linear_fetch_issue "$linear_ref") || return 1
        local linear_identifier linear_branch linear_title
        linear_identifier=$(echo "$linear_issue" | jq -r '.identifier')
        linear_branch=$(echo "$linear_issue" | jq -r '.branchName')
        linear_title=$(echo "$linear_issue" | jq -r '.title')

        if [ -z "$linear_branch" ] || [ "$linear_branch" = "null" ]; then
            echo "エラー: Linear ($linear_identifier) からブランチ名を取得できませんでした。" >&2
            return 1
        fi

        # cmux ワークスペース名用の文字列をセット（repo は worktree ディレクトリ名ではなくリポジトリ名）
        cmux_title="$(_git_repo_name)[${linear_identifier}] ${linear_title}"

        echo "Linear チケット $linear_identifier のブランチ名: $linear_branch"

        # base ブランチ解決（既定は main、--base で上書き）。最新を使うため origin を fetch
        local linear_base="${base_override:-main}"
        echo "ベースブランチ '$linear_base' を取得中..."
        git fetch origin "$linear_base" 2>/dev/null
        local linear_base_ref="origin/$linear_base"
        if ! git rev-parse --verify --quiet "$linear_base_ref" >/dev/null 2>&1; then
            linear_base_ref="$linear_base"
        fi

        local dir_name="${project_name}-$(echo "$linear_branch" | sed 's/\//-/g')"
        worktree_path="${root_dir}/../${dir_name}"

        echo "ブランチ '$linear_branch' の worktree を '$linear_base_ref' から作成中..."
        git worktree add -b "$linear_branch" "$worktree_path" "$linear_base_ref"
        worktree_add_status=$?

        skip_fzf=true
    fi

    # --- 自動モード（PR / Linear）でない場合は通常の fzf 選択 ---
    if [ "$skip_fzf" = "false" ]; then

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
            worktree_add_status=$?
        elif [ "$branch_type" = "remote" ]; then
            local_branch_name=$(echo "$full_ref" | sed 's:^refs/remotes/[^/]\+/::')
            local dir_name="${project_name}-$(echo "$local_branch_name" | sed 's/\//-/g')"
            worktree_path="${root_dir}/../${dir_name}"
            echo "Creating new local branch '$local_branch_name' to track '$display_name'..."
            git worktree add -b "$local_branch_name" "$worktree_path" "$full_ref"
            worktree_add_status=$?
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
        worktree_add_status=$?
    fi

    fi  # PR モードでない場合の終了

    # --- 4. セットアップコマンドの実行 ---
    if [ $worktree_add_status -eq 0 ]; then
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
            find "$root_dir" \( -name .git -o -name node_modules -o -name .venv -o -name venv \) -prune -o "${find_grouped_args[@]}" -print | while read -r src_path; do
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
            if command -v pnpm >/dev/null 2>&1 && { [ -f "$worktree_path/pnpm-lock.yaml" ] || [ -f "package.json" ]; }; then pnpm install --frozen-lockfile; fi
        }

        # 環境変数 GWC_PNPM_EXTRA_DIRS で指定された root 以外のディレクトリでも pnpm install を実行
        # （pnpm workspace が分離しているモノレポなどで、root の挙動を変えずに追加で install したい場合用）
        if [ -n "$GWC_PNPM_EXTRA_DIRS" ] && command -v pnpm >/dev/null 2>&1; then
            local pnpm_extra_dirs=()
            IFS=',' read -r -A pnpm_extra_dirs <<< "$GWC_PNPM_EXTRA_DIRS"
            for extra_dir in "${pnpm_extra_dirs[@]}"; do
                # 前後の空白を除去（"apps/foo, apps/bar" のようなコンマ後スペースに対応）
                extra_dir="${extra_dir#"${extra_dir%%[^[:space:]]*}"}"
                extra_dir="${extra_dir%"${extra_dir##*[^[:space:]]}"}"
                [ -z "$extra_dir" ] && continue
                local install_dir="${worktree_path}/${extra_dir}"
                if [ -f "${install_dir}/pnpm-lock.yaml" ]; then
                    echo "  ${extra_dir} で pnpm install を実行中..."
                    (cd "$install_dir" && pnpm install --frozen-lockfile)
                else
                    echo "  警告: ${extra_dir} に pnpm-lock.yaml が見つかりません。スキップします。" >&2
                fi
            done
        fi

        # cmux 環境なら、コマンド実行元ワークスペース名を設定（非 cmux では nop）。
        # 対象は CMUX_WORKSPACE_ID（実行元）。cmux current-workspace は選択中のものを返すため使わない。
        if [ -n "$cmux_title" ] && _cmux_available; then
            if _cmux_rename_current_workspace "$cmux_title"; then
                echo "cmux workspace 名を設定: $cmux_title"
            fi
        fi

        # --cursor フラグが指定されていた場合、Cursor で開く
        if $open_with_cursor; then
            if command -v cursor >/dev/null 2>&1; then
                echo "\nCursor で開いています..."
                cursor .
                echo "元のディレクトリに戻ります: $original_dir"
                cd "$original_dir"
            else
                echo "警告: cursor コマンドが見つかりません。" >&2
            fi
        fi
        # ★★★ ここまで ★★★

        # --cc / --co: worktree 内で claude / codex を初期プロンプト付きで起動する。
        # プロンプトは位置引数で渡す（codex は stdin パイプ非対応のため。claude も同様に統一）。
        #   - 環境変数（GWC_CLAUDE_CODE_INITIAL_PROMPT / GWC_CODEX_CLI_INITIAL_PROMPT）と
        #     追加プロンプト（agent_extra）の両方があれば「環境変数 + 改行2つ + 追加」を送る
        #   - どちらか一方だけならそれを送る / 両方なければプロンプト無しで素の起動
        if [ -n "$launch_agent" ]; then
            # --cursor で original_dir に戻っている可能性があるため target_dir に入り直す
            cd "$target_dir"
            if ! command -v "$launch_agent" >/dev/null 2>&1; then
                echo "警告: '$launch_agent' コマンドが見つかりません。起動をスキップします。" >&2
            else
                local agent_base=""
                if [ "$launch_agent" = "claude" ]; then
                    agent_base="$GWC_CLAUDE_CODE_INITIAL_PROMPT"
                else
                    agent_base="$GWC_CODEX_CLI_INITIAL_PROMPT"
                fi
                local agent_prompt=""
                if [ -n "$agent_base" ] && [ -n "$agent_extra" ]; then
                    agent_prompt="${agent_base}"$'\n\n'"${agent_extra}"
                elif [ -n "$agent_base" ]; then
                    agent_prompt="$agent_base"
                elif [ -n "$agent_extra" ]; then
                    agent_prompt="$agent_extra"
                fi
                echo "\n$launch_agent を起動します..."
                if [ -n "$agent_prompt" ]; then
                    "$launch_agent" "$agent_prompt"
                else
                    "$launch_agent"
                fi
            fi
        fi
    else
        echo "Worktree creation failed. Skipping setup."
    fi
}

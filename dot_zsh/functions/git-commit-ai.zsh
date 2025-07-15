# git commit メッセージを Claude Code で生成する関数
#
# 使用方法:
#   git-commit-ai [-y|--yes] [-e|--edit] [--amend]
#
# 環境変数:
#   GIT_COMMIT_AI_FORMAT: コミットメッセージの形式をカスタマイズ
#     例: export GIT_COMMIT_AI_FORMAT='コミットメッセージは日本語で書いてください。'
#     ※ git status と git diff の内容は自動的に含まれます
#
git-commit-ai() {
    local auto_commit=false
    local edit_after=false
    local amend_mode=false
    local extra_args=()

    # コマンドラインオプションを解析
    while [[ $# -gt 0 ]]; do
        case "$1" in
        -y | --yes)
            auto_commit=true
            shift
            ;;
        -e | --edit)
            edit_after=true
            shift
            ;;
        --amend)
            amend_mode=true
            extra_args+=("--amend")
            shift
            ;;
        *)
            echo "エラー: 不明なオプション '$1'" >&2
            echo "使用方法: git-commit-ai [-y|--yes] [-e|--edit] [--amend]" >&2
            return 1
            ;;
        esac
    done

    # Git リポジトリ内かチェック
    if ! git rev-parse --git-dir >/dev/null 2>&1; then
        echo "エラー: Git リポジトリではありません。" >&2
        return 1
    fi

    # claude コマンドが利用可能かチェック
    if ! command -v claude &>/dev/null; then
        echo "エラー: claude コマンドが見つかりません。" >&2
        echo "Claude Code CLI をインストールしてください: npm install -g @anthropic-ai/claude-code" >&2
        return 1
    fi

    # コミット可能な変更があるかチェック（--amend モードでない場合）
    if ! $amend_mode; then
        # ステージされた変更があるかチェック
        if git diff --staged --quiet; then
            echo "エラー: ステージされた変更がありません。" >&2
            echo "ヒント: 'git add' でファイルをステージングしてください。" >&2
            return 1
        fi
    fi

    echo "変更内容を確認しています..."

    # git status と git diff の情報を収集
    local git_status_output=$(git status --porcelain)
    local git_diff_output=""

    if $amend_mode; then
        # --amend モードの場合、最後のコミットとの差分を取得
        git_diff_output=$(git diff HEAD~1 HEAD)
    else
        # ステージされた変更がある場合は、それを優先的に表示
        if ! git diff --staged --quiet; then
            # ステージされた変更を表示
            git_diff_output=$(git diff --staged)
        else
            # ステージされた変更がない場合は、すべての変更を表示
            git_diff_output=$(git diff)
        fi
    fi

    # プロンプトを構築
    local git_info="## git status
\`\`\`
$git_status_output
\`\`\`

## git diff
\`\`\`diff
$git_diff_output
\`\`\`"

    # デフォルトの指示
    local default_instructions="コミットメッセージは以下の形式で生成してください：
- 1行目: 適切なprefixを付けて変更の要約（50文字以内）
  - feat: 新機能
  - fix: バグ修正
  - docs: ドキュメントのみの変更
  - style: コードの意味に影響しない変更（空白、フォーマット、セミコロンなど）
  - refactor: バグ修正や機能追加を含まないコードの変更
  - perf: パフォーマンスを向上させるコードの変更
  - test: テストの追加や既存テストの修正
  - build: ビルドシステムや外部依存関係に影響する変更
  - ci: CI設定ファイルやスクリプトの変更
  - chore: その他の変更（ビルドプロセスやツールの変更など）
  - revert: 以前のコミットを取り消す
- 2行目: 空行
- 3行目以降: 詳細な説明（必要に応じて）"

    # 共通の制約事項
    local common_constraints="
重要: git diffに表示されている内容（ステージされた変更）のみに基づいてメッセージを生成してください。
**git statusに表示されていても、git diffに含まれていない変更は無視してください。**
**git statusに表示されていても、git diffに含まれていない変更は無視してください。**

以下の指示に厳密に従ってください：
1. コミットメッセージのテキストのみを出力する
2. 「変更内容を分析すると」などの前置きは一切書かない
3. 「これらの変更に基づいて」などの説明は一切書かない
4. 「以下のコミットメッセージを提案します」などの文言は一切書かない
5. Markdownのコードブロック（\`\`\`）で囲まない
6. コミットメッセージ以外の余計な文章は一切含めない"

    # フォーマット指示の選択
    local format_instructions=""
    if [[ -n "${GIT_COMMIT_AI_FORMAT}" ]]; then
        format_instructions="${GIT_COMMIT_AI_FORMAT}"
    else
        format_instructions="$default_instructions"
    fi

    # 最終的なプロンプトの構築
    local prompt="以下のGitの変更内容から、適切なコミットメッセージを生成してください。

$git_info

$format_instructions

$common_constraints"

    if $amend_mode; then
        prompt="$prompt

注意: これは --amend モードです。既存のコミットを修正するための新しいメッセージを生成してください。"
    fi

    echo "Claude Code でコミットメッセージを生成中..."

    # Claude Code を使ってコミットメッセージを生成
    local commit_message
    if [[ "${GIT_COMMIT_AI_DEBUG:-}" == "1" ]]; then
        echo "Debug: プロンプト長 = ${#prompt} 文字" >&2
    fi

    commit_message=$(claude -p "$prompt" 2>&1)
    local claude_exit_code=$?

    if [[ $claude_exit_code -ne 0 ]]; then
        echo "エラー: Claude Code の実行に失敗しました (exit code: $claude_exit_code)" >&2
        echo "出力: $commit_message" >&2
        return 1
    fi

    if [[ -z "$commit_message" ]]; then
        echo "エラー: コミットメッセージの生成に失敗しました。" >&2
        return 1
    fi

    # 生成されたメッセージを表示
    echo -e "\n生成されたコミットメッセージ:"
    echo "================================"
    echo "$commit_message"
    echo "================================"

    # エディタで編集する場合
    if $edit_after; then
        # 一時ファイルにメッセージを保存
        local temp_file=$(mktemp)
        echo "$commit_message" >"$temp_file"

        # エディタで開く
        ${EDITOR:-vim} "$temp_file"

        # 編集後のメッセージを読み込む
        commit_message=$(cat "$temp_file")
        rm -f "$temp_file"
    fi

    # 自動コミットモードでない場合は確認
    if ! $auto_commit; then
        echo -e "\nこのメッセージでコミットしますか？ [Y/n/e(dit)] "
        read -r response

        case "$response" in
        [nN])
            echo "キャンセルしました。"
            return 0
            ;;
        [eE])
            # エディタで編集
            local temp_file=$(mktemp)
            echo "$commit_message" >"$temp_file"
            ${EDITOR:-vim} "$temp_file"
            commit_message=$(cat "$temp_file")
            rm -f "$temp_file"
            ;;
        *)
            # Y または Enter の場合は続行
            ;;
        esac
    fi

    # コミットを実行
    echo -e "\nコミットを実行しています..."

    # コミットメッセージを一時ファイルに保存してコミット
    local temp_file=$(mktemp)
    echo "$commit_message" >"$temp_file"

    if git commit "${extra_args[@]}" -F "$temp_file"; then
        echo "コミットが完了しました。"
        rm -f "$temp_file"
        return 0
    else
        echo "エラー: コミットに失敗しました。" >&2
        rm -f "$temp_file"
        return 1
    fi
}

# エイリアスも定義
alias gcai=git-commit-ai
alias gca=git-commit-ai

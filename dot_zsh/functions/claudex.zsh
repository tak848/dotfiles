# claudex: Claude Code のハーネスのまま、モデルだけ GPT-5.6 Sol にする
#
# claude-code-proxy が Anthropic Messages API 互換のプロキシとして立ち、ChatGPT サブスクの
# OAuth 経由で Codex backend に転送する。ツールループ・サブエージェント・hooks・MCP は Claude
# Code のものがそのまま効き、推論するモデルだけが入れ替わる。
#
# 初回のみ認証が必要:
#   claude-code-proxy codex auth login   # ChatGPT Plus/Pro アカウントでログイン
#
# 注意:
#   - 消費するのは ChatGPT 側の quota。Claude のサブスクは減らない（素の claude は従来通り）
#   - プロキシはマシン単位で 1 プロセス。全 worktree・全セッションが 1 つを共有する
#   - Anthropic は非 Claude モデルへの gateway ルーティングを公式サポートしていない
#
# 上書き用の環境変数: CLAUDEX_MODEL, CLAUDEX_SMALL_MODEL, CLAUDEX_PORT, CLAUDEX_COMPACT_WINDOW
#   例) CLAUDEX_MODEL='gpt-5.6-sol-fast' claudex        # priority tier で叩く
#   例) CLAUDEX_COMPACT_WINDOW=250000 claudex           # backend が 272K に巻き戻った日は下げる

# ポートが listen されているか（外部コマンドに依存せず zsh 組み込みで確認する）
_claudex_port_open() {
    zmodload zsh/net/tcp 2>/dev/null || return 1
    if ztcp 127.0.0.1 "$1" 2>/dev/null; then
        ztcp -c "$REPLY"
        return 0
    fi
    return 1
}

# プロキシが生きていなければ起動する。同時に複数の claudex が走っても二重起動しないよう
# mkdir のアトミック性でロックを取り、ロックを取れなかった側は起動を待つだけにする
_claudex_ensure_proxy() {
    local port="${CLAUDEX_PORT:-18765}"
    _claudex_port_open "$port" && return 0

    if ! command -v claude-code-proxy >/dev/null 2>&1; then
        echo "エラー: claude-code-proxy が見つかりません（mise install で導入されます）" >&2
        return 1
    fi

    if ! claude-code-proxy codex auth status >/dev/null 2>&1; then
        echo "エラー: Codex が未認証です。'claude-code-proxy codex auth login' を実行してください" >&2
        return 1
    fi

    local lockdir="${TMPDIR:-/tmp}/claudex-${port}.lock"
    if mkdir "$lockdir" 2>/dev/null; then
        local logdir="${XDG_STATE_HOME:-$HOME/.local/state}/claude-code-proxy"
        mkdir -p "$logdir"
        PORT="$port" nohup claude-code-proxy serve --no-monitor >>"$logdir/serve.log" 2>&1 &
        disown
        rmdir "$lockdir" 2>/dev/null
    fi

    local i
    for i in {1..50}; do
        _claudex_port_open "$port" && return 0
        sleep 0.1
    done

    echo "エラー: claude-code-proxy が 127.0.0.1:${port} で起動しませんでした" >&2
    echo "  ログ: ${XDG_STATE_HOME:-$HOME/.local/state}/claude-code-proxy/serve.log" >&2
    return 1
}

claudex() {
    _claudex_ensure_proxy || return 1

    local model="${CLAUDEX_MODEL:-gpt-5.6-sol}"

    # [1m] サフィックスは付けない。gpt-5.6-sol は API では 1.05M context だが、claudex が経由する
    # ChatGPT/Codex バックエンドでは context がキャップされる（実測で ~372K まで到達を確認）。OpenAI は
    # 372K で課金が想定超だったため Codex の default を一時 272K に戻し、数日かけて 372K に戻すと
    # アナウンスした（thsottiaux 2076495156757577895 / openai/codex#31860, #32806）ので、当面 272K↔372K
    # で揺れうる。[1m] を付けると CC が「1M ある」と誤認して auto-compact を焚かず壁に激突する。壁に
    # 当たってからの compact は現 context 全量を要約に送るため同じ上限を超えて失敗し、デッドロックする。
    #
    # 既定は観測済みの 372K キャップに合わせて 360000（compact は窓の約9割手前 ~331K で焚かれ、summary
    # 呼び出しが 372K の壁の内側で完了する）。もし backend が 272K に巻き戻った日に "Context limit
    # reached" のデッドロックが再発したら、その時だけ CLAUDEX_COMPACT_WINDOW=250000 のように下げる。
    #
    # これは OS 環境変数ではなく --settings（CLI 引数）で渡す。settings ファイルの env は OS 環境変数に
    # 勝つため、プロジェクトの .claude/settings.json が env.CLAUDE_CODE_AUTO_COMPACT_WINDOW を設定して
    # いると OS env 注入では負ける。優先順位は Managed > Command-line > Local > Project > User なので、
    # --settings で渡せば project 設定にも勝てる（settings.md）。
    local compact_settings="{\"env\":{\"CLAUDE_CODE_AUTO_COMPACT_WINDOW\":\"${CLAUDEX_COMPACT_WINDOW:-360000}\"}}"

    # CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC は settings.jsonnet と同様に設定しない
    # （remote-control の eligibility チェックがブロックされるため）。無駄なモデル呼び出し自体は
    # settings.jsonnet の DISABLE_NON_ESSENTIAL_MODEL_CALLS で既に止まっている
    ANTHROPIC_BASE_URL="http://127.0.0.1:${CLAUDEX_PORT:-18765}" \
    ANTHROPIC_AUTH_TOKEN="unused" \
    ANTHROPIC_SMALL_FAST_MODEL="${CLAUDEX_SMALL_MODEL:-gpt-5.6-luna}" \
    CLAUDE_CODE_SUBAGENT_MODEL="${model%\[*}" \
    CLAUDE_CODE_MAX_TOOL_USE_CONCURRENCY=3 \
    CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK=1 \
    ENABLE_TOOL_SEARCH=false \
        command claude --model "$model" --settings "$compact_settings" "$@"
}

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
#   例) CLAUDEX_MODEL='gpt-5.6-sol-fast[1m]' claudex    # priority tier で叩く
#   例) CLAUDEX_MODEL='gpt-5.6-sol' claudex             # [1m] が compact を壊す場合のフォールバック
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

    local model="${CLAUDEX_MODEL:-gpt-5.6-sol[1m]}"

    # [1m] を付けて statusline を /1000k にする。gpt-5.6-sol は claudex が経由する ChatGPT/Codex
    # バックエンドでは context がキャップされる（実測で ~372K まで到達を確認。OpenAI は 372K で課金が
    # 想定超だったため Codex default を一時 272K に戻し、数日かけて 372K に戻すとアナウンス:
    # thsottiaux 2076495156757577895 / openai/codex#31860, #32806）。[1m] を外すと statusline の分母が
    # モデル既定（~200K）になり、Sol は 200K を超えるためゲージが 100% を振り切って読めなくなる。
    # 実キャップ 372K に一番近い表示は [1m] の /1000k なので付ける。
    #
    # auto-compact は下の --settings の CLAUDE_CODE_AUTO_COMPACT_WINDOW（既定 360000）で
    # 壁の手前 ~331K に焚く。既定は観測済みの 372K キャップに合わせてある。backend が 272K に
    # 巻き戻った日に "Context limit reached" が再発したら CLAUDEX_COMPACT_WINDOW=250000 で下げる。
    #
    # 【要実測の前提】[1m]（context 窓 1M と認識）と明示 AUTO_COMPACT_WINDOW=360000 が両立し、
    # compact が 1M 手前ではなく 360000 基準（~331K）で焚かれること。ドキュメントが曖昧で nested claude
    # 禁止のため机上検証不可。もし [1m] が AUTO_COMPACT_WINDOW を上書きして 372K で詰むなら、
    # CLAUDEX_MODEL='gpt-5.6-sol'（[1m] 無し）にフォールバックする。
    #
    # AUTO_COMPACT_WINDOW は OS 環境変数ではなく --settings（CLI 引数）で渡す。settings ファイルの env は
    # OS 環境変数に勝つため、プロジェクトの .claude/settings.json が env.CLAUDE_CODE_AUTO_COMPACT_WINDOW を
    # 設定していると OS env 注入では負ける。優先順位は Managed > Command-line > Local > Project > User
    # なので、--settings で渡せば project 設定にも勝てる（settings.md）。
    local compact_settings="{\"env\":{\"CLAUDE_CODE_AUTO_COMPACT_WINDOW\":\"${CLAUDEX_COMPACT_WINDOW:-360000}\"}}"

    # CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC は settings.jsonnet と同様に設定しない
    # （remote-control の eligibility チェックがブロックされるため）。無駄なモデル呼び出し自体は
    # settings.jsonnet の DISABLE_NON_ESSENTIAL_MODEL_CALLS で既に止まっている
    ANTHROPIC_BASE_URL="http://127.0.0.1:${CLAUDEX_PORT:-18765}" \
    ANTHROPIC_AUTH_TOKEN="unused" \
    ANTHROPIC_SMALL_FAST_MODEL="${CLAUDEX_SMALL_MODEL:-gpt-5.6-terra[1m]}" \
    CLAUDE_CODE_SUBAGENT_MODEL="${model%\[*}" \
    CLAUDE_CODE_MAX_TOOL_USE_CONCURRENCY=3 \
    CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK=1 \
    ENABLE_TOOL_SEARCH=false \
        command claude --model "$model" --settings "$compact_settings" "$@"
}

# claudexf: claudex の Codex fast/priority tier 版。
# -fast サフィックスを claude-code-proxy が service_tier: "priority" に翻訳して upstream に投げる。
# メインの推論モデルと small/fast モデル（要約・タイトル生成）の両方を fast tier にする。
# 速い代わりにサブスク usage の減りが早い。quota を使い切れないとき向け。
# CLAUDEX_MODEL / CLAUDEX_SMALL_MODEL が明示指定されていればそれを優先する（fast を強制しない）。
claudexf() {
    CLAUDEX_MODEL="${CLAUDEX_MODEL:-gpt-5.6-sol-fast[1m]}" \
    CLAUDEX_SMALL_MODEL="${CLAUDEX_SMALL_MODEL:-gpt-5.6-terra-fast[1m]}" \
        claudex "$@"
}

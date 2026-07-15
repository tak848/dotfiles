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
#   例) CLAUDEX_COMPACT_WINDOW=250000 claudex           # backend が 272K に巻き戻った日は下げる
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

    # [1m] は必須。claudex は ANTHROPIC_BASE_URL が proxy(=LLM gateway) を指すため、CC は 1M サポートを
    # 検証できず、[1m] を外すと window を 200K として割り当て 200K で auto-compact する
    # （model-config「拡張コンテキスト」/ Sonnet 5 の項に gateway の挙動として明記）。[1m] で 1M window を
    # 選ぶことで実キャップ 372K まで使え、statusline も /1000k になる。
    #
    # gpt-5.6-sol の実キャップ: ChatGPT/Codex backend では ~372K（272K↔372K で揺れる。OpenAI が 372K で
    # 課金想定超のため Codex default を一時 272K に戻し、数日かけて 372K に戻すとアナウンス:
    # thsottiaux 2076495156757577895 / openai/codex#31860, #32806）。
    #
    # compact しきい値: [1m] が window を 1M と認識させ、CLAUDE_CODE_AUTO_COMPACT_WINDOW がその中の発火
    # しきい値を決める（両者は競合せず共存。model-config: Sonnet 5 は 1M window で既定 ~967K しきい値、
    # 変更は CLAUDE_CODE_AUTO_COMPACT_WINDOW）。この値は「そのトークン数ちょうどで compact」であり、9割
    # 手前ではない。既定 360000 は 372K の壁の手前 12K で要約に入る想定（要約リクエストは現 context 全量を
    # 送るのでしきい値 < 実キャップ でないと壁を超えて失敗＝デッドロックする）。backend が 272K に巻き戻った
    # 日は 360000 では詰むため CLAUDEX_COMPACT_WINDOW=250000 のように下げる。
    #
    # AUTO_COMPACT_WINDOW は OS 環境変数ではなく --settings（CLI 引数）で渡す。settings ファイルの env は
    # OS 環境変数に勝つため、プロジェクトの .claude/settings.json が env.CLAUDE_CODE_AUTO_COMPACT_WINDOW を
    # 設定していると OS env 注入では負ける。優先順位は Managed > Command-line > Local > Project > User
    # なので、--settings で渡せば project 設定にも勝てる（settings.md）。
    local compact_settings="{\"env\":{\"CLAUDE_CODE_AUTO_COMPACT_WINDOW\":\"${CLAUDEX_COMPACT_WINDOW:-360000}\"}}"

    # メインの推論モデル（--model）以外に、CC が内部で使う haiku / sonnet エイリアスも Codex モデルに
    # 向けておく。素の Claude 名（claude-haiku-* / claude-sonnet-*）に解決されると proxy 経由で意図しない
    # モデルになるため、両方 terra 系（CLAUDEX_SMALL_MODEL）にマッピングする。
    #   - ANTHROPIC_DEFAULT_HAIKU_MODEL: haiku エイリアス＋バックグラウンド機能（要約・タイトル生成等）。
    #     旧 ANTHROPIC_SMALL_FAST_MODEL は非推奨（model-config の環境変数表の注記）
    #   - ANTHROPIC_DEFAULT_SONNET_MODEL: sonnet エイリアス（/model sonnet 等）
    # opus エイリアスは今は未マッピング（primary は --model で sol を明示しているため通常は不要）。
    #
    # CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC は settings.jsonnet と同様に設定しない
    # （remote-control の eligibility チェックがブロックされるため）。無駄なモデル呼び出し自体は
    # settings.jsonnet の DISABLE_NON_ESSENTIAL_MODEL_CALLS で既に止まっている
    ANTHROPIC_BASE_URL="http://127.0.0.1:${CLAUDEX_PORT:-18765}" \
    ANTHROPIC_AUTH_TOKEN="unused" \
    ANTHROPIC_DEFAULT_HAIKU_MODEL="${CLAUDEX_SMALL_MODEL:-gpt-5.6-terra[1m]}" \
    ANTHROPIC_DEFAULT_SONNET_MODEL="${CLAUDEX_SMALL_MODEL:-gpt-5.6-terra[1m]}" \
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

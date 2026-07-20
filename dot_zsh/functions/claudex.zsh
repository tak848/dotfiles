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
# 上書き用の環境変数: CLAUDEX_MODEL, CLAUDEX_SMALL_MODEL, CLAUDEX_PORT, CLAUDEX_CONTEXT_TOKENS
#   例) CLAUDEX_MODEL='gpt-5.6-sol-fast' claudex    # priority tier で叩く
#   例) CLAUDEX_CONTEXT_TOKENS=272000 claudex      # backend が 272K に巻き戻った日は下げる

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

    # CLAUDE_CODE_MAX_CONTEXT_TOKENS は、ANTHROPIC_BASE_URL 経由の未認識モデルについて Claude Code が
    # 仮定する context window を上書きする。gpt-5.6-sol の実キャップは ChatGPT/Codex backend で約372K
    # （272K↔372Kで変動）なので、既定値を372Kとする。Claude Codeはここから出力領域を予約して
    # auto-compactするため、CLAUDE_CODE_AUTO_COMPACT_WINDOWを340Kに上書きすると余裕を二重に
    # 差し引いてしまう。グローバル設定の500Kはmodel contextの372Kにcapされるため、そのままでよい。
    # [1m] は基盤モデルが実際に1M contextをサポートする場合の指定なので使用しない。
    # statuslineも実態に合う /372k 表示になる。
    #
    # settings ファイルの env は OS 環境変数に勝つため、プロジェクトの .claude/settings.json が同じ変数を
    # 設定していても上書きできるよう --settings で渡す。優先順位は Managed > Command-line > Local >
    # Project > User（settings.md）。
    local context_settings="{\"env\":{\"CLAUDE_CODE_MAX_CONTEXT_TOKENS\":\"${CLAUDEX_CONTEXT_TOKENS:-372000}\"}}"

    # メインの推論モデル（--model）以外に、CC が内部で使う opus / sonnet / haiku エイリアスも Codex モデルに
    # 向けておく。素の Claude 名（claude-opus-* 等）に解決されると proxy 経由で意図しないモデルになるため
    # 全部マッピングする。plan mode 常用（default / opus / opusplan は opus 系に解決）なので特に opus が要る。
    #   - ANTHROPIC_DEFAULT_OPUS_MODEL:   opus エイリアス／plan mode の opusplan（plan フェーズ）→ primary と同じ sol 系
    #   - ANTHROPIC_DEFAULT_SONNET_MODEL: sonnet エイリアス／opusplan の実行フェーズ → 同じく sol 系（実作業なので flagship）
    #   - ANTHROPIC_DEFAULT_HAIKU_MODEL:  haiku エイリアス＋バックグラウンド機能（要約・タイトル生成等）→ terra 系
    #     （旧 ANTHROPIC_SMALL_FAST_MODEL は非推奨: model-config の環境変数表の注記）
    #
    # CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC は settings.jsonnet と同様に設定しない
    # （remote-control の eligibility チェックがブロックされるため）。無駄なモデル呼び出し自体は
    # settings.jsonnet の DISABLE_NON_ESSENTIAL_MODEL_CALLS で既に止まっている
    ANTHROPIC_BASE_URL="http://127.0.0.1:${CLAUDEX_PORT:-18765}" \
    ANTHROPIC_AUTH_TOKEN="unused" \
    ANTHROPIC_DEFAULT_OPUS_MODEL="$model" \
    ANTHROPIC_DEFAULT_SONNET_MODEL="$model" \
    ANTHROPIC_DEFAULT_HAIKU_MODEL="${CLAUDEX_SMALL_MODEL:-gpt-5.6-terra}" \
    CLAUDE_CODE_SUBAGENT_MODEL="${model%\[*}" \
    CLAUDE_CODE_MAX_TOOL_USE_CONCURRENCY=3 \
    CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK=1 \
    ENABLE_TOOL_SEARCH=false \
        command claude --model "$model" --settings "$context_settings" "$@"
}

# claudexf: claudex の Codex fast/priority tier 版。
# -fast サフィックスを claude-code-proxy が service_tier: "priority" に翻訳して upstream に投げる。
# メインの推論モデルと small/fast モデル（要約・タイトル生成）の両方を fast tier にする。
# 速い代わりにサブスク usage の減りが早い。quota を使い切れないとき向け。
# CLAUDEX_MODEL / CLAUDEX_SMALL_MODEL が明示指定されていればそれを優先する（fast を強制しない）。
claudexf() {
    CLAUDEX_MODEL="${CLAUDEX_MODEL:-gpt-5.6-sol-fast}" \
    CLAUDEX_SMALL_MODEL="${CLAUDEX_SMALL_MODEL:-gpt-5.6-terra-fast}" \
        claudex "$@"
}

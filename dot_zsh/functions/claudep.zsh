# claudep: Claude Code を cc-model-router 経由で起動し、1 セッションの中で Claude と Codex（gpt-*）を混ぜる
#
# claudex がモデルを丸ごと Codex に差し替えるのに対し、claudep は素の Claude（サブスクの OAuth、Opus 既定）の
# ままで、gpt-* を名乗るリクエストだけを Codex に流す。用途は `model: gpt-6-astra` を frontmatter に持つ
# subagent（レビュー等）で、素の claude で起動したセッションではそのモデル名は Anthropic に弾かれるため、
# そういう agent は claudep（または claudex）の下でしか使えない。
#
#   claude ─→ cc-model-router（127.0.0.1:8318、go/cmd/cc-model-router）
#               ├─ claude-* / model 無し → api.anthropic.com（ヘッダ・本文とも素通し。サブスクの OAuth のまま）
#               └─ gpt-*                 → cli-proxy-api（127.0.0.1:8317）→ Codex（ChatGPT サブスクの OAuth）
#
# 素通しの根拠: Claude Code は ANTHROPIC_BASE_URL だけ設定し、ANTHROPIC_AUTH_TOKEN / ANTHROPIC_API_KEY /
# apiKeyHelper を設定しなければ、gateway 越しでも claude.ai のログインを認証に使い続ける（llm-gateway.md
# "Subscriptions and gateways"）。条件は gateway が Authorization と anthropic-beta をそのまま転送することで、
# cc-model-router は Claude 向けの経路では Host 以外を触らない。Codex 向けの経路では Authorization / x-api-key を
# 落とすので、Claude のトークンが CLIProxyAPI に渡ることはない。
#
# 初回のみ Codex の認証が必要（claudex と共通）:
#   cli-proxy-api --config "$XDG_CONFIG_HOME/cli-proxy-api/config.yaml" --codex-login
#
# 注意:
#   - Claude への通信は素の claude と同じなので Claude の quota を消費する。gpt-* の分だけ ChatGPT 側の quota
#   - router と cli-proxy-api はマシン単位で 1 プロセスずつ。全 worktree・全セッションで共有し、未起動時だけ自動起動する
#   - router のログ（method / path / model / 転送先 / status / 所要時間のみ）は $XDG_STATE_HOME/cc-model-router/router.log
#
# 上書き用の環境変数: CLAUDEP_GPT_MODEL, CLAUDEP_CONTEXT_TOKENS, CLAUDEP_ROUTER_PORT
#   例) CLAUDEP_GPT_MODEL='gpt-5.6-sol' claudep     # /model の追加候補（と ANTHROPIC_CUSTOM_MODEL_OPTION）を sol にする

_claudep_router_port() {
    echo "${CLAUDEP_ROUTER_PORT:-8318}"
}

_claudep_state_dir() {
    echo "${XDG_STATE_HOME:-$HOME/.local/state}/cc-model-router"
}

# バイナリは run_onchange_after_45-build-statusline.sh.tmpl が ~/.claude/bin に置く（PATH には無い）。
# 開発時に PATH 上の別ビルドで差し替えられるよう、先に command -v を見る
_claudep_router_bin() {
    local bin
    bin="$(command -v cc-model-router 2>/dev/null)" && { echo "$bin"; return 0; }
    bin="$HOME/.claude/bin/cc-model-router"
    [[ -x "$bin" ]] && { echo "$bin"; return 0; }
    return 1
}

# _claudex_ensure_proxy と同じ作り: mkdir でロックを取った側だけが起動し、他は listen を待つ
_claudep_ensure_router() {
    local rport="$1" cport="$2"
    _claudex_port_open "$rport" && return 0

    local bin
    if ! bin="$(_claudep_router_bin)"; then
        echo "エラー: cc-model-router が見つかりません（chezmoi update で ~/.claude/bin にビルドされます）" >&2
        return 1
    fi

    local statedir
    statedir="$(_claudep_state_dir)"
    local lockdir="${TMPDIR:-/tmp}/claudep-${rport}.lock"
    if mkdir "$lockdir" 2>/dev/null; then
        mkdir -p "$statedir"
        nohup "$bin" -listen "127.0.0.1:${rport}" -codex "http://127.0.0.1:${cport}" >>"$statedir/router.log" 2>&1 &
        disown
        rmdir "$lockdir" 2>/dev/null
    fi

    local i
    for i in {1..50}; do
        _claudex_port_open "$rport" && return 0
        sleep 0.1
    done

    echo "エラー: cc-model-router が 127.0.0.1:${rport} で起動しませんでした" >&2
    echo "  ログ: ${statedir}/router.log" >&2
    return 1
}

claudep() {
    local config cport rport
    config="$(_claudex_config_path)"
    cport="$(_claudex_port "$config")"
    rport="$(_claudep_router_port)"

    _claudex_ensure_proxy "$config" "$cport" || return 1
    if ! _claudex_codex_ready "$cport"; then
        echo "エラー: Codex が未認証です。次を実行してブラウザでログインしてください（プロキシの再起動は不要）:" >&2
        echo "  cli-proxy-api --config '$config' --codex-login" >&2
        return 1
    fi
    _claudep_ensure_router "$rport" "$cport" || return 1

    local gpt_model="${CLAUDEP_GPT_MODEL:-gpt-6-astra}"

    # gpt-* は Claude Code のモデル名パターンに合わないので effort / thinking の対応を明示する（claudex と同じ理由。
    # adaptive_thinking が無いと /effort が Codex の reasoning.effort に届かない）。
    # ANTHROPIC_CUSTOM_MODEL_OPTION は /model の候補に 1 件足すだけで、Claude 側の 4 スロットは素のまま。
    # subagent の frontmatter `model: gpt-6-astra` は Claude Code が model ID をそのまま API に送るので、
    # この変数が無くても router 経由で Codex に届く（実機で確認済み）。capabilities の宣言がその subagent にも
    # 効くか（subagent 側で effort / thinking が有効になるか）は未検証。
    local caps="effort,xhigh_effort,thinking,adaptive_thinking,interleaved_thinking"

    # CLAUDE_CODE_MAX_CONTEXT_TOKENS は claude-* を名乗る ID には（DISABLE_COMPACT を併用しない限り）効かず、
    # 認識できない ID（gpt-*）にだけ効く（model-config.md "Correct the window for a gateway or custom model ID"）。
    # Claude 側の context 判定はそのままに、gpt-* に切り替えた時だけ Codex の実 context 長（872K、根拠は
    # claudex.zsh のコメント）を宣言する。--settings で渡すのはプロジェクトの settings に勝たせるため
    local settings="{\"env\":{\"CLAUDE_CODE_MAX_CONTEXT_TOKENS\":\"${CLAUDEP_CONTEXT_TOKENS:-872000}\"}}"

    # ANTHROPIC_AUTH_TOKEN / ANTHROPIC_API_KEY は絶対に設定しない（設定した瞬間サブスクのログインが使われなくなる）。
    # claudex が付けている ANTHROPIC_DEFAULT_*_MODEL / CLAUDE_CODE_SUBAGENT_MODEL / ENABLE_TOOL_SEARCH 等は
    # 「primary が gpt-*」前提の調整なので、Claude が primary の claudep では付けない
    env \
    ANTHROPIC_BASE_URL="http://127.0.0.1:${rport}" \
    ANTHROPIC_CUSTOM_MODEL_OPTION="$gpt_model" \
    ANTHROPIC_CUSTOM_MODEL_OPTION_SUPPORTED_CAPABILITIES="$caps" \
        claude --settings "$settings" "$@"
}

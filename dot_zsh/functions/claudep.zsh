# claudep: Claude Code を cc-model-router 経由で起動し、1 セッションの中で Claude と Codex（gpt-*）を混ぜる
#
# claudex がモデルを丸ごと Codex に差し替えるのに対し、claudep は素の Claude（サブスクの OAuth、既定モデルのまま）の
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
# 上書き用の環境変数: CLAUDEP_GPT_MODEL, CLAUDEP_CONTEXT_TOKENS, CLAUDEP_ROUTER_PORT, CLAUDEP_FABLE_MODEL
#   例) CLAUDEP_GPT_MODEL='gpt-5.6-sol' claudep     # /model の追加候補（と ANTHROPIC_CUSTOM_MODEL_OPTION）を sol にする
#   例) claudeap                                   # fable スロットだけ astra（下の claudeap を参照）

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

# /healthz の応答。1 行目が ok なら router 本人、2 行目以降は起動時に固定された転送先（codex=... / anthropic=...）
_claudep_router_health() {
    curl -sS -m 2 "http://127.0.0.1:${1}/healthz" 2>/dev/null
}

# ポートが開いているだけでは別のプロセス（無関係な dev server 等）かもしれないので、
# router 自身の /healthz が ok を返すことまで確認する
_claudep_router_ready() {
    [[ "$(_claudep_router_health "$1" | head -n1)" == "ok" ]]
}

# _claudex_ensure_proxy と同じ作り: mkdir でロックを取った側だけが起動し、他は listen を待つ。
# router は転送先を起動時に固定するので、既に動いている場合は /healthz の codex= が今の CLIProxyAPI のポートと
# 一致することも確認する（config.yaml の port を変えた後に古いプロセスが残っているケース）。
# バイナリの更新（chezmoi update）は検出できないので、その後は `pkill -x cc-model-router` で入れ替える。
_claudep_ensure_router() {
    local rport="$1" cport="$2"
    if _claudex_port_open "$rport"; then
        local health
        health="$(_claudep_router_health "$rport")"
        if [[ "${health%%$'\n'*}" != "ok" ]]; then
            echo "エラー: 127.0.0.1:${rport} は listen されていますが cc-model-router ではありません（/healthz が応答しない）" >&2
            echo "  別のプロセスが使っているなら CLAUDEP_ROUTER_PORT で router のポートを変えてください" >&2
            return 1
        fi
        # codex= の行を取り出して行単位で完全一致させる（部分一致だと 8317 と 831 のような前方一致を見逃す）
        local want="codex=http://127.0.0.1:${cport}" got
        got="$(printf '%s\n' "$health" | sed -n 's/^codex=//p' | head -n1)"
        if [[ "codex=${got}" != "$want" ]]; then
            echo "エラー: 起動中の cc-model-router の転送先（codex=${got}）が今の設定（${want}）と違います" >&2
            echo "  古いプロセスを止めてから再実行してください: pkill -x cc-model-router" >&2
            return 1
        fi
        return 0
    fi

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
        _claudep_router_ready "$rport" && return 0
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

    # ANTHROPIC_CUSTOM_MODEL_OPTION は /model の候補に 1 件足すだけで、Claude 側の 4 スロットは素のまま。
    # _SUPPORTED_CAPABILITIES は /model でその gpt-* を primary に切り替えたとき用（gpt-* は Claude Code の
    # モデル名パターンに合わないので effort / thinking の対応を明示する。claudex と同じ理由）。
    # subagent（frontmatter `model: gpt-6-astra`、例: dot_claude/agents/gpt-review.md）には関係ない。Claude Code は
    # model ID をそのまま送り、thinking は main の設定を継承し、effort は frontmatter の effort:（無ければ settings の
    # effortLevel）が output_config.effort になる。この変数の有無で変わらないことを捕捉サーバーで確認済み
    # （詳細は CLAUDE.md の claudep セクション）。
    local caps="effort,xhigh_effort,thinking,adaptive_thinking,interleaved_thinking"

    # CLAUDE_CODE_MAX_CONTEXT_TOKENS は claude-* を名乗る ID には（DISABLE_COMPACT を併用しない限り）効かず、
    # 認識できない ID（gpt-*）にだけ効く（model-config.md "Correct the window for a gateway or custom model ID"）。
    # Claude 側の context 判定はそのままに、gpt-* に切り替えた時だけ Codex の実 context 長（872K、根拠は
    # claudex.zsh のコメント）を宣言する。--settings で渡すのはプロジェクトの settings に勝たせるため
    local settings="{\"env\":{\"CLAUDE_CODE_MAX_CONTEXT_TOKENS\":\"${CLAUDEP_CONTEXT_TOKENS:-872000}\"}}"

    # claudeap 用: fable スロットだけ Codex モデルに向ける（CLAUDEP_FABLE_MODEL が非空のとき）。
    # ANTHROPIC_DEFAULT_FABLE_MODEL が効くのは `fable` エイリアス（--model fable、/model fable、model: fable の
    # subagent）だけで、settings.json の model に入っている claude-fable-5-1[1m] のような full ID には効かない。
    # そのため claudeap は --model fable を明示して起動する（"$@" より前に置くので、呼び出し側の --model が勝つ）。
    # opus / sonnet / haiku のスロットは触らないので Claude のまま。
    local -a fable_env=() fable_args=()
    if [[ -n "${CLAUDEP_FABLE_MODEL:-}" ]]; then
        fable_env=(
            ANTHROPIC_DEFAULT_FABLE_MODEL="$CLAUDEP_FABLE_MODEL"
            ANTHROPIC_DEFAULT_FABLE_MODEL_SUPPORTED_CAPABILITIES="$caps"
        )
        fable_args=(--model fable)
    fi

    # ANTHROPIC_AUTH_TOKEN / ANTHROPIC_API_KEY は設定した瞬間サブスクのログインが使われなくなる（API 課金か認証失敗）
    # ので、呼び出し元のシェルに入っていても継承しないよう明示的に外す。
    # claudex が付けている ANTHROPIC_DEFAULT_*_MODEL / CLAUDE_CODE_SUBAGENT_MODEL / ENABLE_TOOL_SEARCH 等は
    # 「primary が gpt-*」前提の調整なので、Claude が primary の claudep では付けない
    env -u ANTHROPIC_AUTH_TOKEN -u ANTHROPIC_API_KEY \
    ANTHROPIC_BASE_URL="http://127.0.0.1:${rport}" \
    ANTHROPIC_CUSTOM_MODEL_OPTION="$gpt_model" \
    ANTHROPIC_CUSTOM_MODEL_OPTION_SUPPORTED_CAPABILITIES="$caps" \
    "${fable_env[@]}" \
        claude --settings "$settings" "${fable_args[@]}" "$@"
}

# claudeap: claudep の fable スロットを GPT-6 Astra に向けた版。primary は --model fable 経由で astra になり、
# opus / sonnet / haiku（plan mode の opusplan、model: sonnet の subagent、バックグラウンド処理）は Claude のまま。
# claudex との違いは「Claude 側のスロットを丸ごと差し替えない」こと。Claude の quota は opus 以下の分だけ減る。
# 上書き: CLAUDEP_FABLE_MODEL（既定 gpt-6-astra）
claudeap() {
    CLAUDEP_FABLE_MODEL="${CLAUDEP_FABLE_MODEL:-gpt-6-astra}" claudep "$@"
}

# claudeapf: claudeap の Codex priority tier 版。fable スロットを gpt-6-astra-fast（config.yaml の別名。CLIProxyAPI が
# payload.override で service_tier: priority を付けて gpt-6-astra に送る）に向ける。2x speed の代わりに ChatGPT 側の
# usage の減りが早い。Claude Code の fast mode（fastMode）は Opus 専用で gpt-* には効かないので使わない。
# Claude 側（opus / sonnet / haiku）は通常速度のまま。
claudeapf() {
    CLAUDEP_FABLE_MODEL="${CLAUDEP_FABLE_MODEL:-gpt-6-astra-fast}" claudep "$@"
}

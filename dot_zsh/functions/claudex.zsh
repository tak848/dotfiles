# claudex: Claude Code のハーネスのまま、モデルだけ Codex（GPT-5.6 Sol / GPT-6 Astra）にする
#
# CLIProxyAPI（router-for-me/CLIProxyAPI、mise の github backend で導入）が Anthropic Messages API 互換の
# プロキシとして立ち、ChatGPT サブスクの OAuth 経由で Codex backend に転送する。ツールループ・サブエージェント・
# hooks・MCP は Claude Code のものがそのまま効き、推論するモデルだけが入れ替わる。
# 設定は $XDG_CONFIG_HOME/cli-proxy-api/config.yaml（chezmoi 管理: dot_config/cli-proxy-api/config.yaml.tmpl）。
# OAuth トークンと serve.log は $XDG_STATE_HOME/cli-proxy-api/。
#
# 初回のみ認証が必要（ブラウザで ChatGPT Plus/Pro アカウントにログイン）:
#   cli-proxy-api --config "$XDG_CONFIG_HOME/cli-proxy-api/config.yaml" --codex-login
#
# 注意:
#   - 消費するのは ChatGPT 側の quota。Claude のサブスクは減らない（素の claude は従来通り）
#   - プロキシはマシン単位で 1 プロセス。全 worktree・全セッションが 1 つを共有する
#   - Anthropic は非 Claude モデルへの gateway ルーティングを公式サポートしていない
#   - モデルカタログは起動時と 3 時間ごとに router-for-me/models（GitHub）から取り直すので、Codex に新モデルが
#     出てもバイナリ更新を待たずに使える
#
# 上書き用の環境変数: CLAUDEX_MODEL, CLAUDEX_FABLE_MODEL, CLAUDEX_MID_MODEL, CLAUDEX_SMALL_MODEL, CLAUDEX_CONTEXT_TOKENS
#   例) CLAUDEX_MODEL='gpt-6-astra' claudex         # primary も astra にする
#   例) CLAUDEX_CONTEXT_TOKENS=272000 claudex      # backend が 272K に巻き戻った日は下げる
#
# 以下の _claudex_* ヘルパーは claudep（dot_zsh/functions/claudep.zsh）からも使う。~/.zsh/functions/*.zsh は
# 全て source されるので、名前や引数を変えるときは claudep 側も合わせること。

_claudex_config_path() {
    echo "${XDG_CONFIG_HOME:-$HOME/.config}/cli-proxy-api/config.yaml"
}

# config.yaml の auth-dir と同じ場所。CLIProxyAPI は環境変数を展開しないので、config.yaml 側は chezmoi の
# テンプレートで apply 時の XDG_STATE_HOME を焼き込んでいる（dot_config/cli-proxy-api/config.yaml.tmpl）
_claudex_state_dir() {
    echo "${XDG_STATE_HOME:-$HOME/.local/state}/cli-proxy-api"
}

# listen ポートは config.yaml が唯一の情報源。環境変数で別ポートを渡せるようにすると YAML と食い違うので読むだけにする
_claudex_port() {
    local port
    port="$(sed -n 's/^port:[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$1" 2>/dev/null | head -n1)"
    echo "${port:-8317}"
}

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
    local config="$1" port="$2"
    _claudex_port_open "$port" && return 0

    if ! command -v cli-proxy-api >/dev/null 2>&1; then
        echo "エラー: cli-proxy-api が見つかりません（mise install で導入されます）" >&2
        return 1
    fi
    if [[ ! -f "$config" ]]; then
        echo "エラー: $config がありません（chezmoi update で配置されます）" >&2
        return 1
    fi

    local statedir
    statedir="$(_claudex_state_dir)"
    local lockdir="${TMPDIR:-/tmp}/claudex-${port}.lock"
    if mkdir "$lockdir" 2>/dev/null; then
        mkdir -p "$statedir"
        nohup cli-proxy-api --config "$config" >>"$statedir/serve.log" 2>&1 &
        disown
        rmdir "$lockdir" 2>/dev/null
    fi

    local i
    for i in {1..50}; do
        _claudex_port_open "$port" && return 0
        sleep 0.1
    done

    echo "エラー: cli-proxy-api が 127.0.0.1:${port} で起動しませんでした" >&2
    echo "  ログ: ${statedir}/serve.log" >&2
    return 1
}

# Codex の OAuth が入っているか。CLIProxyAPI は認証済みプロバイダのモデルだけを /v1/models に出すので、
# gpt-* が 1 つも無ければ未認証（またはトークン失効）と判断する。auth ファイルの命名に依存しない
_claudex_codex_ready() {
    curl -sS -m 5 "http://127.0.0.1:${1}/v1/models" -H 'Authorization: Bearer unused' 2>/dev/null | grep -q '"gpt-'
}

claudex() {
    local config port
    config="$(_claudex_config_path)"
    port="$(_claudex_port "$config")"
    _claudex_ensure_proxy "$config" "$port" || return 1

    if ! _claudex_codex_ready "$port"; then
        echo "エラー: Codex が未認証です。次を実行してブラウザでログインしてください（プロキシの再起動は不要）:" >&2
        echo "  cli-proxy-api --config '$config' --codex-login" >&2
        return 1
    fi

    # Codex の live カタログ（codex debug models）上の序列は astra（"Our most capable model"、GPT-6）
    # > sol（"Reliable agentic workhorse"）> terra（balanced）> luna（fast/affordable）で、Claude 側の
    # fable > opus > sonnet > haiku というスロットの重みに素直に対応する。primary（--model）は素の Claude の
    # 既定が Opus であるのに合わせて sol のままにし、fable スロットだけ astra に向ける。
    # claudexf 用。CLAUDEX_FAST=1 なら各モデルを <model>-fast（config.yaml の別名。CLIProxyAPI が payload.override で
    # service_tier: priority を付けて素の名前で Codex に送る）に向ける。Claude Code の fast mode（fastMode +
    # speed: fast）は Opus 専用で、gpt-* の primary では speed が送られないことを捕捉サーバーで確認したため、
    # モデル名で tier を分ける方式にした。CLAUDEX_*_MODEL で明示された名前には付けない。
    local suffix=""
    [[ "${CLAUDEX_FAST:-0}" == 1 ]] && suffix="-fast"
    local model="${CLAUDEX_MODEL:-gpt-5.6-sol${suffix}}"
    local fable_model="${CLAUDEX_FABLE_MODEL:-gpt-6-astra${suffix}}"
    local mid_model="${CLAUDEX_MID_MODEL:-gpt-5.6-terra${suffix}}"
    local small_model="${CLAUDEX_SMALL_MODEL:-gpt-5.6-luna${suffix}}"

    # Claude Code は model ID のパターンで effort / thinking 対応を判定するため、gpt-* だとどちらも無効になる。
    # 各スロットの _SUPPORTED_CAPABILITIES で明示する。adaptive_thinking が重要で、これが無いと Claude Code は
    # thinking を {type: enabled, budget_tokens} で送り、CLIProxyAPI は budget から effort を逆算して
    # output_config.effort（/effort の値）を無視する。adaptive なら /effort がそのまま Codex の reasoning.effort になる。
    # max は Codex 側に無い（low/medium/high/xhigh）ので max_effort は宣言しない。
    local caps="effort,xhigh_effort,thinking,adaptive_thinking,interleaved_thinking"

    # CLAUDE_CODE_MAX_CONTEXT_TOKENS は、ANTHROPIC_BASE_URL 経由の未認識モデルについて Claude Code が
    # 仮定する context window を上書きする。値の根拠は ChatGPT アカウントに配られる Codex の live カタログで、
    # 次のコマンドで確認できる:
    #   codex debug models | jq -r '.models[] | "\(.slug) \(.context_window) \(.max_context_window)"'
    # カタログの既定は context_window=272000 だが、クライアントは max_context_window まで引き上げてよい
    # （codex 本体も model_context_window をこの値で clamp する）。2026-08 に OpenAI が API key 限定だった
    # 1M context を ChatGPT アカウントにも解禁し、このアカウントの max_context_window は 872000
    # （2026-08-31 時点。おそらく 1,000,000 から出力 128,000 を引いた値）。astra も同じ 872000。
    # カタログは過去に 272K ↔ 372K と揺れているので、巻き戻ったら CLAUDEX_CONTEXT_TOKENS で下げる。
    #
    # dot_zshenv.tmpl の CLAUDE_CODE_AUTO_COMPACT_WINDOW=750000 が model context より小さいため、
    # 約 122K を残して compaction が先に走る。これがカタログ巻き戻り時の保険にもなる。
    #
    # settings ファイルの env は OS 環境変数に勝つため、プロジェクトの .claude/settings.json が同じ変数を
    # 設定していても上書きできるよう --settings で渡す。優先順位は Managed > Command-line > Local >
    # Project > User（settings.md）。
    local settings="{\"env\":{\"CLAUDE_CODE_MAX_CONTEXT_TOKENS\":\"${CLAUDEX_CONTEXT_TOKENS:-872000}\"}}"

    # メインの推論モデル（--model）以外に、CC が内部で使う fable / opus / sonnet / haiku エイリアスも Codex
    # モデルに向けておく。素の Claude 名（claude-opus-* 等）に解決されると proxy に「unknown provider」で
    # 弾かれるため全部マッピングする（config.yaml の oauth-model-alias が最後の逃げ道）。plan mode 常用
    # （default / opus / opusplan は opus 系に解決）なので特に opus が要る。
    # subagent も、定義側で model を明示しているものはこのスロット経由で解決される。
    #   - ANTHROPIC_DEFAULT_FABLE_MODEL:  fable エイリアス（/model fable、model: fable の subagent）→ astra 系
    #   - ANTHROPIC_DEFAULT_OPUS_MODEL:   opus エイリアス／plan mode の opusplan（plan フェーズ）→ primary と同じ sol 系
    #   - ANTHROPIC_DEFAULT_SONNET_MODEL: sonnet エイリアス／opusplan の実行フェーズ → terra 系
    #   - ANTHROPIC_DEFAULT_HAIKU_MODEL:  haiku エイリアス＋バックグラウンド機能（要約・タイトル生成等）→ luna 系
    #     （旧 ANTHROPIC_SMALL_FAST_MODEL は非推奨: model-config の環境変数表の注記）
    #
    # CLAUDE_CODE_ALWAYS_ENABLE_EFFORT は _SUPPORTED_CAPABILITIES で effort を宣言していれば冗長だが、
    # --model に渡した名前がどのスロットにも一致しないとき（CLAUDEX_MODEL で独自指定した場合）の保険として付ける。
    #
    # CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC は settings.jsonnet と同様に設定しない
    # （remote-control の eligibility チェックがブロックされるため）。無駄なモデル呼び出し自体は
    # settings.jsonnet の DISABLE_NON_ESSENTIAL_MODEL_CALLS で既に止まっている
    env \
    ANTHROPIC_BASE_URL="http://127.0.0.1:${port}" \
    ANTHROPIC_AUTH_TOKEN="unused" \
    ANTHROPIC_DEFAULT_FABLE_MODEL="$fable_model" \
    ANTHROPIC_DEFAULT_FABLE_MODEL_SUPPORTED_CAPABILITIES="$caps" \
    ANTHROPIC_DEFAULT_OPUS_MODEL="$model" \
    ANTHROPIC_DEFAULT_OPUS_MODEL_SUPPORTED_CAPABILITIES="$caps" \
    ANTHROPIC_DEFAULT_SONNET_MODEL="$mid_model" \
    ANTHROPIC_DEFAULT_SONNET_MODEL_SUPPORTED_CAPABILITIES="$caps" \
    ANTHROPIC_DEFAULT_HAIKU_MODEL="$small_model" \
    ANTHROPIC_DEFAULT_HAIKU_MODEL_SUPPORTED_CAPABILITIES="$caps" \
    CLAUDE_CODE_ALWAYS_ENABLE_EFFORT=1 \
    CLAUDE_CODE_SUBAGENT_MODEL="${model%\[*}" \
    CLAUDE_CODE_MAX_TOOL_USE_CONCURRENCY=3 \
    CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK=1 \
    ENABLE_TOOL_SEARCH=false \
        claude --model "$model" --settings "$settings" "$@"
}

# claudexf: claudex の Codex priority tier 版。4 スロットとも <model>-fast（config.yaml の別名）に向け、CLIProxyAPI が
# service_tier: priority を付けて Codex に送る。速い代わりにサブスク usage の減りが早い。quota を使い切れないとき向け。
claudexf() {
    CLAUDEX_FAST=1 claudex "$@"
}

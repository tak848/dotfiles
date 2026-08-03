# codexp: CODEX_PROFILE を解釈して codex に --profile を自動付与する
#
# Codex CLI の --profile <name> は $CODEX_HOME/<name>.config.toml を base config
# （$CODEX_HOME/config.toml）の上にレイヤーする。repo ごとに Codex の設定を切り替えたいが、
# Codex 自身には profile を選ぶ環境変数が無いため、ここで補う。
#
#   export CODEX_PROFILE=sample
#   codexp                  # codex --profile sample と同じ
#   codexp exec "..."       # codex --profile sample exec "..." と同じ
#   codexp -p other exec    # 明示指定が優先される（二重付与しない）
#   codexp login            # profile 非対応サブコマンドには付与しない（下記参照）
#
# CODEX_PROFILE が未設定なら素の codex と完全に同じ挙動になる。
#
# 注意:
#   - --profile が使えるのは runtime サブコマンド（サブコマンド無し / exec / review /
#     resume / archive / delete / unarchive / fork / mcp / sandbox / debug prompt-input）
#     だけで、login や doctor に付けると Codex はエラーで即終了する
#   - Codex は存在しない profile 名を黙って無視する。typo に気づけないため、
#     ここで profile ファイルの存在を確認して警告する
#   - base config に legacy な [profiles.<name>] が残っていると、同名の --profile <name>
#     はエラーになる。profile 名は dot_codex/modify_config.toml の [profiles.*] と
#     衝突させないこと

codexp() {
    # --profile を付けてはいけないサブコマンド。`codex --help` のサブコマンド一覧から
    # runtime commands を引いた差集合（a は apply の alias。exec の alias e は付与側）。
    # debug は prompt-input だけ profile 対応だが、デバッグ用途なので debug 単位で除外する。
    local -a no_profile_subcommands=(
        login logout plugin mcp-server app-server remote-control app completion
        update doctor debug apply a cloud exec-server features help
    )

    local profile="${CODEX_PROFILE:-}"
    if [ -z "$profile" ]; then
        command codex "$@"
        return
    fi

    # 明示指定があればそちらを優先する
    local arg
    for arg in "$@"; do
        case "$arg" in
        -p | --profile | --profile=*)
            command codex "$@"
            return
            ;;
        esac
    done

    # 最初の非オプショントークンをサブコマンド候補とみなす。`codex [OPTIONS] [PROMPT]` 形態では
    # プロンプト文字列が来るが、denylist に無ければ付与する側に倒れるだけで害はない。逆に
    # オプションの値をサブコマンドと誤認した場合も「付与しない」方向にしか外れない。
    local subcommand=""
    for arg in "$@"; do
        if [[ "$arg" != -* ]]; then
            subcommand="$arg"
            break
        fi
    done
    if [ -n "$subcommand" ] && (( ${no_profile_subcommands[(Ie)$subcommand]} )); then
        command codex "$@"
        return
    fi

    local profile_file="${CODEX_HOME:-$HOME/.codex}/${profile}.config.toml"
    if [ ! -f "$profile_file" ]; then
        echo "警告: CODEX_PROFILE='${profile}' に対応する ${profile_file} がありません。profile 無しで起動します。" >&2
        command codex "$@"
        return
    fi

    command codex --profile "$profile" "$@"
}

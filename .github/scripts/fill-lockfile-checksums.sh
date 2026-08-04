#!/bin/bash
# mise.lock の platform エントリのうち、url はあるが checksum が無いものを埋める。
#
# mise は backend が checksum を供給できる場合（aqua backend が aqua-registry から取る等）
# に限って `mise lock` で checksum を書き込む。http backend は checksum_url が設定されて
# いる場合しかクロスプラットフォームの checksum を解決できず、配布元が checksum ファイルを
# 公開していないツール（x.ai の Grok Build CLI 等）は url だけが記録される。
#
# そのままだと install 時に整合性検証が働かないため、CI で実アーティファクトを取得して
# sha256 を計算し、lockfile に追記する。mise は lockfile に checksum があれば install 時に
# 検証し、不一致なら失敗する。
#
# 使い方: fill-lockfile-checksums.sh <lockfile> [<lockfile> ...]
set -euo pipefail

sha256_of_stdin() {
    if command -v sha256sum &>/dev/null; then
        sha256sum | cut -d' ' -f1
    else
        shasum -a 256 | cut -d' ' -f1
    fi
}

for lockfile in "$@"; do
    if [[ ! -f "$lockfile" ]]; then
        echo "skip: $lockfile (not found)" >&2
        continue
    fi

    # checksum が欠けている platform セクションを「セクション行<TAB>URL」で列挙する。
    # mise.lock はセクション間が空行で区切られるため、空行と EOF を終端として扱う。
    pending=$(
        awk '
            function flush() {
                if (section ~ /platforms\./ && !has_checksum && url != "") {
                    print section "\t" url
                }
                section = ""; has_checksum = 0; url = ""
            }
            /^\[/                { flush(); section = $0; next }
            /^checksum = /       { has_checksum = 1; next }
            /^url = /            { url = $0; sub(/^url = "/, "", url); sub(/"$/, "", url); next }
            END                  { flush() }
        ' "$lockfile"
    )

    if [[ -z "$pending" ]]; then
        echo "$lockfile: すべての platform エントリに checksum がある"
        continue
    fi

    # 同一 URL を指す platform が複数ある（linux-x64 と linux-x64-musl 等）ため、
    # URL 単位で 1 回だけダウンロードする。
    mapfile=$(mktemp)
    declare -A url_to_sha=()
    while IFS=$'\t' read -r section url; do
        [[ -z "$section" ]] && continue
        if [[ -z "${url_to_sha[$url]:-}" ]]; then
            echo "checksum を計算中: $url" >&2
            url_to_sha[$url]=$(curl -sfL --retry 3 "$url" | sha256_of_stdin)
            if [[ -z "${url_to_sha[$url]}" ]]; then
                echo "ERROR: ダウンロードに失敗しました: $url" >&2
                exit 1
            fi
        fi
        printf '%s\t%s\n' "$section" "${url_to_sha[$url]}" >>"$mapfile"
    done <<<"$pending"

    # TOML のキー順序に意味は無いので、セクション行の直後に checksum を挿入する。
    tmp=$(mktemp)
    awk -v mapfile="$mapfile" '
        BEGIN {
            while ((getline line < mapfile) > 0) {
                split(line, kv, "\t")
                sha[kv[1]] = kv[2]
            }
        }
        { print }
        /^\[/ { if ($0 in sha) print "checksum = \"sha256:" sha[$0] "\"" }
    ' "$lockfile" >"$tmp"
    mv "$tmp" "$lockfile"
    rm -f "$mapfile"
    unset url_to_sha

    echo "$lockfile: $(printf '%s\n' "$pending" | wc -l | tr -d ' ') 件の checksum を追記した"
done

#!/usr/bin/env bash
# mise lock が <dir>/.mise/locks/ に書いた sidecar（npm / pypi ツールの依存グラフ）を <dir>/locks/ に移し、
# <dir>/mise.lock の参照パスを書き換える。
#
# dot_config/mise/mise.lock は chezmoi で ~/.config/mise/mise.lock に配布する。mise は sidecar の置き場を
# lockfile のパスから決め（src/lockfile/graph.rs の sidecar_root）、親が .config の mise/mise.lock なら
# <dir>/locks、それ以外は <dir>/.mise/locks にする。repo の dot_config/mise は後者、配布先は前者なので、
# repo の置き場のまま配ると、配布先の mise が lockfile の保存のたびに sidecar を locks/ へ写して
# mise.lock を書き換え、chezmoi update が「has changed since chezmoi last wrote it?」を出す。
# 配布先と同じ locks/ で記録しておけば、配布先の mise は何も書き換えない。
# digest は sidecar の中身から計算されるので、パスを書き換えても変わらない。
#
# 使い方: relocate-mise-sidecars.sh <mise.lock のあるディレクトリ>
# mise lock の直後に呼ぶ。<dir>/.mise/locks が無いとき（lock せずに呼んだとき）は既存の locks/ に触れず、
# パスの書き換えと検査だけを行う。

set -euo pipefail

dir="${1:?usage: relocate-mise-sidecars.sh <directory containing mise.lock>}"
lock="$dir/mise.lock"
from="$dir/.mise/locks"
to="$dir/locks"

if [ ! -f "$lock" ]; then
  echo "relocate-mise-sidecars: $lock がありません" >&2
  exit 1
fi

if [ -d "$from" ]; then
  rm -rf "$to"
  mv "$from" "$to"
  rmdir "$dir/.mise" 2>/dev/null || true
fi

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
sed 's#path = "\.mise/locks/#path = "locks/#g' "$lock" >"$tmp"
cat "$tmp" >"$lock"

# 置換漏れ（mise.lock の書式が変わった等）と、参照先の欠けを検出する
if grep -n '"\.mise/locks/' "$lock" >&2; then
  echo "relocate-mise-sidecars: $lock に .mise/locks/ への参照が残っています" >&2
  exit 1
fi
missing=0
while IFS= read -r ref; do
  if [ ! -d "$dir/$ref" ]; then
    echo "relocate-mise-sidecars: $lock が参照する $dir/$ref がありません" >&2
    missing=1
  fi
done < <(grep -o 'path = "locks/[^"]*"' "$lock" | sed 's/^path = "//; s/"$//')
exit "$missing"

#!/usr/bin/env zsh
# zsh の login の対話シェル（ターミナルが起動するのと同じ `zsh -lic exit`）の起動時間を測り、
# 時間のかかっている箇所を出す。.github/workflows/zsh-startup.yaml と手元の両方で使う。
# ${ZDOTDIR:-$HOME} に置かれた実際の設定を読む。計測するだけで、合否は判定しない。
#
# 使い方: measure-zsh-startup.zsh [--markdown <ファイル>] [回数（既定 5）]
#   --markdown  同じ結果を Markdown でこのファイルに書く（ワークフローが job summary と PR コメントに使う）
#
# 時間のかかっている箇所は、PS4 に時刻を入れた xtrace を取り、次の行に進むまでの時間が長い順に出す。
# 関数やプラグインの中の行も出るので、.zshrc のどの行から呼ばれたかは前後の行で判断する。
# xtrace 自体の負荷があるので、ここに出る秒数は起動時間の計測より長めになる。
# zsh -c で起動するシェルは履歴ファイルを書き換えない。

emulate -L zsh
zmodload zsh/datetime zsh/mathfunc

usage="usage: measure-zsh-startup.zsh [--markdown <ファイル>] [回数（1 以上の整数）]"
zparseopts -D -E -F -- -markdown:=opt_markdown 2>/dev/null || { print -u2 -r -- "$usage"; exit 2; }
markdown=${opt_markdown[-1]:-}
runs=${1:-5}
top=15
zshrc="${ZDOTDIR:-$HOME}/.zshrc"

if [[ $runs != <1-> ]]; then
  print -u2 -r -- "$usage"
  exit 2
fi
if [[ ! -r $zshrc ]]; then
  print -u2 -r -- "measure-zsh-startup: $zshrc がありません。chezmoi で zsh の設定を展開してから実行してください"
  exit 1
fi

print -r -- "=== ウォームアップ（zinit の annex の取得など、初回だけの処理を済ませる） ==="
zsh -lic exit

print -r -- ""
print -r -- "=== 起動時間（zsh -lic exit を $runs 回） ==="
times_ms=()
for i in {1..$runs}; do
  start=$EPOCHREALTIME
  zsh -lic exit >/dev/null 2>&1
  times_ms+=($(( int((EPOCHREALTIME - start) * 1000) )))
  printf '%d 回目: %5d ms\n' $i $times_ms[-1]
done
sorted=(${(on)times_ms})
median_ms=$sorted[$(( (runs + 1) / 2 ))]
printf '中央値: %d ms\n' $median_ms

trace=$(mktemp)
PS4='+%D{%s.%6.} %N:%i> ' zsh -o xtrace -lic exit >/dev/null 2>"$trace"
slow=$(awk '/^\+[0-9]+\.[0-9]+ / {
         t = substr($1, 2) + 0
         if (prev != "") printf "%8.3f  %s\n", t - pt, substr(prev, 1, 200)
         pt = t; prev = substr($0, length($1) + 2)
       }' "$trace" | sort -rn | head -n $top)
rm -f -- "$trace"

print -r -- ""
print -r -- "=== 次の行に進むまでの時間が長い箇所（上位 $top 件。左が秒、右がその間に実行していた行） ==="
print -r -- "$slow"

if [[ -n $markdown ]]; then
  {
    print -r -- "### $(uname -sm)"
    print -r -- ""
    print -r -- "\`zsh -lic exit\` の中央値: **${median_ms} ms**（$runs 回: ${(j:, :)times_ms} ms）"
    print -r -- ""
    print -r -- "<details><summary>次の行に進むまでの時間が長い箇所（上位 $top 件。xtrace の負荷込み）</summary>"
    print -r -- ""
    print -r -- '```'
    print -r -- "$slow"
    print -r -- '```'
    print -r -- ""
    print -r -- "</details>"
  } > "$markdown"
fi

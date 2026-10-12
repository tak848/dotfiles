#!/usr/bin/env zsh
# zsh の login の対話シェル（ターミナルが起動するのと同じ `zsh -lic`）を検査する。
# CI（.github/workflows/ci.yaml）と手元の両方で使う。${ZDOTDIR:-$HOME} に置かれた実際の設定を読む。
#
#   1. 履歴の上限が /etc/zshrc に上書きされていないか。macOS の /etc/zshrc は HISTSIZE=2000 /
#      SAVEHIST=1000 を代入し、~/.zshenv より後に読まれる（#309 で .zshenv に移して履歴が切り詰められた）
#   2. .zshrc が eval "$(...)" / source <(...) しているコマンドの出力が大きすぎないか。出力は起動のたびに
#      構文解析される（pup の補完 3.7MB の eval で起動に 10 秒かかっていた）
#   3. 起動時間の中央値がしきい値以内か。超えたら時刻付きの xtrace を取り、遅い箇所を出す
#
# 使い方: check-zsh-startup.zsh
#   ZSH_STARTUP_THRESHOLD  起動時間のしきい値（秒、既定 3）
#
# zsh -c で起動するシェルは履歴ファイルを書き換えない。

emulate -L zsh
setopt pipe_fail
zmodload zsh/datetime zsh/mathfunc

threshold=${ZSH_STARTUP_THRESHOLD:-3}
runs=5
max_eval_bytes=$((256 * 1024))
zshrc="${ZDOTDIR:-$HOME}/.zshrc"
failed=0

fail() {
  print -u2 -r -- "NG: $*"
  failed=1
}

# 対話シェルの中で引数のコードを実行し、marker で始まる行の残りを返す。
# .zshrc が出す他の出力（zinit のログ等）と混ざらないよう、marker で拾う
interactive_value() {
  local marker=$1 code=$2
  zsh -lic "print -r -- \"$marker \$($code)\"" 2>/dev/null | sed -n "s/^$marker //p" | tail -n 1
}

if [[ ! -r $zshrc ]]; then
  print -u2 -r -- "NG: $zshrc がありません。chezmoi で zsh の設定を展開してから実行してください"
  exit 1
fi

print -r -- "=== ウォームアップ（zinit の annex の取得など、初回だけの処理を済ませる） ==="
zsh -lic exit

print -r -- ""
print -r -- "=== 履歴の上限 ==="
read -r histsize savehist <<<"$(interactive_value HISTVARS 'print -r -- $HISTSIZE $SAVEHIST')"
print -r -- "HISTSIZE=$histsize SAVEHIST=$savehist"
if [[ $histsize != <-> || $savehist != <-> ]] || (( savehist < 100000 || histsize < savehist )); then
  fail "対話シェルの履歴の上限が小さい（HISTSIZE=$histsize SAVEHIST=$savehist）。macOS の /etc/zshrc は HISTSIZE=2000 / SAVEHIST=1000 を代入し、~/.zshenv より後に読まれる。履歴の変数は dot_zshenv.tmpl ではなく dot_zshrc.tmpl で設定してください"
fi

print -r -- ""
print -r -- "=== .zshrc が eval / source <(...) しているコマンドの出力（上限 $max_eval_bytes バイト） ==="
# コマンドの解決は対話シェルの PATH で行う（mise activate 後の PATH でないと見つからないツールがある）
interactive_path=$(interactive_value PATHVAR 'print -r -- $PATH')
cmds=(${(f)"$(grep -oE 'eval "\$\([^)]*\)"|source <\([^)]*\)' "$zshrc" | sed -E 's/^eval "\$\((.*)\)"$/\1/; s/^source <\((.*)\)$/\1/')"})
for cmd in $cmds; do
  name=${${(z)cmd}[1]}
  if ! (PATH=$interactive_path; whence -p -- $name >/dev/null); then
    print -r -- "      skip  $cmd（$name が見つからない）"
    continue
  fi
  bytes=$(PATH=$interactive_path zsh -fc "$cmd" 2>/dev/null | wc -c)
  bytes=${bytes//[[:space:]]/}
  printf '%10d  %s\n' $bytes "$cmd"
  if (( bytes > max_eval_bytes )); then
    fail "\`$cmd\` の出力が $bytes バイトある（上限 $max_eval_bytes）。.zshrc で eval すると起動のたびに構文解析される。補完なら run_onchange_after_12-zsh-completions.sh.tmpl のようにファイルへ書き出して zcompile してください"
  fi
done

print -r -- ""
print -r -- "=== 起動時間（zsh -lic exit を $runs 回、しきい値 $threshold 秒） ==="
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
if (( median_ms > threshold * 1000 )); then
  fail "起動時間の中央値 ${median_ms} ms がしきい値 ${threshold} 秒を超えた。下は時刻付きの xtrace で、次の行に進むまでの時間が長い順（左が秒、右がその間に実行していた行）"
  trace=$(mktemp)
  PS4='+%D{%s.%6.} %N:%i> ' zsh -o xtrace -lic exit >/dev/null 2>"$trace"
  awk '/^\+[0-9]+\.[0-9]+ / {
         t = substr($1, 2) + 0
         if (prev != "") printf "%8.3f  %s\n", t - pt, substr(prev, 1, 200)
         pt = t; prev = substr($0, length($1) + 2)
       }' "$trace" | sort -rn | head -n 15 >&2
  rm -f -- "$trace"
fi

print -r -- ""
if (( failed )); then
  print -u2 -r -- "zsh の起動の検査に失敗しました（上の NG を参照）"
  exit 1
fi
print -r -- "zsh の起動の検査はすべて通りました"

#!/bin/sh
set -e

# clone 後に手動で実行する初回セットアップ用スクリプト。
#
# - chezmoi が既に PATH にあればそのまま `chezmoi init --apply tak848`
# - 無ければリポジトリ同梱の mise bootstrap script で mise を起動し、
#   ルート .mise.toml で pin した chezmoi を install してから apply する
#
# README の one-liner（get.chezmoi.io 経由）と違い、こちらは clone 済みの
# リポジトリ資産のみで完結し、checksum もリポジトリ内 mise.lock で検証される。
#
# apply には --force を付ける。mise install が chezmoi 管理下の
# ~/.config/mise/mise.lock を書き換えるため、--force 無しだと 2 回目以降の実行が
# 「chezmoi が書いた後に変更されている」の上書き確認で止まり、TTY の無い環境では
# `could not open a new TTY` で失敗して冪等でなくなる。remote main が
# single source of truth なので、ローカル変更を上書きするのは意図通り。

cd "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

if command -v chezmoi >/dev/null 2>&1; then
  exec chezmoi init --apply --force tak848
fi

MISE_BOOTSTRAP="$(pwd)/dot_local/bin/executable_mise"
if [ ! -f "$MISE_BOOTSTRAP" ]; then
  echo "Error: mise bootstrap script not found at $MISE_BOOTSTRAP" >&2
  exit 1
fi
# chezmoi apply 前のため source 側の executable bit に依存しない（safety net）
chmod +x "$MISE_BOOTSTRAP"

# bootstrap script を素の名前で呼ぶと mise が argv[0]=`executable_mise` を見て
# shim mode と誤認するため、`mise` という名前のシンボリックリンク経由で起動する
TMP_BIN_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_BIN_DIR"' EXIT INT TERM
ln -s "$MISE_BOOTSTRAP" "$TMP_BIN_DIR/mise"
MISE="$TMP_BIN_DIR/mise"

# 非対話実行でも config を読み込めるよう、ルート .mise.toml を信頼する。
# trust が無いと mise は untrusted-config エラーで停止するか config をスキップし、
# tools（chezmoi 含む）が install されない。明示パスで trust して path 解決を確実にする。
"$MISE" trust "$(pwd)/.mise.toml"

# ルート .mise.toml の tools を install（chezmoi 含む）
"$MISE" install

# mise が用意した chezmoi を呼び出して初回 apply
"$MISE" exec -- chezmoi init --apply --force tak848

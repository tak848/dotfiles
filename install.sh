#!/bin/sh
set -e

# clone 後に手動で実行する初回セットアップ用スクリプト。README の one-liner と等価。
# chezmoi が既にあればそれを再利用し、なければ get.chezmoi.io から取得して bootstrap する。
# どちらの経路でも chezmoi は本リポジトリをあらためて source として clone するため、
# 結果はこのワーキングディレクトリの状態には依存しない。

if command -v chezmoi >/dev/null 2>&1; then
  exec chezmoi init --apply tak848
fi

exec sh -c "$(curl -fsLS get.chezmoi.io)" -- init --apply tak848

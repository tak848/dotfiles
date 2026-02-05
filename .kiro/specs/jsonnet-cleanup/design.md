# Technical Design: jsonnet-cleanup

## Overview

Jsonnet から生成される JSON ファイルの冗長な Git 管理を解消し、chezmoi の `run_onchange_after_*` スクリプトを活用して `chezmoi apply` 時に JSON を直接ホームディレクトリに生成する仕組みへ移行する。

## 移行後のアーキテクチャ

```
Jsonnet ソース (Git管理)
        │
        ▼ chezmoi apply (run_onchange_after_* スクリプト)
JSON ファイル (直接生成: ~/.claude/*.json, ~/.gemini/*.json)
```

**ポイント**: JSON ファイルは chezmoi の `dot_` 管理対象外。スクリプトが直接ホームディレクトリに書き込む。

## 変更対象ファイル

### 1. 新規作成: run_onchange スクリプト

| ファイル | 役割 |
|---------|------|
| `run_onchange_after_generate-jsonnet.sh.tmpl` | jsonnet 変更時に JSON を生成 |

**スクリプト例** (mise-install.sh.tmpl と同様のパターン):
```bash
#!/bin/bash
# {{ include "dot_claude/settings.jsonnet" | sha256sum }}
# {{ include "dot_claude/dot_mcp.jsonnet" | sha256sum }}
# {{ include "dot_gemini/settings.jsonnet" | sha256sum }}
# jsonnet ファイルが変更された時のみ JSON を生成する

{{ template "path-setup.tmpl" . }}

if command -v jsonnet &> /dev/null; then
    echo "Jsonnet から JSON を生成中..."
    jsonnet "{{ .chezmoi.sourceDir }}/dot_claude/settings.jsonnet" > ~/.claude/settings.json
    jsonnet "{{ .chezmoi.sourceDir }}/dot_claude/dot_mcp.jsonnet" > ~/.claude/.mcp.json
    jsonnet "{{ .chezmoi.sourceDir }}/dot_gemini/settings.jsonnet" > ~/.gemini/settings.json
    echo "JSON 生成完了"
fi
```

### 2. 削除: dot_* の JSON ファイル

chezmoi 管理から除外（Git から削除、`.gitignore` に追加）:
- `dot_claude/settings.json`
- `dot_claude/dot_mcp.json`
- `dot_gemini/settings.json`

### 3. 更新: .gitignore

```gitignore
# Jsonnet から生成される JSON（chezmoi apply 時に直接生成）
dot_claude/settings.json
dot_claude/dot_mcp.json
dot_gemini/settings.json
```

### 4. 更新: Taskfile.yaml

- `generate-claude-settings` タスク削除
- `generate` タスクは形だけ残す（no-op、`default` との依存関係維持）
- `check` から JSON diff チェックを除外

**generate タスクの更新後**:
```yaml
generate:
  desc: (No-op) JSON generation moved to chezmoi
  cmds:
    - echo "JSON generation is now handled by chezmoi apply"
```

**default タスクは変更なし** (`generate` への依存を維持)

### 5. 更新: .github/workflows/ci.yaml

Jsonnet 構文チェックステップを追加:
```yaml
- name: Check Jsonnet syntax
  run: |
    for f in dot_claude/*.jsonnet dot_gemini/*.jsonnet; do
      jsonnet --check "$f" || exit 1
    done
```

## 実装手順

### Phase 1: スクリプト追加
1. `run_onchange_after_generate-jsonnet.sh.tmpl` を作成
2. `chezmoi apply` で動作確認

### Phase 2: Git 管理変更
1. `.gitignore` に JSON ファイルを追加
2. `git rm --cached` で JSON を Git インデックスから削除
3. `dot_claude/*.json`, `dot_gemini/*.json` を削除

### Phase 3: Taskfile 更新
1. `generate-claude-settings` タスク削除
2. `generate` タスクを no-op に変更（形だけ残す、`default` との依存関係維持）
3. `check` タスクから JSON を除外

### Phase 4: CI 更新
1. Jsonnet 構文チェックステップを追加

## 検証方法

1. `chezmoi apply` で JSON が `~/.claude/`, `~/.gemini/` に生成されることを確認
2. jsonnet ソース変更後、再度 `chezmoi apply` でスクリプトが再実行されることを確認
3. `git status` で JSON が untracked 表示されないことを確認
4. CI で Jsonnet 構文チェックが動作することを確認

## ロールバック計画

- `run_onchange_after_generate-jsonnet.sh.tmpl` を削除
- `.gitignore` から JSON を除外
- Taskfile の `generate` タスクを復元
- JSON ファイルを `task generate` で再生成してコミット

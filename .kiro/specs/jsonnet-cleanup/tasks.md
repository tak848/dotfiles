# Implementation Plan

## Tasks

- [x] 1. chezmoi スクリプト作成
  - `run_onchange_after_generate-jsonnet.sh.tmpl` を作成し、Jsonnet ソース変更時に JSON を直接ホームディレクトリへ生成する
  - 各 Jsonnet ファイルの sha256sum をコメントに含め、変更検知を有効化
  - `chezmoi apply` で `~/.claude/settings.json`, `~/.claude/.mcp.json`, `~/.gemini/settings.json` が正しく生成されることを確認
  - 生成結果が現在の `task generate` 出力と一致することを検証
  - _Requirements: 2.1, 2.2, 2.3, 2.4_

- [ ] 2. Git 管理変更
  - `.gitignore` に `dot_claude/settings.json`, `dot_claude/dot_mcp.json`, `dot_gemini/settings.json` を追加
  - `git rm --cached` で JSON ファイルを Git インデックスから削除
  - `dot_claude/*.json`, `dot_gemini/*.json` のファイル自体を削除
  - `git status` で JSON ファイルが表示されないことを確認
  - _Requirements: 1.1, 1.2, 1.3_

- [ ] 3. (P) Taskfile 更新
  - `generate-claude-settings` タスクを削除
  - `generate` タスクを no-op に変更（`default` との依存関係は維持）
  - `check` タスクから JSON diff チェックを除外
  - _Requirements: 3.1, 3.2, 3.3_

- [ ] 4. (P) CI 更新
  - Jsonnet 構文チェックステップを追加（`jsonnet --check` で各ファイルを検証）
  - `task check` が JSON ファイルの存在を前提としないことを確認
  - 構文エラー時にビルドが失敗することを検証
  - _Requirements: 4.1, 4.2, 4.3_

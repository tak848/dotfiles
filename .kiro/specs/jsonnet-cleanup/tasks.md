# Implementation Plan

## Task 1: chezmoi テンプレートファイルの作成

- [ ] 1.1 (P) Claude settings.json テンプレートの作成
  - chezmoi の output 関数を使用して jsonnet CLI を呼び出すテンプレートを作成
  - joinPath で chezmoi ソースディレクトリからの相対パスを解決
  - 末尾の改行を制御するためにハイフン付きトリミングを使用
  - _Requirements: 2.1, 2.4_

- [ ] 1.2 (P) Claude .mcp.json テンプレートの作成
  - dot_mcp.json.tmpl として作成し、展開時に .mcp.json となるようにする
  - settings.json.tmpl と同様の output 関数パターンを使用
  - _Requirements: 2.2, 2.4_

- [ ] 1.3 (P) Gemini settings.json テンプレートの作成
  - dot_gemini ディレクトリ配下にテンプレートを作成
  - Claude 用テンプレートと同じパターンで jsonnet を実行
  - _Requirements: 2.3, 2.4_

## Task 2: 動作確認と同一性検証

- [ ] 2.1 chezmoi apply による生成結果の検証
  - chezmoi apply --dry-run でテンプレート展開結果を確認
  - 現行の task generate で生成された JSON との差分比較
  - jsonnet CLI がインストールされていることを前提条件として確認
  - 3つのファイル全てで同一の出力が得られることを検証
  - _Requirements: 2.1, 2.2, 2.3, 2.4_

## Task 3: Git 管理からの JSON ファイル除外

- [ ] 3.1 .gitignore への除外パターン追加
  - dot_claude/settings.json を除外パターンに追加
  - dot_claude/dot_mcp.json を除外パターンに追加
  - dot_gemini/settings.json を除外パターンに追加
  - _Requirements: 1.1, 1.2, 1.3_

- [ ] 3.2 Git 追跡からの JSON ファイル削除
  - git rm --cached で JSON ファイルを Git インデックスから削除
  - ワーキングディレクトリのファイルは保持
  - git status で untracked 表示がされないことを確認
  - _Requirements: 1.2, 1.3_

## Task 4: Taskfile の更新

- [ ] 4.1 generate タスクと関連タスクの削除
  - generate-claude-settings タスクを削除
  - generate タスク自体を削除（他の Jsonnet 処理がないため）
  - default タスクから generate 依存を削除
  - _Requirements: 3.1, 3.2_

- [ ] 4.2 check タスクの更新
  - chezmoi 管理に移行した JSON ファイルの diff チェックを除外
  - 残存する lockfile 系タスクは維持
  - _Requirements: 3.3_

## Task 5: CI ワークフローの更新

- [ ] 5.1 Jsonnet 構文チェックの追加
  - jsonnetfmt --test による構文検証ステップを追加
  - dot_claude/*.jsonnet と dot_gemini/*.jsonnet を対象とする
  - 構文エラー時に CI を失敗させる
  - _Requirements: 4.2, 4.3_

- [ ] 5.2 task check の変更への追従
  - 生成 JSON の存在を前提としないよう確認
  - Taskfile の変更が CI で正常に動作することを検証
  - _Requirements: 4.1_

## Task 6: ステアリング文書の更新

- [ ] 6.1 structure.md の更新
  - AI ツール設定セクションの記述を更新
  - 旧記述「Task ランナーで自動変換: task generate」を削除
  - 新記述「chezmoi テンプレート（settings.json.tmpl）による動的生成」に変更
  - _Requirements: 2.1, 2.2, 2.3_


# user による指示

プロジェクト横断のユーザー指示です。

## 言語

Think in English, interact with the user in Japanese.

## MCP の使用（必須）

### 検索・情報取得

**重要**: 以下の場合、必ず MCP で裏付けを取ること。憶測や古い知識での回答は禁止。

- ライブラリ・フレームワークのバージョン差異
- 設定方法や API の使い方
- 新機能や最新のベストプラクティス
- 知識のカットオフ以降の情報

**使用する MCP:**

1. **context7** (`mcp__context7__resolve-library-id`, `mcp__context7__query-docs`)
   - **第一選択として必ず使用すること**
   - ライブラリのドキュメント検索に最適
   - 特定バージョンが必要な場合は `resolve-library-id` でバージョン付きIDを取得

2. **deepwiki** (`mcp__deepwiki__ask_question`, `mcp__deepwiki__read_wiki_contents`)
   - context7 で見つからない場合、または自然言語での解説が必要な場合に使用
   - GitHub リポジトリの構造理解に有効

**禁止事項:**
- MCP で確認せずに「〜だと思います」「おそらく〜」と回答すること
- 古いバージョンの情報を最新と偽ること

### GitHub 操作

- `gh` コマンドの直接利用ではなく、**GitHub MCP (`mcp__github__*`) を優先して使用すること**
- PR のコメント確認時は、**issue comment (`get_comments`) と review comment (`get_review_comments`) の両方を確認すること**。review comment は `get_comments` では取得できない
- GitHub MCP の `body` パラメータに改行を含める際、リテラル `\n` ではなく実際の改行文字を使うこと（リテラル `\n` はエスケープされて壊れる）
- レビューコメントの指摘に対して修正を行った場合は、必ず該当コメントに reply すること。修正した commit へのリンク（`https://github.com/{owner}/{repo}/commit/{sha}` 形式）を含めること
- issue/PR にコメント・返信する際は、本文末尾に `(by Claude Code)` を付与すること
- **PR 作成時は、リポジトリ内の PULL REQUEST テンプレートを探索し（ルート、`.github/`、`docs/`、各 `PULL_REQUEST_TEMPLATE/` サブディレクトリ）、必ず従うこと。テンプレートを無視した PR は禁止**
- **PR 作成完了後は、必ず full URL（`https://github.com/{owner}/{repo}/pull/{number}` 形式）を提示すること**

### 自分の知識を過信しない

あなたが知らない記法・構文が実際には正しく動作している場合がある。リポジトリ固有のルールや規約が存在する場合がある。API やオプションが非推奨になっていたり、新しく追加されていたりする場合がある。

- 自分の知識だけで「間違っている」「存在しない」「非推奨」と断定しないこと
- 確信が持てない場合は context7・deepwiki 等の MCP・web search・ドキュメント直接参照等で一次ソースの裏付けを取ること
- 裏付けが取れなかった場合は「未確認」と明記すること

## 各種一時的ファイルの出力ディレクトリ

一時的なファイルの出力先として `z` ディレクトリが使える（`.config/git/ignore` により ignore 済み）。
ファイル名に日時を含めると整理しやすい（例: `YYYYMMDDhhmm-hoge.md`）。日時は `date` コマンドで取得する。

## ファイル編集時の注意事項

ファイルを編集する際は、必ず最終行に空行を入れてください。
これにより、Git での差分表示が見やすくなり、POSIX 準拠のテキストファイルとなります。

- **AI やツールが自動挿入したコメント・バッジ（Devin review badge, Greptile コメント等）は絶対に削除しないこと。PR description を編集する際は、必ず現在の description を直接取得して確認すること**

## Python 実行ポリシー

- `python` / `python3` の直接実行は禁止。代わりに `uv run` を使用すること
- `uv run` は都度許可が必要なため、許可済みのツール（`awk`, `jq`, シェルスクリプト等）で実現できる場合はそちらを優先する

## Agent / Team 使用時のモデル制約

- `haiku` は使用禁止
- 基本は model 無指定（opus がそのまま使われる）。model パラメータは基本的に指定しないこと
- 本当に軽微なタスクに限り `sonnet` を指定してもよい

## ユーザー向け出力を tool 呼び出しと同一メッセージに書かない

Fable 5 には、`thinking → text → tool_use` の並びで出した text がユーザーに表示されない既知バグがある（anthropics/claude-code #74558 / #74176）。本文は復元不能で、モデル自身は「出力した」と誤認する。

- ユーザーに伝える内容（plan の変更点・却下への応答・質問の背景・判断理由・status update）は、tool 呼び出しを含まないメッセージで text として出し切る。その後、**同一ターン内で続けて** `AskUserQuestion` / `ExitPlanMode` 等を呼ぶ
- text を出した後に「表示を確認できたら次のターンで〜します」等とユーザーの確認を待ってターンを終えるのは禁止。text 出力とツール呼び出しはメッセージを分けるだけで、間に確認を挟まず一気に完遂する
- ExitPlanMode が却下されたら、何をどう変えたかを text で説明してから plan を再提示する。無言の再提示は禁止
- 「表示されたはず」を前提にしない。ユーザーが直前の出力に言及せず噛み合わないときは、飲み込まれた可能性を疑い本文を出し直す

## Git 許可設定

- 自動許可: `git switch`, `git restore`, `git commit`（amend除く）
- 禁止: `--amend`, `--no-gpg-sign`, `reset --hard`

## メモリ（auto-memory）を使わない

Claude Code の auto-memory（`~/.claude/projects/<project>/memory/`）は設定で無効化済み。使わない。学び・規約・コンテキストは memory に溜めず、内容に応じて置き場所を振り分ける:

- **プロジェクトに還元すべき規約・知見** → 各 Agent が追える位置に配置し、プロジェクトで git 管理する。Claude はそのプロジェクトの `CLAUDE.md`（リポジトリ直下／該当サブディレクトリ）、Codex はそのプロジェクトの `AGENTS.md`。
- **複数人で共有する意味のないもの**（タスクの進め方・細かい運用ルール・個人の嗜好寄りの話）→ `CLAUDE.local.md` に追記する。git worktree の場合は worktree root の `CLAUDE.local.md` に書く。`CLAUDE.local.md` へ書く前に必ずユーザーに確認する。

## 散文に不要な改行を入れない

PR 本文・plan・issue/PR コメント・ドキュメント等の散文で、見栄え目的の改行（hard wrap）を入れない。改行は段落の区切り・リスト・コードブロックなど意味のある区切りにだけ使う。1文の途中で折り返さない。

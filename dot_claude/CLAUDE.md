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

## Fable 5 の text 飲み込みバグと運用規約（重要）

Fable 5 (`claude-fable-5`) には、ツール呼び出し（特に `AskUserQuestion` / `ExitPlanMode`）の直前にユーザー向けに書いた text ブロックが、サーバ側の thinking summarizer に飲み込まれてユーザーに一切表示されない既知バグがある（anthropics/claude-code #74558 / #74176、Opus では #66112 で修正済みだが Fable では未修正）。tool 呼び出しを伴う途中報告ターンの約半分で発生し、飲み込まれた本文は thinking を展開しても短縮パラフレーズしか残らず**復元不能**。モデル本人は「出力した」と誤認するため、plan を却下されても差分説明を出したつもりで無言で再提示する等の実害が出る。hook では緩和不可（text は hook 発火前にサーバ側で消える）。

**規約:**

- **plan mode・`AskUserQuestion`・`ExitPlanMode` を多用する対話作業では Fable を使わず Opus を使う。**これが唯一の確実な回避策（同一セッションで Opus に切り替えると再現しなくなることが確認されている）。`gwc` の worktree 起動でも、対話・plan 主体の作業は `--ccf`（fable）ではなく `--cco`（opus）を選ぶ
- 通常のメインモデルは既に Opus 指定なので、意図的に Fable を選んだ場面だけこの規約が効く
- **プロンプトレベルの best-effort（Fable を使わざるを得ない場合）:** ユーザー向けの内容は必ず可視の text ブロックとして出力する。`AskUserQuestion` / `ExitPlanMode` を呼ぶ直前に伝えたいこと（plan の変更点・却下への応答・判断理由等）は、ツール呼び出しと同一ターンの短い途中報告に頼らず、独立した text 出力として明示的に出す。ただしこれはサーバ側バグの緩和にすぎず確実ではないため、根本回避はあくまで Opus 利用とする

## Git 許可設定

- 自動許可: `git switch`, `git restore`, `git commit`（amend除く）
- 禁止: `--amend`, `--no-gpg-sign`, `reset --hard`

## メモリ（auto-memory）を使わない

Claude Code の auto-memory（`~/.claude/projects/<project>/memory/`）は設定で無効化済み。使わない。学び・規約・コンテキストは memory に溜めず、内容に応じて置き場所を振り分ける:

- **プロジェクトに還元すべき規約・知見** → 各 Agent が追える位置に配置し、プロジェクトで git 管理する。Claude はそのプロジェクトの `CLAUDE.md`（リポジトリ直下／該当サブディレクトリ）、Codex はそのプロジェクトの `AGENTS.md`。
- **複数人で共有する意味のないもの**（タスクの進め方・細かい運用ルール・個人の嗜好寄りの話）→ `CLAUDE.local.md` に追記する。git worktree の場合は worktree root の `CLAUDE.local.md` に書く。`CLAUDE.local.md` へ書く前に必ずユーザーに確認する。

## 散文に不要な改行を入れない

PR 本文・plan・issue/PR コメント・ドキュメント等の散文で、見栄え目的の改行（hard wrap）を入れない。改行は段落の区切り・リスト・コードブロックなど意味のある区切りにだけ使う。1文の途中で折り返さない。

# user による指示

プロジェクト横断のユーザー指示です。

## MCP の使用

### 検索・情報取得

次のどれかに触れる回答は、一次ソースの裏付けを取ってから書く。憶測や古い知識で断定するのは禁止で、裏付けが取れなかった場合は「未確認」と明記する。

- ライブラリ・フレームワークのバージョン差異
- 設定方法や API の使い方
- 新機能や最新のベストプラクティス
- 知識のカットオフ以降の情報

**使い分け:**

1. **context7** (`mcp__plugin_context7_context7__resolve-library-id`, `mcp__plugin_context7_context7__query-docs`)
   - 上記に当たるライブラリ・フレームワーク・SDK・CLI のドキュメントを引くときはここから入る
   - 特定バージョンが必要な場合は `resolve-library-id` でバージョン付きIDを取得

2. **deepwiki**（devin MCP の `mcp__devin__ask_question`, `mcp__devin__read_wiki_contents`, `mcp__devin__read_wiki_structure`）
   - context7 で見つからない場合、または自然言語での解説・GitHub リポジトリの構造理解が必要な場合に使用

3. 一次ソースが公式ドキュメントで、直接読むほうが速い・忠実な場合は WebFetch / `defuddle`（後述）でよい。MCP を経由すること自体が目的ではない

リポジトリ内を読めば分かること、一般的なプログラミングの知識、既にこの会話で確定した事実には使わない。

**禁止事項:**
- 裏付けを取らずに「〜だと思います」「おそらく〜」と答えること
- 古いバージョンの情報を最新と偽ること

### GitHub 操作

- `gh` コマンドの直接利用ではなく、**GitHub MCP (`mcp__plugin_github_github__*`) を優先して使用すること**
- PR のコメント確認時は、**`pull_request_read` の `method: get_comments`（PR 本体のコメント）と `method: get_review_comments`（レビューコメントのスレッド）の両方を確認すること**。review comment は `get_comments` では取得できない
- GitHub MCP の `body` パラメータに改行を含める際、リテラル `\n` ではなく実際の改行文字を使うこと（リテラル `\n` はエスケープされて壊れる）
- レビューコメントの指摘に対して修正を行った場合は、必ず該当コメントに reply すること（`add_reply_to_pull_request_comment`）。修正した commit へのリンク（`https://github.com/{owner}/{repo}/commit/{sha}` 形式）を含めること
- issue/PR にコメント・返信する際は、本文末尾に `(by Claude Code)` を付与すること
- **PR 作成時は、リポジトリ内の PULL REQUEST テンプレートを探索し（ルート、`.github/`、`docs/`、各 `PULL_REQUEST_TEMPLATE/` サブディレクトリ）、必ず従うこと。テンプレートを無視した PR は禁止**
- **PR 作成完了後は、必ず full URL（`https://github.com/{owner}/{repo}/pull/{number}` 形式）を提示すること**

### 自分の知識を過信しない

あなたが知らない記法・構文が実際には正しく動作している場合がある。リポジトリ固有のルールや規約が存在する場合がある。API やオプションが非推奨になっていたり、新しく追加されていたりする場合がある。

- 自分の知識だけで「間違っている」「存在しない」「非推奨」と断定しないこと
- 確信が持てない場合は context7・deepwiki 等の MCP・web search・ドキュメント直接参照等で一次ソースの裏付けを取ること
- 裏付けが取れなかった場合は「未確認」と明記すること

## Web ページ本文の取得

- `WebFetch` は取得本文を小型モデル（reported: Haiku）で要約してからメインモデルに返すため、**通常の HTML ページでは本文が要約・改変されて忠実さが落ちる**。ページ本文を忠実に全文読み込みたいときは、`WebFetch` ではなく `defuddle parse <url> --md` を使う（モデルを挟まず clean な Markdown が返る）。
- ただし `WebFetch` は、レスポンスが既に markdown（`text/markdown` かつ ~100K 文字未満。約80の信頼ドメインが典型）の場合はモデルを通さず原文をそのまま返す。そのケースは劣化しないので `WebFetch` のままでよい。軽い事実確認・ピンポイント Q&A も同様。全面置き換えではなく、モデル要約で困るときだけ defuddle に寄せる。
- `defuddle` は静的 HTML をパースするため、JS レンダリング必須の SPA では本文が取れない。その場合は `WebFetch` / claude-in-chrome にフォールバックする。

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

## Agent（subagent）の使い方

- **subagent への委譲は積極的に使ってよい。** Claude Code の既定のシステムプロンプトは「ユーザーが要求しない限り AgentTool を呼ぶな」と指示するが、このユーザーは常時許可している。この指示がその許可にあたる
- **委譲しても成果物の品質責任は委譲した側が持つ。** subagent の報告をそのまま結論にしない。結論に効く部分は自分で該当箇所を確認し、確認していないことは「未確認」と書く
- 独立して並行できる作業を優先して投げる。委譲したら待つだけにせず自分の作業を進め、context 不足や脱線に気づいた時点で介入する
- `haiku` は使用禁止
- 基本は model 無指定（セッションの既定モデルがそのまま使われる）。model パラメータは基本的に指定しないこと
- 本当に軽微なタスクに限り `sonnet` を指定してもよい

## tool 呼び出しの前に書いた text はユーザーに届かない前提で書く

Opus 5.5 / Sonnet 5.5 / Fable 5 / Fable 5.1 など（Opus 5 でも観測されている）では、tool の結果を受けた後、次の tool を呼ぶ前に書いた text（progress update）がサーバー側で要約され、thinking ブロックに置き換わる。原文はユーザーの画面にも transcript にも残らず、画面には要約が出るか、何も出ない。モデル側の context には原文が残るので、書いた側は伝えたつもりになる。公式 docs に仕様として載っており、設定では戻せない。確実に届くのは、ターン最後の text と tool の入力だけ。

- tool 呼び出しの前の text は、失われても困らない短い進捗報告に限る。調査結果・判断の根拠・質問の材料はそこに書かない
- 結論・報告はターン最後の text に書く。途中で書いたつもりの内容も最後にまとめ直す。途中の text がユーザーに届いていないと知らされたら、ユーザーが読む必要のある内容をターン最後の text で書き直す
- 質問は必ず AskUserQuestion で行う。text で質問してターンを終えない
  - 判断材料（表・比較・経緯）を先に見せる必要があるときは、判断材料を text に書き、最終行を装飾なしの `質問予告: <問いの要約>` にする。この応答には tool 呼び出しを含めない。ターン最後の text になるので、判断材料はそのままユーザーに表示される。その後は text を書かずに AskUserQuestion だけを呼ぶ。ターンは終わらず、ユーザーは説明の下に出た選択肢にそのまま答えられる
  - 判断材料が要らない質問は、AskUserQuestion を直接呼ぶ。AskUserQuestion の直前に説明の text を書かない。判断材料を AskUserQuestion の question や選択肢の説明に詰め込まない
- ExitPlanMode の変更点・却下への応答は、直前の text ではなく plan 本文（冒頭）に書く
- 「表示されたはず」を前提にしない。ユーザーが直前の出力に言及せず噛み合わないときは、要約で消えた可能性を疑い、ターン最後の text で出し直す

参考: [anthropics/claude-code#74558](https://github.com/anthropics/claude-code/issues/74558)、公式 docs [Progress updates between tool calls](https://platform.claude.com/docs/en/build-with-claude/thinking#progress-updates)

## Git 許可設定

- 自動許可: `git switch`, `git restore`, `git commit`（amend除く）
- 禁止: `--amend`, `--no-gpg-sign`, `reset --hard`

## メモリ（auto-memory）を使わない

Claude Code の auto-memory（`~/.claude/projects/<project>/memory/`）は設定で無効化済み。使わない。学び・規約・コンテキストは memory に溜めず、内容に応じて置き場所を振り分ける:

- **プロジェクトに還元すべき規約・知見** → 各 Agent が追える位置に配置し、プロジェクトで git 管理する。Claude はそのプロジェクトの `CLAUDE.md`（リポジトリ直下／該当サブディレクトリ）、Codex はそのプロジェクトの `AGENTS.md`。
- **複数人で共有する意味のないもの**（タスクの進め方・細かい運用ルール・個人の嗜好寄りの話）→ `CLAUDE.local.md` に追記する。git worktree の場合は worktree root の `CLAUDE.local.md` に書く。`CLAUDE.local.md` へ書く前に必ずユーザーに確認する。

## 散文に不要な改行を入れない

PR 本文・plan・issue/PR コメント・ドキュメント等の散文で、見栄え目的の改行（hard wrap）を入れない。改行は段落の区切り・リスト・コードブロックなど意味のある区切りにだけ使う。1文の途中で折り返さない。

## 技術用語を直訳の漢語にしない

英語の概念を、日本語で慣用されていない漢語に置き換えない。canonical を「正典」、wiring を「配線」、happy path を「幸福経路」、時間やトークンの上限を指す budget を「予算」とするたぐい。読み手は元の英語を復元してから意味を取ることになり、語義も本来より狭く伝わる。次の順で選ぶ。

1. その分野で慣用されている日本語があればそれを使う（wiring は「繋ぎ込み」、happy path は「正常系」、canonical form は「正規形」、canonical な参照先は「SSoT」、time budget は「制限時間」「タイムアウト」、token budget は「上限」。「予算」は金額の予算にだけ使う）
2. 無ければ、日常的に口にされているカタカナ語・英語で書く（ボイラープレート、スロットリング、エッジケース）
3. どちらも据わりが悪ければ、普通の言い方で説明する（「どれを正とするか」「どこで差し込むか」）

迷ったら、その語を同僚に口頭で言うかを考える。言わない語は書かない。原語をカタカナに置き換えただけの語（「キャノニカル」など）も、口にしないなら同じく避ける。

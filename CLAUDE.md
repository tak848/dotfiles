# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Chezmoi で管理している個人用 dotfiles リポジトリ。macOS/Linux 対応。

**重要: このリポジトリの remote main が single source of truth。** ターゲットマシンでは `chezmoi update` でリモートから pull & apply する運用のみ。ローカルの chezmoi source directory とこのリポジトリは直接リンクしていないため、このリポジトリ内で `chezmoi cat` / `chezmoi diff` / `chezmoi apply` 等を実行しても意味がない。

## Commands

```bash
# mise ツールをインストール
mise install

# aqua ツールをインストール（mise 管理外のツール用）
aqua install
```

## Taskfile（自動生成ファイル管理）

自動生成ファイル（mise.lock, aqua-checksums, mise bootstrap）を統一的に管理するための Task ランナー。
jsonnet からの JSON 生成は `chezmoi apply` 時に run_onchange スクリプトで実行される。

### 基本コマンド

```bash
# 全ての自動生成を実行（lockfiles + checksums + bootstrap）
task

# lockfiles/checksums/bootstrap のみ更新
task lock

# 個別タスク
task mise:lock      # mise lockfile のみ
task aqua:checksum  # aqua checksum のみ
task mise:bootstrap # bootstrap のみ

# CI用: 生成後に diff チェック（mise.lock, JSON 除外）
task check
```

### 設計方針

| 項目 | 説明 |
|------|------|
| `root: true` | サブディレクトリからも task コマンドを実行可能 |
| `method: checksum` | ファイル内容が変わった時のみ再実行（キャッシュ） |
| `-p PLATFORMS` | mise lock で全プラットフォームを明示的に指定 |

### mise.lock の既知の問題

mise.lock は実行環境（macOS/Linux）によって結果が異なる[既知の問題](https://github.com/jdx/mise/discussions/6942)がある。そのため：

- **CI の diff チェックからは除外**（`task check` は mise.lock を検証しない）
- **mise.lock の更新は GitHub Actions ワークフロー（`lockfiles-and-checksums.yaml`）に任せる**

## Architecture

### ツール管理の構成

```
Homebrew
  └─ 基本パッケージ

~/.local/bin/mise (bootstrap スクリプト)
  └─ mise (統合ツール管理)
       ├─ ランタイム管理: go, node, pnpm (core backend)
       ├─ CLI ツール管理: fzf, ripgrep, starship, etc. (aqua backend)
       ├─ 独自ホスティング配布のツール: grok (http backend)
       ├─ Python 製 CLI: snowflake-cli (pipx backend / 内部で uv tool install)
       ├─ npm グローバルパッケージ
       └─ aqua CLI (github backend)
            └─ mise lock で checksum 取得不可のツール
                 └─ aws-cli, 1password/cli, zoxide
```

- **mise** (`dot_config/mise/`, `dot_local/bin/executable_mise`): ランタイム、CLI ツール、npm パッケージの統合管理。bootstrap 方式でインストール
- **aqua** (`dot_config/aquaproj-aqua/`): mise lock で checksum が取得できないツールを管理。aqua CLI 自体は mise でインストール

### claudex（Claude Code を Codex のモデルで駆動する）

`dot_zsh/functions/claudex.zsh` が提供する `claudex` コマンドは、Claude Code のハーネス（ツールループ・サブエージェント・hooks・MCP）をそのままに、推論するモデルだけを GPT-5.6 Sol / GPT-6 Astra に差し替える。CLIProxyAPI（`router-for-me/CLIProxyAPI`、mise の `github:` backend で導入、バイナリ名は `cli-proxy-api`）が Anthropic Messages API 互換のプロキシとして `127.0.0.1:8317` に立ち、ChatGPT サブスクの OAuth 経由で Codex backend に転送する。設定は `dot_config/cli-proxy-api/config.yaml.tmpl`（`$XDG_CONFIG_HOME/cli-proxy-api/config.yaml`）。初回のみ `cli-proxy-api --config "$XDG_CONFIG_HOME/cli-proxy-api/config.yaml" --codex-login` が必要で、トークンとログは `$XDG_STATE_HOME/cli-proxy-api/` に置かれる。CLIProxyAPI は設定内で環境変数を展開しない（`~` のみ）ため、auth-dir は chezmoi apply 時の `XDG_STATE_HOME` をテンプレートで焼き込んでいる。`XDG_STATE_HOME` は `dot_zshenv.tmpl` で他の XDG 変数と同様に export している。

- 素の `claude` は従来通り Claude サブスク / Opus で動く。`claudex` 使用中は Anthropic にリクエストが飛ばないため Claude の quota は減らず、代わりに ChatGPT 側の quota を消費する
- プロキシはマシン単位で 1 プロセス。worktree ごとには立たず、全 worktree・全セッションが 1 つを共有する（`claudex` が未起動時のみ自動起動する）。listen ポートは config.yaml の `port` が唯一の情報源で、`claudex` はそれを読む
- Anthropic は非 Claude モデルへの gateway ルーティングを公式サポートしていない。壊れても直らない前提で使う
- Claude 側の 4 スロット（fable / opus / sonnet / haiku）はそれぞれ `ANTHROPIC_DEFAULT_*_MODEL` で Codex モデルに向ける。Codex の live カタログ上の序列（astra = GPT-6 の最上位 > sol = workhorse > terra = balanced > luna = fast/affordable）に合わせて fable を astra、opus を sol、sonnet を terra、haiku を luna に割り当てる。primary（`--model`）は素の Claude の既定が Opus であるのに合わせて sol のまま。subagent も、定義側で model を明示しているものはこのスロット経由で解決される。上書きは `CLAUDEX_MODEL` / `CLAUDEX_FABLE_MODEL` / `CLAUDEX_MID_MODEL` / `CLAUDEX_SMALL_MODEL`
- Claude Code は model ID のパターンで effort / thinking の対応を判定するため、`gpt-*` ではどちらも無効になる。`claudex` は各スロットの `ANTHROPIC_DEFAULT_*_MODEL_SUPPORTED_CAPABILITIES` で `effort,xhigh_effort,thinking,adaptive_thinking,interleaved_thinking` を宣言する。`adaptive_thinking` が無いと Claude Code は thinking を `budget_tokens` 付きで送り、CLIProxyAPI は budget から effort を逆算して `/effort` の値を無視する
- CLIProxyAPI はモデルカタログを起動時と 3 時間ごとに `router-for-me/models`（GitHub）から取り直すので、Codex に新モデルが出てもバイナリ更新を待たずに使える。以前使っていた raine/claude-code-proxy はバイナリ内の allowlist でモデルを弾く方式で、`gpt-6-astra` が使えるようになるまでリリースを待つ必要があったため乗り換えた（#807 の選定理由だった effort マッピングと thinking ブロック変換は CLIProxyAPI 側にも入った）
- 認証状態は `/v1/models` に `gpt-*` が出るかで判定する。CLIProxyAPI は認証済みプロバイダのモデルしか一覧に出さないので、空なら未認証かトークン失効。ログイン後は auth-dir の file watcher が拾うのでプロキシの再起動は要らない
- `claudexf` は Claude Code の fast mode（`fastMode: true` + `CLAUDE_CODE_SKIP_FAST_MODE_ORG_CHECK=1`）で起動し、CLIProxyAPI がリクエストの `speed: fast` を Codex の `service_tier: priority` に翻訳する。CLIProxyAPI にはモデル名サフィックスで tier を指定する仕組みが無いためこの形になっている。Claude Code の fast mode は Opus 系にしか対応しないので、`gpt-*` で実際に `speed: fast` が送られるかは未検証
- `claudex` は `CLAUDE_CODE_MAX_CONTEXT_TOKENS` で Codex モデルの実 context 長（872K）を宣言する。値の根拠は ChatGPT アカウントに配られる Codex の live カタログの `max_context_window` で、`codex debug models` で確認できる。カタログは過去に 272K ↔ 372K と揺れているため、巻き戻ったら `CLAUDEX_CONTEXT_TOKENS` で下げる
- `dot_zshenv.tmpl` で export している `CLAUDE_CODE_AUTO_COMPACT_WINDOW=750000` は、素の Claude（Opus[1m]）でも `claudex`（872K）でも model context より小さいので、そのまま compaction 閾値として効く

これは既に有効化されている `codex@openai-codex` プラグイン（Claude Code から Codex CLI に作業を委譲する）とは別物で、両立する。

### claudep（1 セッションで Claude と Codex のモデルを混ぜる）

`dot_zsh/functions/claudep.zsh` が提供する `claudep` コマンドは、Claude Code を素の Claude（サブスクの OAuth、既定モデルのまま）で起動しつつ、`gpt-*` を名乗るリクエストだけを Codex に流す。`claudex` がセッション全体のモデルを差し替えるのに対し、こちらは `model: gpt-6-astra` のような frontmatter を持つ subagent を Claude のセッションの中から呼ぶためのもので、そういう agent は `claudep`（または `claudex`）の下でしか動かない。素の `claude` では Anthropic にそのモデル名を弾かれる。

```
claude ─→ cc-model-router（127.0.0.1:8318、go/cmd/cc-model-router）
            ├─ claude-* / model 無し → api.anthropic.com（ヘッダ・本文とも素通し）
            └─ gpt-*                 → cli-proxy-api（127.0.0.1:8317）→ Codex
```

- Claude Code は `ANTHROPIC_BASE_URL` だけ設定して `ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_API_KEY` / `apiKeyHelper` を設定しなければ、gateway 越しでも claude.ai のログインを認証に使い続ける（公式ドキュメント llm-gateway "Subscriptions and gateways"）。条件は gateway が `Authorization` と `anthropic-beta`（OAuth の capability）をそのまま転送することで、router は Claude 向けの経路で Host 以外を一切触らない。`httputil.ReverseProxy` の `Director` ではなく `Rewrite` を使っているのは、`Director` が `X-Forwarded-For` を勝手に足すため
- Codex 向けの経路では `Authorization` / `x-api-key` を落としてダミーの Bearer に差し替える。Claude サブスクのトークンが CLIProxyAPI（とそのログ）に渡らないようにするため。router 自身のログも method / path / model / 転送先 / status / 所要時間しか出さず、ヘッダや本文は決して出さない
- 振り分けは本文 JSON のトップレベル `model` の前方一致（既定 `gpt-`、大文字小文字を区別しない）だけ。本文が無い・JSON でない・`model` が無いリクエスト（`HEAD /api/hello`、`GET /v1/models` 等）は Anthropic に流す。`/v1/messages/count_tokens` も同じ規則で振り分ける（CLIProxyAPI が `gpt-*` で 200 と本文長に応じた `input_tokens` を返すことは curl で確認済み。Claude Code が実際にこのパスを叩くかは観測していない）
- router・cli-proxy-api ともマシン単位で 1 プロセス、未起動時だけ `claudep` が自動起動する（`claudex` と同じ mkdir ロック）。起動済みの router は `/healthz` の `codex=` が今の CLIProxyAPI のポートと一致することを確認してから使う。バイナリ更新（`chezmoi update`）は検出できないので、その後は `pkill -x cc-model-router` で入れ替える。router のログは `$XDG_STATE_HOME/cc-model-router/router.log`。バイナリは `run_onchange_after_45-build-statusline.sh.tmpl` が `~/.claude/bin/cc-model-router` に置く
- `claudex` が付ける `ANTHROPIC_DEFAULT_*_MODEL` / `CLAUDE_CODE_SUBAGENT_MODEL` / `ENABLE_TOOL_SEARCH=false` 等は「primary が gpt-*」前提の調整なので `claudep` は付けない。付けるのは `ANTHROPIC_CUSTOM_MODEL_OPTION=gpt-6-astra`（`/model` の候補に 1 件足す）とその `_SUPPORTED_CAPABILITIES`、`CLAUDE_CODE_MAX_CONTEXT_TOKENS=872000` だけ。後者は `claude-*` を名乗る ID には `DISABLE_COMPACT` を併用しない限り効かず、認識できない ID（`gpt-*`）にだけ効く（model-config "Correct the window for a gateway or custom model ID"）ので、Claude 側の context 判定は変わらない
- 実機で確認済み: `claudep -p` の Claude 経路がサブスクの OAuth で通ること、`--model gpt-6-astra` が Codex に届くこと、primary が Claude のセッションから `model: gpt-6-astra` の subagent（`--agents` で定義）を起動すると Claude Code が frontmatter の model ID をそのまま送り、router が Codex に振り分けて応答が返ること
- subagent の effort / thinking は `ANTHROPIC_CUSTOM_MODEL_OPTION_SUPPORTED_CAPABILITIES` に関係なく送られる（Codex の代わりに立てた捕捉サーバーで観測。変数の有無にかかわらず `thinking: {type: adaptive}` と `output_config.effort` が付き、frontmatter の `effort:` がそのまま `output_config.effort` になる。無指定なら settings の `effortLevel`）。subagent は main の thinking 設定を継承する仕様（sub-agents "Choose a model"）で、model ID のパターン判定は通らないらしい。この変数が効くのは `/model gpt-6-astra` で primary を切り替えたときだけ

### gpt-review（GPT-6 Astra で動くレビュー専用 subagent）

`dot_claude/agents/gpt-review.md`。`model: gpt-6-astra` の frontmatter を持つので `claudep` / `claudex` の下でしか動かない（素の `claude` では Anthropic に弾かれる）。`codex-review` agent（Codex CLI を `codex exec` で叩くラッパー）とは別物で、こちらは Claude Code のネイティブな subagent として Read / Grep / Glob / Bash / WebFetch / WebSearch / context7 / deepwiki を自分で使ってレビューする。修正はしない。

- `codex-review` の thread_id 方式に当たるものは Claude Code の subagent resume で、修正後の再レビューは同じ agent に `SendMessage` で修正内容を送る（会話が残るので前回指摘と突き合わせられる）。確証バイアスを避ける最終確認は、前回の指摘を渡さずに新しいインスタンスを起動する
- `effort: xhigh` を frontmatter で固定している。Codex の reasoning.effort は low / medium / high / xhigh で、`max` は無い
- `tools` は読み取り系に絞っているが Bash は含める（テスト・lint・`git diff` を根拠にさせるため）。ファイルを変える操作は本文で禁止している
- 出力形式（結論 / 重要度付きの指摘 / 問題なしと判断した点 / 参照ファイル / 未確認事項）は本文で固定し、呼び出し元が全文を読む前提
- Claude への通信は素の `claude` と同じなので Claude の quota を消費する。`gpt-*` の分だけ ChatGPT 側の quota。上書きは `CLAUDEP_GPT_MODEL` / `CLAUDEP_CONTEXT_TOKENS` / `CLAUDEP_ROUTER_PORT`

### codexp（Codex を profile 付きで起動する）

`dot_zsh/functions/codexp.zsh` が提供する `codexp` コマンドは、環境変数 `CODEX_PROFILE` を読んで `codex --profile <name>` に変換する薄いラッパー。Codex CLI の `--profile` は `$CODEX_HOME/<name>.config.toml` を base config の上にレイヤーする仕組みだが、Codex 自身には profile を選ぶ環境変数が無いため、repo ごとの切り替えをこの関数で補う。`CODEX_PROFILE` 未設定なら素の `codex` と完全に同じ挙動になる。

- `--profile` が使えるのは runtime サブコマンド（サブコマンド無し / `exec` / `review` / `resume` / `archive` / `delete` / `unarchive` / `fork` / `mcp` / `sandbox` / `debug prompt-input`）だけで、`login` や `doctor` に付けると Codex はエラーで即終了する。そのため非対応サブコマンドを denylist で除外している
- Codex は存在しない profile 名を黙って無視するため、`codexp` 側で profile ファイルの存在を確認して警告を出す
- profile 定義ファイル（`$CODEX_HOME/<name>.config.toml`）は chezmoi 管理外。`dot_codex/modify_config.toml` に残る legacy な `[profiles.*]` と同名の profile 名を指定すると Codex がエラーになるため、名前を衝突させないこと
- `gwc` の `--co` / `--ccco` は `codex` ではなく `codexp` を起動する

### snowx（Snowflake CLI を PYTHONPATH 無しで起動する）

`dot_local/bin/executable_snowx` が提供する `snowx` コマンドは、`env -u PYTHONPATH snow` を実行するだけの薄いラッパー。`snow` は起動時に `google.protobuf` を import するが、`google` は名前空間パッケージなので、`PYTHONPATH` 側に `__init__.py` を持つ通常パッケージとしての `google/` があると、Python はそこで解決を打ち切って venv 側の `google/protobuf` に到達できず `ModuleNotFoundError` で落ちる。protoc / gRPC の生成コード置き場を `.envrc` で `PYTHONPATH` に入れている repo で踏む。

- `PYTHONPATH=`（空文字）では直らない。空エントリは cwd に解決されうるので unset する必要がある
- 呼び出し元の `PYTHONPATH` は触らない。repo 側の Python 作業を壊さないため
- `claudex` / `codexp` と違って zsh 関数ではなくスクリプトなのは、非対話シェル（Claude Code / Codex のシェルツール、スクリプト、cron）から使いたいため。zsh 関数は対話シェルでしか読まれない。`~/.local/bin` は `dot_zshenv.tmpl` で PATH に入るので非対話でも通る
- `snowx` という名前は mise が提供しないので、`mise activate` が installs を PATH 先頭に足しても衝突しない。`snow` を上書きする形の shim だとこの PATH 争いに負ける
- `snow` の実体は「PATH → `mise/shims/snow` → `mise which snow`」の順で解決する。非対話シェルには mise の PATH が通っていないことがあるため
- 素の `snow` は残してあり、`PYTHONPATH` を汚していない場所ではそのまま使える

### statusline（cc-statusline / cc-subagent-statusline）

`go/cmd/cc-statusline` が画面下部の statusline を 3 行で描き、`go/cmd/cc-subagent-statusline` が subagent 実行中だけプロンプト下に出るパネルの各行を描く。両者は別設定・別コマンドで、後者を設定しなければ Claude Code 組み込みの `name · description · token count` が使われる。表示ロジックは `go/internal/render` に集約している。

入力 JSON のスキーマは [公式ドキュメント](https://code.claude.com/docs/en/statusline) にあるが、手元の環境で来るかどうかは別問題なので、実装時に確認した結果を残す。

- `rate_limits.spend_limit` は Claude apps gateway 配下でのみ生成される。個人の Max 契約では来ないので、読むだけにして表示は成り行きに任せている
- `permission_mode` は JSON に含まれない。statusline の再実行トリガーではあるが値は渡らないため、表示はできない
- `remote.session_id` は公式ドキュメントに記載が無いが、remote session では実際に来る
- `worktree.*` は Claude Code の worktree セッション機能専用で、`gwc` 運用では来ない。ワークツリー名は `workspace.git_worktree` を使う

実装で踏みやすい落とし穴が 4 つある。

- **null をポインタで受ける。** `encoding/json` は非ポインタ型に `null` を入れても no-op でゼロ値のまま通す。`context_window.current_usage` を値型で受けると、`/compact` 直後に「0k・0%・緑バー」というもっともらしい嘘が出る
- **数値は `float64` で受ける。** `resets_at` を `int64` で受けていると、上流が小数付き数値を出した瞬間に decode 全体が失敗し、statusline が丸ごと消える
- **外部由来の文字列は制御文字を落とす。** `session_name` や PR タイトルに改行が 1 個混ざると幻の行が増え、ESC が混ざると色や OSC 8 の終端を乗っ取られる。`render.Sanitize` を必ず通す
- **各行に必ず残るセグメントを置く。** 行が空になると statusline の高さが変わり、fullscreen renderer では入力欄が上下に跳ねる。`render.Segment` の `Drop` が 0 のものは幅が足りなくても落とさない

そのほかの設計上の判断。

- 時刻はすべて絶対時刻で出しているので `refreshInterval` は設定していない。`resets_at` / `expires_at` の到達自体が再実行トリガーなので、イベント駆動だけで表示が古びない。ただし経過時間だけは入力のスナップショットなので、アイドル中は止まって見える
- OSC 8 ハイパーリンクの可否は環境変数だけで決める（stdout がパイプなので isatty が使えない）。`CC_STATUSLINE_HYPERLINKS` で明示的に上書きでき、`NO_COLOR` や `TERM=dumb` では出さない
- 失敗時の振る舞いは 2 つで逆にしてある。cc-statusline は無出力だと statusline が黙って消えて壊れたことに気づけないのでエラー行を出し、cc-subagent-statusline は無出力が既定描画へのフォールバックになるので黙って終わる
- `run_onchange_after_45-build-statusline.sh.tmpl` の `include` リストから漏れたファイルは、編集しても再ビルドされず古いバイナリが残る。エラーも警告も出ないので、`go/cmd/cc-statusline/buildscript_test.go` が網羅を検証している

### 自動生成ファイル一覧

| ファイル | 生成元 | 生成方法 |
|---------|--------|---------|
| `mise.lock` | `.mise.toml` | `task mise:lock` |
| `dot_config/mise/mise.lock` | `dot_config/mise/config.toml` | `task mise:lock` |
| `dot_config/aquaproj-aqua/aqua-checksums.json` | `dot_config/aquaproj-aqua/aqua.yaml` | `task aqua:checksum` |
| `dot_local/bin/executable_mise` | `.mise-bootstrap-version` | `task mise:bootstrap` |

### chezmoi apply 時の自動管理ファイル

| ターゲットファイル | 生成元 | 方式 |
|------------------|--------|------|
| `~/.claude/settings.json` | `dot_claude/settings.jsonnet` | jsonnet 全体生成（run_onchange） |
| `~/ccgate.libsonnet` | `ccgate.libsonnet` | Claude/Codex 共通の ccgate ルール |
| `~/.gemini/antigravity-cli/settings.json` | `dot_gemini/antigravity-cli/settings.jsonnet` | jsonnet 全体生成（run_onchange） |
| `~/.gemini/antigravity-cli/mcp_config.json` | `dot_gemini/antigravity-cli/mcp_config.jsonnet` | jsonnet 全体生成（run_onchange） |
| `~/.claude.json` | `modify_dot_claude.json` | chezmoi modify テンプレート（差分適用） |
| `~/.codex/config.toml` | `dot_codex/modify_config.toml` | chezmoi modify テンプレート（差分適用） |
| `~/.codex/ccgate.jsonnet` | `dot_codex/ccgate.jsonnet` | Codex PermissionRequest 補助判定ルール |

`~/.claude.json` と `~/.codex/config.toml` はツールが自動的に書き込むため、jsonnet で全体生成せず modify テンプレートで管理対象キーのみ差分適用する。

### Claude Code カスタムマーケットプレイス

dotfiles リポジトリ自体がカスタムマーケットプレイス (`tak848-plugins`) として機能する。MCP サーバーは `claude-plugins/` 配下にプラグインとして定義。

| ファイル | 役割 |
|---------|------|
| `.claude-plugin/marketplace.json` | マーケットプレイス定義 |
| `claude-plugins/{name}/.claude-plugin/plugin.json` | プラグインメタデータ |
| `claude-plugins/{name}/.mcp.json` | MCP サーバー設定 |
| `dot_claude/settings.jsonnet` の `extraKnownMarketplaces` | マーケットプレイス登録 |
| `dot_claude/settings.jsonnet` の `enabledPlugins` | プラグイン有効化 |

### chezmoi apply 時の自動実行スクリプト

`chezmoi apply` 時にターゲットディレクトリで実行されるスクリプト。リポジトリ内にはファイルを生成しない。

| スクリプト | トリガー | 処理 |
|-----------|---------|------|
| `run_once_before_00-install-essentials.sh.tmpl` | 初回のみ | Homebrew, zinit, cursor-agent インストール |
| `run_onchange_after_10-mise-install.sh.tmpl` | `config.toml` 変更時 | `mise install` |
| `run_onchange_after_20-aqua-install.sh.tmpl` | `aqua.yaml` 変更時 | `aqua install` |
| `run_onchange_after_30-install-packages.sh.tmpl` | `packages.yaml` 変更時 | `brew install`（macOS） |
| `run_onchange_after_40-generate-jsonnet.sh.tmpl` | jsonnet ファイル変更時 | jsonnet → JSON 生成（`~/.claude/settings.json`, `~/.gemini/antigravity-cli/{settings,mcp_config}.json`） |
| `run_onchange_after_45-build-statusline.sh.tmpl` | Go ソース変更時 | `~/.claude/bin/` と `~/.codex/bin/` へ Go バイナリをビルド |
| `run_onchange_after_50-claude-plugins.sh.tmpl` | プラグイン定義変更時 | `claude plugin marketplace update` + `install` |

### Chezmoi ファイル命名規則

| プレフィックス | 展開先 | 例 |
|--------------|--------|-----|
| `dot_` | `~/.` | `dot_zshrc.tmpl` → `~/.zshrc` |
| `executable_` | 実行権限付与 | `executable_mise` → `mise` (+x) |
| `.tmpl` | テンプレート展開 | OS/アーキテクチャ分岐 |
| `modify_` | 既存ファイルを差分適用 | `modify_dot_claude.json` → `~/.claude.json` |
| `run_once_before_*` | 初回のみ実行 | Homebrew インストール |
| `run_onchange_after_*` | ファイル変更時実行 | mise install |

### 自動更新ワークフロー（Renovate + GitHub Actions）

| ワークフロー | トリガー | 処理 |
|-------------|---------|------|
| `ci.yaml` | push | `task check` で自動生成ファイルの diff チェック |
| `lockfiles-and-checksums.yaml` | push（`.mise.toml` / `dot_config/mise/config.toml` / `dot_config/aquaproj-aqua/aqua.yaml` 変更時、`main` / `lazy-lock-update` ブランチを除く） | `mise lock`（ルート + `dot_config/mise/`）→ `fill-lockfile-checksums.sh` → `aqua update-checksum --prune` を Renovate PR ブランチへ commit |

`fill-lockfile-checksums.sh` は、`mise lock` では checksum が埋まらないツールを補う。mise は backend が checksum を供給できる場合しか lockfile に書き込まないため、`http` backend のツールや aqua-registry 側に checksum 定義が無いパッケージ（`anthropics/claude-code`, `tamasfe/taplo`, `http:grok`）は url だけが残り、install 時の整合性検証が働かない。そこで実アーティファクトを取得して sha256 を計算し、lockfile に追記する。checksum のあるエントリは触らないので冪等。アーティファクトを丸ごとダウンロードする都合上、対象が増えるほど CI 時間は延びる（現状で合計 2 GB 強）。
| `mise-bootstrap.yaml` | push（`.mise-bootstrap-version` 変更時） | `mise generate bootstrap` |
| `lazy-lock.yaml` | nvim 設定変更 / 週次 cron | Lazy.nvim lockfile 更新（PR 作成 or Renovate PR へコミット） |

### zsh 設定の読み込み順序

```
~/.zshenv      # PATH, 環境変数（非インタラクティブ含む）
~/.zprofile    # mise shims（IDE 連携用）
~/.zshrc       # インタラクティブ設定、プラグイン、エイリアス
~/.zshrc.local # マシン固有設定（Git 管理外）
```

## Conventions

- コミットメッセージは日本語、`feat:`, `fix:`, `chore:` などのプレフィックス必須
- PR タイトルも日本語、`feat:`, `fix:`, `chore:` などの Conventional Commits プレフィックス必須。`[codex]` / `[claude]` のような agent 名プレフィックスは付けない
- Renovate PR への push には GitHub App Token が必要（GITHUB_TOKEN では不可）
- 自動生成ファイルは手動編集しない（`task` または Renovate ワークフローで自動更新）

### このリポジトリで作業するエージェントへの厳守事項

- **変更が一段落したら、確認を待たず commit → push → draft PR まで一気に完遂する**（commit で止めない）。PR は draft で作成する
- `git commit` の前に必ず現在のブランチを確認する（`git branch --show-current`）。plan mode から戻った後や PR マージ後は main に戻っている可能性が高い
- main に直接 commit しない。main から新しいブランチを切る。push の前にリモートブランチの状態を確認する（`git ls-remote` 等）。マージ済みブランチには push しない・不要なリモートブランチを散らかさない
- ユーザーが別 repo / 機構を参照したら、default branch だけで「無い」と判断せず `git branch -a` / `git log --all` で他ブランチも確認してから答える
- **要求スコープを厳守する。** ユーザーが「A の代替として B を作る」等と明示したら、その範囲だけに絞る。隣接レイヤー（前段・後段・類似機能）の改修を勝手に計画へ足さない。関連改善は本線の計画を立てた上で別途質問する
- **環境を直接変更しない。** 変更は必ずこの repo のソース（`dot_` プレフィックス付きファイル等）を編集し、PR 経由で行う。`~/.local/share/chezmoi` 等の repo 外パスや、`~/.claude/` `~/.codex/` `~/.config/` 等のターゲットファイルを直接書き換えてはならない。環境への適用は `chezmoi update` に委ねる（remote main が single source of truth）
- **chezmoi ソースを編集する。** `~/.claude/CLAUDE.md` 等のターゲットではなく `dot_claude/CLAUDE.md` 等のソースを編集する。ターゲットを直接編集しても `chezmoi update` で上書きされ、PR にも含められない
- **ツール導入手段として Homebrew を提案しない**（`packages.yaml` への追加・`brew install` を選択肢に挙げない）。mise（aqua / github / go / npm / pipx backend）または aqua CLI で完結させる
- Python 製 CLI でバイナリ配布が無いものは mise の `pipx` backend で入れる（uv が入っていれば mise は内部で `uv tool install` を使う）。Renovate の mise manager が PyPI datasource として追える。システム Python に引きずられないよう `install_env = { UV_PYTHON_PREFERENCE = "only-managed" }` を付ける
- mise にツールを追加する際、`mise search` / `mise registry` で見つからなくても [aqua-registry](https://github.com/aquaproj/aqua-registry/tree/main/pkgs) に定義があれば `"aqua:<registry path>" = "<version>"` で追加できる（Renovate 自動更新・checksum 検証に乗る）。`http` backend で URL を手書きするのは aqua-registry にも無い最終手段のみ
- 例外として、aqua-registry 側の定義が `type: http`（GitHub リリースではなく独自ホスティング配布）のパッケージは `aqua:` で追加しない。mise の aqua backend が lockfile 生成のたびに GitHub のタグ取得を試みて必ず失敗し警告を出すうえ、[Renovate の mise manager も aqua の http パッケージを抽出対象外にしている](https://docs.renovatebot.com/modules/manager/mise/#limitations)ため更新も効かない。この場合は mise の `http` backend で URL を直接指定する（`dot_config/mise/config.toml` の `[tools."http:grok"]` が例）
- 環境変数（API key 等）の置き場所を勝手に特定ファイル（`.zshrc.local` 等）に指定しない。置き場所はユーザーに委ねる（エラーメッセージやコメントにも特定ファイル名を書かない）
- `~/.codex/config.toml` 等の modify テンプレート（`dot_codex/modify_config.toml`）は、出力で再出力しないキーを `chezmoi apply` 時に削除する。ツールが書き込む既存キー（`projects` / `notice` / `hooks.state` 等）は保持ブロックに追加すること

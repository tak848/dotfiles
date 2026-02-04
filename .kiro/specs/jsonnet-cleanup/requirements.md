# Requirements Document

## Project Description (Input)
jsonnetとjsonどっちもcommitしなくてもchezmoiで生成すれば良いので掃除する

## Introduction

本機能は、dotfiles リポジトリにおける Jsonnet から生成される JSON ファイルの冗長なコミットを解消するためのクリーンアップです。現在、`dot_claude/` と `dot_gemini/` ディレクトリでは Jsonnet ソースファイルと生成された JSON ファイルの両方がコミットされています。chezmoi のテンプレート機能を活用することで、JSON ファイルを `chezmoi apply` 時に動的に生成し、リポジトリからは Jsonnet ソースのみを管理する形に移行します。

## Requirements

### Requirement 1: JSON 生成ファイルの Git 管理除外
**Objective:** As a dotfiles 管理者, I want Jsonnet から生成される JSON ファイルを Git 管理から除外したい, so that ソースと生成物の二重管理を排除し、リポジトリをシンプルに保てる

#### Acceptance Criteria
1. The chezmoi source shall Jsonnet ソースファイル（`settings.jsonnet`, `dot_mcp.jsonnet`）のみを Git リポジトリに保持する
2. The Git repository shall 生成された JSON ファイル（`settings.json`, `dot_mcp.json`）を追跡しない
3. When `git status` を実行した場合, the Git repository shall Jsonnet から生成された JSON ファイルを untracked または無視として表示しない

### Requirement 2: chezmoi による JSON 自動生成
**Objective:** As a dotfiles 利用者, I want `chezmoi apply` 実行時に JSON ファイルが自動生成されるようにしたい, so that Jsonnet ソースの変更が自動的にホームディレクトリに反映される

#### Acceptance Criteria
1. When `chezmoi apply` を実行した場合, the chezmoi shall `dot_claude/settings.jsonnet` から `~/.claude/settings.json` を生成する
2. When `chezmoi apply` を実行した場合, the chezmoi shall `dot_claude/dot_mcp.jsonnet` から `~/.claude/.mcp.json` を生成する
3. When `chezmoi apply` を実行した場合, the chezmoi shall `dot_gemini/settings.jsonnet` から `~/.gemini/settings.json` を生成する
4. The chezmoi shall 生成された JSON ファイルの内容が現在の `task generate` と同一の結果となる

### Requirement 3: Taskfile の整理
**Objective:** As a 開発者, I want Taskfile から不要になった JSON 生成タスクを整理したい, so that タスク定義がシンプルになり、責務が明確になる

#### Acceptance Criteria
1. When Jsonnet からの JSON 生成が chezmoi に移行された場合, the Taskfile shall `generate` タスクの対象から該当ファイルを除外する
2. If `task generate` が他の Jsonnet ファイルを処理しない場合, the Taskfile shall `generate` タスク自体を削除する
3. The Taskfile shall `task check` で chezmoi 管理に移行した JSON ファイルの diff チェックを行わない

### Requirement 4: CI ワークフローの整合性
**Objective:** As a CI 管理者, I want CI ワークフローが新しい構成に対応するようにしたい, so that JSON ファイルの生成方法変更後も CI が正常に動作する

#### Acceptance Criteria
1. When CI で `task check` を実行した場合, the CI shall chezmoi 管理に移行した JSON ファイルの存在を前提としない
2. The CI workflow shall Jsonnet ソースファイルの構文チェックを継続して実行する
3. If Jsonnet ソースに構文エラーがある場合, the CI shall エラーとして検出し、ビルドを失敗させる


# 環境構築

開発環境の前提、タスクランナー、CI、lint / format、行数管理、Git Hooks を定義する。

技術選定の背景は [アーキテクチャ設計](../architecture/overview.md)、テスト方針と CI 要件は [非機能要件](../requirements/non-functional.md#保守性テスト)、runner ホスト側（本ツールを動かす側）の前提は [ランナーホストのセットアップ](../operations/runner-host-setup.md) を参照。

## 概要

| 項目 | 選定 |
|------|------|
| 言語 | Go（バージョンは `go.mod` を単一の情報源とする） |
| 対象 OS | Linux（amd64 / arm64） |
| 成果物 | 単一バイナリ `gsr-helper` |
| タスクランナー | Makefile |
| CI | GitHub Actions（self-hosted runner） |
| Lint | golangci-lint + `go vet` |
| Format | gofmt（標準） |
| 行数管理 | Linterly |
| Git Hooks | Lefthook |
| 開発ツールの管理 | `go.mod` の tool ディレクティブ |
| Swagger / OpenAPI | 対象外（[理由](#swagger--openapi)） |

### Go バージョンを二重管理しない

Go のバージョンは `go.mod` にのみ書く。CI では `actions/setup-go` の `go-version-file: go.mod` で読み取り、ドキュメントにも具体的なバージョン番号を書かない。`go.mod` を上げれば CI も追従する。

## ディレクトリ構造

[アーキテクチャ設計 / ディレクトリ構成](../architecture/overview.md#ディレクトリ構成) を参照。パッケージごとの責務と依存の向きは [コンポーネント設計](../components/overview.md) にある。

本ドキュメントで追加する設定ファイルはリポジトリルートに置く。

```
.github/workflows/ci.yml   CI 定義
.golangci.yml              golangci-lint 設定
.linterly.yml              行数上限の設定
.linterlyignore            行数チェックの除外
lefthook.yml               Git Hooks 定義
Makefile                   タスク定義（CI / hooks / 手元で共用）
```

## 開発環境セットアップ

### 必要なもの

| ツール | 用途 | 備考 |
|--------|------|------|
| Go | ビルド・テスト・開発ツールの実行 | バージョンは `go.mod` に従う |
| make | タスク実行 | 大半の Linux ディストリビューションに同梱 |
| git | バージョン管理・Git Hooks | — |

golangci-lint / linterly / lefthook は個別にインストールしない。後述のとおり `go.mod` の tool ディレクティブで管理し、`go tool` 経由で実行する。

`systemctl` / `docker` / `journalctl` / `gh` は**開発には不要**である。これらは実行時の依存であり、無い環境では該当機能を無効化して動作する（[アーキテクチャ設計 / 起動シーケンスと能力判定](../architecture/overview.md#起動シーケンスと能力判定)）。実機に近い動作確認をする場合のみ [ランナーホストのセットアップ](../operations/runner-host-setup.md) に従って用意する。

### 手順

```bash
git clone https://github.com/ousiassllc/gsr-helper.git
cd gsr-helper

make tools     # 開発ツールを含む依存の取得
make hooks     # Git Hooks の登録
make check     # format / vet / lint / linterly / test を通して確認
```

### 開発ツールのバージョン管理

開発ツールは `go.mod` の tool ディレクティブで管理する。バージョンが `go.mod` / `go.sum` に固定されるため、CI と手元で lint 結果がずれない。

```bash
go get -tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint
go get -tool github.com/ousiassllc/linterly/cmd/linterly
go get -tool github.com/evilmartians/lefthook
```

以後の実行は `go tool <名前>` で行う（`go tool golangci-lint run` など）。Makefile の各ターゲットがこれをラップしているため、通常は `make lint` のように呼ぶ。

ツールを更新する場合は `go get -tool <パス>@<バージョン>` を実行し、`go.mod` の差分をコミットする。

## タスクランナー

lint / test / build のコマンド列を Makefile に集約し、**CI・Git Hooks・手元の 3 経路から同じターゲットを呼ぶ**。コマンドの二重管理を防ぐことが目的である。

| ターゲット | 内容 |
|-----------|------|
| `make help` | ターゲット一覧を表示（既定） |
| `make tools` | 依存モジュールと開発ツールの取得 |
| `make fmt` | `go fmt ./...` でモジュール内のパッケージを整形する |
| `make fmt-check` | 未整形のファイルがあれば失敗する（CI / hooks 用） |
| `make vet` | `go vet ./...` |
| `make lint` | `golangci-lint run` |
| `make linterly` | 行数チェック |
| `make test` | `go test ./...` |
| `make build` | `gsr-helper` をビルド |
| `make hooks` | Lefthook を Git Hooks に登録 |
| `make check` | `fmt-check` → `vet` → `lint` → `linterly` → `test` を順に実行 |

```makefile
.DEFAULT_GOAL := help

GO   ?= go
BIN  := gsr-helper
CMD  := ./cmd/gsr-helper

.PHONY: help tools fmt fmt-check vet lint linterly test build hooks check

help: ## ターゲット一覧を表示する
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

tools: ## 依存モジュールと開発ツールを取得する
	$(GO) mod download

fmt: ## gofmt で整形する
	$(GO) fmt ./...

fmt-check: ## 未整形のファイルがないか確認する
	@out=$$(gofmt -l $$($(GO) list -f '{{.Dir}}' ./...)); \
	if [ -n "$$out" ]; then \
		echo "gofmt が必要なファイル:"; echo "$$out"; exit 1; \
	fi

vet: ## go vet を実行する
	$(GO) vet ./...

lint: ## golangci-lint を実行する
	$(GO) tool golangci-lint run

linterly: ## 行数チェックを実行する
	$(GO) tool linterly check

test: ## テストを実行する
	$(GO) test ./...

build: ## バイナリをビルドする
	$(GO) build -o $(BIN) $(CMD)

hooks: ## Git Hooks を登録する
	$(GO) tool lefthook install

check: fmt-check vet lint linterly test ## すべてのチェックを実行する
```

## CI/CD

### 構成

| 項目 | 内容 |
|------|------|
| プラットフォーム | GitHub Actions |
| ランナー | self-hosted（`runs-on: [self-hosted, linux, x64]`） |
| トリガー | `main` への push、および PR |
| ジョブ | `lint` / `test` / `build` の 3 本を並列実行 |
| デプロイ | なし（配布は `go install`。[非機能要件 / 可搬性](../requirements/non-functional.md#可搬性)） |

ジョブを並列にするのは、lint が落ちてもテスト結果が同時に得られるようにするためである。ジョブ間に依存はない。

`permissions` は `contents: read` のみを与える。CI はリポジトリへの書き込みを行わない。

### ワークフロー定義

`.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read

concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true

jobs:
  lint:
    if: github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository
    runs-on: [self-hosted, linux, x64]
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
      - run: make fmt-check
      - run: make vet
      - run: make lint
      - run: make linterly

  test:
    if: github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository
    runs-on: [self-hosted, linux, x64]
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
      - run: make test

  build:
    if: github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository
    runs-on: [self-hosted, linux, x64]
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
      - run: make build
```

`actions/setup-go` はモジュールとビルドのキャッシュを既定で有効にするため、`cache` の明示指定は不要。

**初版では `build` ジョブが失敗する。** `make build` の対象である `cmd/gsr-helper` がまだ存在しないためで（実測: `go build` が exit 1、`make` が exit 2）、CI 設定の不備ではない。エントリポイントは Issue #3 の成果物であり、#3 で解消する。それまで **`build` を required status check に指定しない**。

### self-hosted runner を使う前提

CI は GitHub ホストランナーではなく self-hosted runner で実行する。本ツールが管理する対象そのものの上で CI が回るため、以下を前提とする。

| 項目 | 前提 |
|------|------|
| runner の所在 | org（`ousiassllc`）レベルに登録された runner を使う。リポジトリレベルには登録しない |
| ラベル | `self-hosted` / `linux` / `x64` の 3 つを AND で要求する。1 つでも欠けるとジョブはエラーにならず無期限に `queued` で止まる（[ランナーホストのセットアップ](../operations/runner-host-setup.md#ラベル)） |
| OS | Linux（amd64）。対象 OS と一致するため、GitHub ホストランナーでは検証できない `systemctl` / `journalctl` 前提の挙動もそのまま確認できる |
| ワークスペース | ジョブ間で作業ディレクトリが再利用される。`actions/checkout` の既定（`clean: true`）に依存し、ビルド成果物を残す前提のステップを書かない |
| ツールの導入 | Go は `actions/setup-go` がツールキャッシュへ導入する。ホストに Go を事前インストールしない（バージョンの二重管理を避ける） |
| 並列実行 | `lint` / `test` / `build` の 3 ジョブが同時に走るため、runner は 3 台以上を稼働させる。台数が足りない場合はジョブが順番待ちになるだけで失敗はしない |
| 権限 | CI ジョブは runner の実行ユーザー権限で動く。`sudo` を必要とするテストを CI に置かない（`Executor` のテスト実装で代替する。[非機能要件 / 保守性・テスト](../requirements/non-functional.md#保守性テスト)） |

#### fork からの PR で self-hosted ジョブを起動しない

self-hosted runner でワークフローを実行することは、**そのワークフローに runner の実行ユーザー権限を与える**ことを意味する。[セキュリティ設計](../architecture/security.md#パスワード不要-sudo-の要求への対応)のとおり `NOPASSWD: ALL` の付与は「その runner で実行される任意のワークフローに実質 root を与える」ことに等しい。本リポジトリは public であり、fork からの PR は第三者が書いた任意のコードを含むため、そのまま self-hosted runner で走らせるとホストが第三者の実行環境になる。

**防御は多層で構成する。ワークフロー側の `if` 条件はその 1 層にすぎず、単体では境界にならない。**

| 層 | 手段 | 位置づけ |
|----|------|---------|
| 一次防御 | fork PR の承認ポリシー（リポジトリ設定） | 悪意ある第三者に対する実質的な境界 |
| 一次防御 | org runner group の対象リポジトリ限定 | 同じ runner を掴めるリポジトリを絞る |
| 補助 | ワークフローの `if` 条件 | 事故防止と runner 負荷削減。善意の fork PR による誤起動を止める |

ワークフロー側には、全ジョブに次の条件を付ける。

```yaml
if: github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository
```

- `push`（`main`）では常に実行する
- PR では **head が同一リポジトリのブランチである場合のみ**実行する。fork からの PR ではジョブが `skipped` になる

**この `if` はセキュリティ境界にはならない。** `pull_request` イベントでは、ワークフロー定義自体がマージコミット側（= PR の内容を含む側）から取られる（GitHub のドキュメントは `pull_request_target` を「`pull_request` イベントのようにマージコミットのコンテキストではなく、ベースリポジトリの既定ブランチのコンテキストで実行される」と対比して説明している）。つまり **fork 側で `.github/workflows/ci.yml` の `if:` 行を削除でき、その改変版が `pull_request` の実行に使われる。**したがって `if` は事故防止・runner 負荷削減の防御層として有効だが、悪意ある第三者を止めるのはリポジトリ設定側である。

一次防御の前提と限界:

- **fork PR の承認ポリシー**（実測: 現在 `approval_policy: first_time_contributors`）。GitHub 自身が、self-hosted runner を使っている場合は「設定した承認ポリシーで承認をバイパスできるユーザーの悪意あるワークフローコードは自動実行される」と警告している。さらにこのポリシーでは**リポジトリにコミットまたは PR がマージされたことのあるユーザーは以後承認不要**になるため、悪意あるユーザーは些細な typo 修正をメンテナに受け入れさせるだけでこの要件を満たせる。**承認ポリシーを「全外部貢献者に承認必須」へ変更することを推奨する**（リポジトリ設定の変更自体は別 Issue で行う）。
- **org runner group** の対象リポジトリを本リポジトリに限定し、他リポジトリのワークフローが同じ runner を掴めないようにする。

**`skipped` は required status check に対して success として扱われる。** GitHub のドキュメントは「スキップされたジョブはステータスを Success として報告する。required check であっても PR のマージを妨げない」「required status check は保護ブランチへ変更を加える前に `successful` / `skipped` / `neutral` のいずれかである必要がある」と明記している。したがって将来 `lint` / `test` / `build` を required status check に指定すると、**fork PR は CI が一度も走っていないのに 3 つとも緑になり、マージ可能に見える**（実測: 現在は branch protection もルールセットも未設定なため潜在的な問題に留まる）。fork PR のマージを機械的に止めたいなら、`if` で skip するのではなく、**常時実行される別のゲートジョブ**（GitHub ホストランナー上で動き、fork PR のときに失敗する軽量ジョブ）を required status check に指定する設計が必要である。

fork からの PR を検証する場合は、内容を確認した上で同一リポジトリ内のブランチへ取り込み、そのブランチの PR で CI を通す。**この手順は人的運用に依存する**（前段のとおり fork PR 側の check は緑に見えるため、機械的には止まらない）。`pull_request_target` は使わない（fork の PR に対してベース側の権限でワークフローが動くため、この対策の意味が失われる）。

**トリガーを追加する際はガード条件を必ず見直す。** 現在の式は「`pull_request` でなければ無条件に実行する」という**ブロックリスト形**である。`merge_group` や `workflow_dispatch` を `on:` に足すと、左辺 `github.event_name != 'pull_request'` が true になって短絡し、**fork チェックが評価されないまま実行される**。逆に merge queue を導入する際に `merge_group` を `on:` に足さないと、required status check が報告されずキューが詰まる（GitHub のドキュメントに明記がある）。将来的には、新しいトリガーを足したときに既定が「実行しない」側へ倒れる**許可リスト形**への移行が候補である。

```yaml
if: github.event_name == 'push' || (github.event_name == 'pull_request' && github.event.pull_request.head.repo.full_name == github.repository)
```

### 将来の拡張候補

初版では入れないが、必要になった時点で追加する。

- **クロスコンパイル検証** — `GOARCH=arm64` でのビルドを CI で確認する。可搬性要件で arm64 を掲げているため、arm64 環境で問題が出た場合に導入する
- **カバレッジ計測** — `go test -coverprofile`。閾値の運用ルールを決めてから入れる
- **GoReleaser** — バイナリ配布を始める場合に検討する

## Lint

### golangci-lint

`.golangci.yml`:

```yaml
version: "2"

linters:
  default: standard
  enable:
    - errorlint
    - gocritic
    - gosec
    - revive
  exclusions:
    rules:
      # テストではエラーの取り扱いとパス操作を緩める
      - path: _test\.go
        linters:
          - errcheck
          - gosec

formatters:
  enable:
    - gofmt
```

`default: standard` で有効になる標準セット（`errcheck` / `govet` / `ineffassign` / `staticcheck` / `unused`）に、以下を追加する。

| リンター | 追加する理由 |
|---------|------------|
| `gosec` | **root 権限で動作するツールであり、パス操作と外部コマンド実行の誤りが致命的になる。**削除処理のパス検証（[セキュリティ設計](../architecture/security.md)）を機械的に補強する |
| `errorlint` | エラーの比較・ラップの誤り（`==` による比較、`%v` でのラップ）を検出する。部分的な失敗を集約して返す設計上、エラーの取り扱いミスが表面化しにくい |
| `revive` | 命名と可読性の検査。「clear naming, small functions」の方針を機械的に支える |
| `gocritic` | 冗長な記述・非効率な記述の検出。パーサとマージ処理が中心のコードベースで効きやすい |

`formatters` に `gofmt` を入れることで、`golangci-lint run` でも整形漏れを検出できる。`make fmt-check` と役割が重なるが、どちらか一方だけを実行しても検出できる状態にしておく。

### 抑制の方針

- **抑制は行単位で行い、必ず理由を書く。** `//nolint:gosec // 引数は runner ディレクトリ配下であることを検証済み` のように、なぜ安全かを書く。ファイル単位・パッケージ単位の抑制は使わない。
- **`gosec` の G204（可変引数での外部コマンド実行）は `internal/exec` に集中する。** 外部プロセス実行は Executor 1 本に集約する設計（[アーキテクチャ設計](../architecture/overview.md#外部コマンドの実行)）のため、抑制箇所も 1 箇所に収まる。ドメイン層に G204 の抑制が現れた場合は、**抑制ではなく設計違反**として `exec` 層経由に直す。
- **現時点で `internal/runner/systemd.go` の `systemctl show` 呼び出しに G204 の抑制が 1 箇所ある（前項の方針に反する暫定措置）。** 集約先の `internal/exec`（Issue #3 の成果物）が未実装で、`Executor` 経由に書き換える先がまだ無いためである。#3 で `Executor` 経由に置き換え、この抑制を除去する。
- 抑制が増えてきた場合は `.golangci.yml` の `exclusions` にルールとして書き、経緯をこのドキュメントに残す。

### go vet

`golangci-lint` にも `govet` が含まれるが、CI では `make vet` を別ステップとして残す。golangci-lint の設定ミスや導入失敗時にも標準ツールのチェックが動くようにするためである。

## Format

**gofmt のみを使う。** Go 標準ツールチェーンで完結し、追加の依存もエディタ設定の追従も不要である。

| 用途 | コマンド |
|------|---------|
| 整形する | `make fmt` |
| 整形漏れを検出する | `make fmt-check` |

`gofmt -l` は未整形ファイルを列挙するだけで終了コードが 0 のままなので、`fmt-check` では出力が空であることを検証している。

**`gofmt` に `.` を渡さず、`go list -f '{{.Dir}}' ./...` で解決したパッケージディレクトリだけを対象にする。** `gofmt` はパッケージ単位ではなくファイルシステムを再帰するため、`.` を渡すと作業用に切った入れ子の git worktree（別ブランチのチェックアウト）配下の `.go` ファイルまで拾い、`make fmt` が無関係なブランチのファイルを書き換えたり、`make fmt-check` が無関係な未整形ファイルで失敗したりする。`go fmt` / `go list` はパッケージパターンで解決するため、独自の `go.mod` を持つ入れ子ディレクトリは対象外になる。あわせて `make fmt` を `$(GO) fmt` に、`fmt-check` の対象解決を `$(GO) list` に寄せ、複数の Go バージョンが入った環境でも[バージョンの単一情報源](#go-バージョンを二重管理しない)から外れないようにする。

import の並び順は `gocritic` / `revive` の範囲では強制しない。必要になった時点で golangci-lint の `formatters` に `goimports` を追加する（設定ファイル 1 行の追加で済むため、先回りしない）。

## Linterly

ファイル・ディレクトリ単位の行数上限をチェックし、肥大化を防ぐ。UI 層を Atomic Design で細かく分割する設計（[TUI コンポーネント設計](../ui/atomic-design.md)）と方向が一致するため、pre-commit と CI の両方で実行する。

`.linterly.yml`:

```yaml
rules:
  # max_lines_per_file / max_lines_per_directory はデフォルト値（300 / 2000）を使う。
  # 特別な理由がない限り変更しない。
  # max_lines_per_file: 300
  # max_lines_per_directory: 2000
  warning_threshold: 10

count_mode: all
default_excludes: true
language: ja
```

- **上限はデフォルト値のまま使う。** 上限に当たった場合は数値を上げるのではなく、分割を検討する。分割できない正当な理由がある場合のみ、理由をこのドキュメントに記録してから変更する。
- `warning_threshold: 10` は**行数ではなくパーセント**である。上限 300 行に対して `300 × (1 + 10 / 100) = 330` 行が境界になり、301〜330 行は `warn`、331 行以上が `error` になる。上限を超えた時点で即座に失敗させず、分割の猶予を持たせるための設定である。
- `count_mode: all`（コメント・空行を含む全行を数える）は変更しない。

`.linterlyignore`:

```
# 自動生成コードのみを除外する。
# 手書きの Go ソースコードは除外しない（除外すれば行数管理の意味が無くなる）。

# Go のモジュール管理ファイル（tool ディレクティブの推移的依存で機械的に増える）
go.mod
go.sum

# 仕様書（Go ソースではないため行数上限の対象外）
*.md

# ツールの作業状態（人が行数を管理する対象ではない）
.sweep/
```

`default_excludes: true` は `.git/` や `dist/` 等を自動で除外するが、その既定リストに `go.mod` / `go.sum` / `*.md` / `.sweep/` は含まれないため、この 4 つは明示的に追記している。**linterly は `.gitignore` を読まない**（除外は `default_excludes` と `.linterlyignore` だけで決まる）ため、gitignore 済みのツール生成物も明示的に書く必要がある。

- `go.mod` / `go.sum` は自動生成の依存マニフェストであり、行数を人が管理する対象ではない。tool ディレクティブで開発ツールを追加すると推移的依存で機械的に膨らみ、`go.sum` は初版時点で 1000 行を超える。
- `*.md` は Go のソースではない。分割の単位は行数ではなくドキュメントとしての章立てで決まるため、行数上限の対象外とする。
- `.sweep/` は `issue-sweep` の作業状態である。`.sweep/spinoff-draft.jsonl` はツールが 1 行ずつ追記する JSONL で、行数を人が管理する対象ではない（放置すれば上限超過で `linterly check` を落とす）。位置づけは `go.mod` / `go.sum` と同じ。

| 違反レベル | 挙動 |
|-----------|------|
| `warn` | 閾値超え。終了コード 0（コミット・CI は通る） |
| `error` | 上限超え。終了コード 1（コミット・CI が失敗する） |

## Git Hooks

Lefthook を使う。`make hooks`（= `go tool lefthook install`）で登録する。

| フック | 実行内容 | 狙い |
|--------|---------|------|
| pre-commit | gofmt（自動修正）・golangci-lint・linterly | 数秒で終わる静的チェックのみ。コミットを軽く保つ |
| pre-push | `go test ./...` | 壊れたコードをリモートに上げない |

`lefthook.yml`:

```yaml
pre-commit:
  parallel: false
  commands:
    fmt:
      glob: "*.go"
      run: gofmt -w {staged_files}
      stage_fixed: true
    lint:
      glob: "*.go"
      run: go tool golangci-lint run
    linterly:
      run: go tool linterly check

pre-push:
  commands:
    test:
      run: go test ./...
```

- `fmt` は `stage_fixed: true` により整形結果を自動で staging に戻す。整形漏れでコミットが失敗する状況を作らない。
- **実行順は `parallel: false` だけでは決まらない。** `parallel: false` は同時実行を止めるだけで（lefthook の既定値でもあるため `lefthook dump` の出力からは消える）、順序は `commands` のキー名の**辞書順**で決まる。`fmt` の整形結果を `lint` が見る必要があるため、`parallel: false` に加えて `fmt` < `lint` < `linterly` となる命名を維持する必要がある。実測では `fmt` を `zfmt` にリネームすると実行順が `lint` → `linterly` → `zfmt` に変わり、整形前のコードを読んだ `lint` が gofmt 違反で先に落ちた。
- **既知の制約: `lint` / `linterly` はステージ内容ではなく作業ツリー全体を見る。** `fmt` は `{staged_files}` にスコープされるが、`lint`（`go tool golangci-lint run`）と `linterly` は対象を絞っていない。lefthook が未ステージ変更を隠すのは**同一ファイル内に staged と unstaged が混在するケースだけ**で、完全に未ステージのファイルは隠されない（実測）。そのため、ステージした内容がすべてきれいでも無関係な作業中ファイルの整形崩れでコミットが落ち、しかも `fmt` はそのファイルを直さない（`{staged_files}` に入らないため）。`.go` の**削除のみ**のコミットでも同じ症状になる（`fmt` は対象ファイルが無くスキップされるが、`lint` は `glob` がマッチして実行される）。対処（`--new-from-rev=HEAD` でのスコープ限定など）は別 Issue で決着させる。
- **`make hooks` は入れ子の git worktree 内では実行しない。** git worktree では `.git` が `gitdir:` 参照のファイルになり、`git rev-parse --git-path hooks` はリポジトリ共有の `<リポジトリルート>/.git/hooks` を返す（実測）。そのため worktree 内での `make hooks` はメインの作業ツリーと将来のすべての worktree に同時に効く。**メインの作業ツリーで一度登録すれば全 worktree に効く**ため、登録はそこで行う。あわせて、`lefthook.yml` を持たないブランチで `make hooks` を実行すると**テンプレートの `lefthook.yml` が生成される**ため、登録はこのファイルがあるブランチで行う。
- `lefthook install` は既存の同名フックを `*.old` に退避し、`lefthook uninstall` で復元する（可逆）。また、設定に無い `prepare-commit-msg` も生成される（lefthook 側の仕様）。
- **`--no-verify` でのスキップは行わない。** フックが失敗した場合はスキップせず原因を直す。CI で同じチェックが動くため、スキップしても後で落ちるだけである。
- フック実行を一時的に無効化する必要がある場合は `LEFTHOOK=0` を使い、理由を PR に書く。

## Swagger / OpenAPI

**対象外とする。** 本ツールは TUI であり API を提供しない。GitHub REST API は利用する側であり、スキーマを公開する対象がない（[外部インターフェース](../api/external-interfaces.md)）。

利用する GitHub API のエンドポイントと必要なトークンスコープは同ドキュメントに表形式で定義されているため、別途 OpenAPI 定義を持つ意味がない。

## 改訂履歴

| 版 | 日付 | 変更内容 | 変更理由 |
|----|------|---------|---------|
| 1.0 | 2026-08-21 | 新規作成 | 初版 |
| 1.1 | 2026-08-21 | CI のランナーを `ubuntu-latest` から self-hosted（`[self-hosted, linux, x64]`、org レベル）へ変更し、前提と fork PR ガードを追加 | 本ツールの対象環境と CI 環境を一致させるため。public リポジトリで self-hosted runner を使うと fork PR 経由でホスト上に任意コードが実行されるため、ガードを仕様として固定する必要がある |
| 1.2 | 2026-08-21 | `.linterlyignore` に `go.mod` / `go.sum` / `*.md` を追加し、`warning_threshold` の説明をパーセント指定として修正 | 設定ファイルを実際に導入したところ、`default_excludes` の既定リストにこれらが含まれず、`go.sum`（1035 行）と 330 行超（`error` 判定）の仕様書 3 本が上限超過で `linterly check` を失敗させたため（`docs/components/overview.md` は 302 行で `warn` に留まる）。`warning_threshold` は上限までの行数ではなくパーセントとして解釈されることを実測で確認したため |
| 1.3 | 2026-08-21 | `make fmt` / `make fmt-check` の対象をモジュール内パッケージに限定（`$(GO) fmt ./...` / `gofmt -l $$($(GO) list -f '{{.Dir}}' ./...)`）し、Format 節に理由を追記。`.linterlyignore` に `.sweep/` を追加し、冒頭コメントを「手書きの Go ソースコード」に修正。「抑制の方針」に `internal/runner/systemd.go` の G204 暫定抑制を記録 | `gofmt` はパッケージではなくファイルシステムを再帰するため、`.` 指定では作業用の入れ子 git worktree 配下まで対象に含み、`make fmt` が別ブランチのファイルを書き換えていた。`.sweep/spinoff-draft.jsonl` は追記型 JSONL でいずれ上限を超えるが、linterly は `.gitignore` を読まないため明示除外が必要。仕様に反する G204 抑制がコード側コメントにしか記録されておらず、仕様書だけでは追えなかったため |
| 1.4 | 2026-08-21 | 「Git Hooks」節に既知の制約（pre-commit の `lint` / `linterly` は作業ツリー全体を見る）と `make hooks` の注意（共有 `.git/hooks` への書き込み、`lefthook.yml` の自動生成、既存フックの `*.old` 退避、`prepare-commit-msg` の生成）を追記し、`parallel: false` の説明を「実行順は `commands` のキー名の辞書順で決まる」旨に修正。`.gitignore` に `lefthook-local.yml` を追加 | lefthook を実際に導入して挙動を実測したところ、`fmt` だけが `{staged_files}` にスコープされ `lint` / `linterly` は作業ツリー全体を読むため、無関係な未ステージファイルの整形崩れでコミットが落ちる（かつ `fmt` はそのファイルを直さない）ことを確認した。lefthook が未ステージ変更を隠すのは同一ファイル内に staged と unstaged が混在する場合だけである。また `fmt` を `zfmt` にリネームすると実行順が `lint` → `linterly` → `zfmt` に変わり、`parallel: false` が順序の必要条件にすぎないことを確認した。`git rev-parse --git-path hooks` は worktree からでも共有の `<リポジトリルート>/.git/hooks` を返すため、入れ子 worktree での `make hooks` がメインの作業ツリーに副作用を出す。`lefthook-local.yml` は lefthook 標準のローカル上書きファイルで、置かれた場合に誤コミットされるため |
| 1.5 | 2026-08-21 | 「fork からの PR で self-hosted ジョブを起動しない」節を書き換え、`if` 条件を多層防御の 1 層（一次防御は fork PR の承認ポリシーと org runner group の対象リポジトリ限定）と位置づけ、`skipped` が required status check では success 扱いになること・fork PR のマージを機械的に止める場合はゲートジョブが必要なこと・トリガー追加時のガード見直しと許可リスト形への移行候補を追記。「CI/CD」節に初版では `build` ジョブが失敗する旨を追記 | CI を実際に導入して確認したところ、従前の記述は fork ガードの実効性を過大に書いていた。`pull_request` はワークフロー定義をマージコミット側から取るため fork 側で `if:` 行を削除した改変版が実行され得る（`if` は悪意ある第三者に対する境界にならない）。`skipped` は required status check に対して success として報告されるため、fork PR が CI 未実行のまま緑になりマージ可能に見える。承認ポリシーは実測で `first_time_contributors` であり、一度コミットが取り込まれたユーザーは以後承認不要になる。public リポジトリ + self-hosted runner の組み合わせでは、[セキュリティ設計](../architecture/security.md#パスワード不要-sudo-の要求への対応)の言うとおり `NOPASSWD: ALL` 付与時に実質 root を渡すことになるため、防御の位置づけを正確に書く必要があった。あわせて `make build` が `cmd/gsr-helper` 未作成で失敗すること（Issue #3 で解消）を実測で確認したため |

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
Makefile                   タスク定義（CI / 手元で共用。Git Hooks は一部のみ経由）
```

## 開発環境セットアップ

### 必要なもの

| ツール | 用途 | 備考 |
|--------|------|------|
| Go | ビルド・テスト・開発ツールの実行 | バージョンは `go.mod` に従う |
| make | タスク実行 | 大半の Linux ディストリビューションに同梱 |
| git | バージョン管理・Git Hooks | — |
| C コンパイラ（`gcc` / `cc`） | `make test` の競合検出 | `-race` は cgo を必要とする（実測: `CGO_ENABLED=0 go test -race` が `-race requires cgo` で失敗する）。Debian 系では `build-essential` で導入する |

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

`make hooks` は**入れ子の git worktree 内では実行しない**。登録はメインの作業ツリーで一度行えば全 worktree に効き、`lefthook.yml` を持たないブランチで実行するとテンプレートが生成される。詳細は [Git Hooks](#git-hooks) を参照。

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

lint / test / build のコマンド列を Makefile に集約し、**CI と手元は同じ Makefile ターゲットを呼ぶ**。コマンドの二重管理を防ぐことが目的である。

**Git Hooks は一部だけ make を経由する。** 対象をコミット内容に絞る必要がある `fmt`（`{staged_files}`）と `lint`（`--new-from-rev=HEAD`）はコマンドを直接呼び、対象を絞れない `linterly` と絞る必要のない `test` は `make linterly` / `make test` を呼ぶ。判断基準は[make を経由するかどうかの基準](#make-を経由するかどうかの基準)にまとめている。

| ターゲット | 内容 |
|-----------|------|
| `make help` | ターゲット一覧を表示（既定） |
| `make tools` | 依存モジュールと開発ツールの取得 |
| `make fmt` | `go fmt ./...` でモジュール内のパッケージを整形する |
| `make fmt-check` | `gofmt -l` の対象を `go list ./...` で解決したモジュール内の Go ファイルに限定し、未整形のファイルがあれば失敗する（CI / `make check` 用）。`go list` の失敗と対象 0 件も失敗として扱う |
| `make vet` | `go vet ./...` |
| `make lint` | `golangci-lint run` |
| `make linterly` | 行数チェック |
| `make test` | `go test -race ./...`（競合検出あり） |
| `make build` | `go build ./...` で全パッケージのコンパイルを検証し、`cmd/gsr-helper` が存在する場合はさらに単一バイナリ `gsr-helper` を生成する |
| `make run` | `build` を実行してから生成したバイナリを起動する。引数は `ARGS` で渡す（`make run ARGS="--root /path/to/actions-runner"`） |
| `make hooks` | Lefthook を Git Hooks に登録 |
| `make check` | `fmt-check` → `vet` → `lint` → `linterly` → `test` を順に実行 |

```makefile
.DEFAULT_GOAL := help

GO   ?= go
BIN  := gsr-helper
CMD  := ./cmd/gsr-helper

# run に渡す引数。make run ARGS="--root /path/to/actions-runner" のように使う。
ARGS ?=

# go fmt が内部で使う gofmt（GOROOT/bin/gofmt）を fmt-check でも使い、整形と検査で
# ツールチェーンがずれないようにする。
GOFMT ?= $(shell $(GO) env GOROOT)/bin/gofmt

# gofmt はパッケージ単位ではなくファイルシステムを再帰するため、対象はディレクトリでは
# なくファイル単位で解決する。go fmt ./... と同じ集合（テストとビルドタグで除外された
# ファイルを含み、testdata/ と入れ子 worktree は含まない）になる。
GOFILES_TMPL := {{range .GoFiles}}{{printf "%s/%s\n" $$.Dir .}}{{end}}{{range .CgoFiles}}{{printf "%s/%s\n" $$.Dir .}}{{end}}{{range .TestGoFiles}}{{printf "%s/%s\n" $$.Dir .}}{{end}}{{range .XTestGoFiles}}{{printf "%s/%s\n" $$.Dir .}}{{end}}{{range .IgnoredGoFiles}}{{printf "%s/%s\n" $$.Dir .}}{{end}}

.PHONY: help tools fmt fmt-check vet lint linterly test build run hooks check

help: ## ターゲット一覧を表示する
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

tools: ## 依存モジュールと開発ツールを取得する
	$(GO) mod download

fmt: ## gofmt で整形する
	$(GO) fmt ./...

fmt-check: ## 未整形のファイルがないか確認する
	@files=$$($(GO) list -f '$(GOFILES_TMPL)' ./...) || exit 1; \
	if [ -z "$$files" ]; then \
		echo "対象の Go ファイルがありません" >&2; exit 1; \
	fi; \
	out=$$($(GOFMT) -l $$files) || exit 1; \
	if [ -n "$$out" ]; then \
		echo "gofmt が必要なファイル:"; echo "$$out"; exit 1; \
	fi

vet: ## go vet を実行する
	$(GO) vet ./...

lint: ## golangci-lint を実行する
	$(GO) tool golangci-lint run

linterly: ## 行数チェックを実行する
	$(GO) tool linterly check

test: ## テストを実行する（競合検出あり）
	$(GO) test -race ./...

build: ## 全パッケージをコンパイル検証し、エントリポイントがあればバイナリを生成する
	$(GO) build ./...
	@if [ -d "$(CMD)" ]; then \
		echo "$(GO) build -o $(BIN) $(CMD)"; \
		$(GO) build -o $(BIN) $(CMD); \
	fi

run: build ## TUI を起動する（make run ARGS="--root /path/to/actions-runner"）
	./$(BIN) $(ARGS)

hooks: ## Git Hooks を登録する
	$(GO) tool lefthook install

check: fmt-check vet lint linterly test ## すべてのチェックを実行する
```

### テストは常に競合検出付きで実行する

`make test` は `go test -race ./...` である。**競合検出を別ターゲット（`test-race` 等）に分けない。** CI・`make check`・pre-push フックはいずれも `make test` を呼ぶため、ターゲットを分けると「どの経路で競合検出が働くか」が増えた分だけ分岐し、CI だけで検出される競合が手元で再現しない状態を作る。

| 項目 | 実測値 |
|------|-------|
| `go test ./...`（ビルドキャッシュあり） | 1.5 秒 |
| `go test -race ./...`（ビルドキャッシュあり） | 21 秒 |

増分のほぼ全部が `internal/exec/command` の 20.6 秒である。このパッケージのテストは外部コマンドへの依存を避けるために**テストバイナリ自身を子プロセスとして起動する**ため、競合検出を有効にしたバイナリの起動コスト（1 回あたり約 1 秒）を起動回数ぶん払う。ほかのパッケージは合計しても数秒に収まる。

**pre-push では競合検出付きで実行する。** push はコミットより頻度が低く、20 秒台は「壊れたコードをリモートに上げない」目的に見合う。一方 pre-commit には入れない（「数秒で終わる静的チェックのみ」という狙いを壊す）。

**`-race` は cgo を必要とする。** C コンパイラが無い環境では `-race requires cgo` で失敗するため、開発マシンと runner ホストの双方に `gcc` が必要である（[ランナーホストのセットアップ](../operations/runner-host-setup.md#c-コンパイラ)）。

## CI/CD

### 構成

| 項目 | 内容 |
|------|------|
| プラットフォーム | GitHub Actions |
| ランナー | self-hosted（`runs-on: [self-hosted, linux, x64]`） |
| トリガー | `main` への push、および PR |
| ジョブ | fork ガードの `guard`（GitHub ホストランナー）と、それに依存する `lint` / `test` / `build` の 3 本を並列実行 |
| 設定の不変条件 | `internal/buildconfig` のテストが `ci.yml` / `Makefile` / `lefthook.yml` / lint 設定に加え、`docs/` 配下のドキュメントの不変条件を検証する。`test` ジョブで実行されるため、CI で機械的に守られる |
| デプロイ | なし（配布は `go install`。[非機能要件 / 可搬性](../requirements/non-functional.md#可搬性)） |

`lint` / `test` / `build` を並列にするのは、lint が落ちてもテスト結果が同時に得られるようにするためである。この 3 つの間に依存はなく、いずれも fork ガードの `guard` ジョブだけに依存する（[fork からの PR で self-hosted ジョブを起動しない](#fork-からの-pr-で-self-hosted-ジョブを起動しない)）。

`permissions` は `contents: read` のみを与える。CI はリポジトリへの書き込みを行わない。

`concurrency` はグループを `${{ github.workflow }}-${{ github.ref }}` とし、`cancel-in-progress` を `${{ github.event_name == 'pull_request' }}` にする。

- **`group` にワークフロー名を含める。** リテラルの `ci-<ref>` にすると group はリポジトリ内の全ワークフローで共有されるため、将来 `release.yml` 等が同じ group を使うと相互にキャンセルし合う。
- `github.ref` は `main` への push が `refs/heads/main`、PR が `refs/pull/<番号>/merge` になるため、**push と PR でグループが衝突しない**（GitHub Actions の仕様）。
- concurrency は run 単位で効くため、**同一 run 内の `lint` / `test` / `build` の 3 ジョブは互いをキャンセルしない**。
- PR に追加 push すると同じ PR の前の run がキャンセルされ、runner が即座に解放される。**オンラインの runner が限られる self-hosted 環境では、待ち行列の膨張を抑える効果が大きい**。
- **`main` への push ではキャンセルしない。** `cancel-in-progress: true` を無条件にすると、連続マージで先行する `main` の run がキャンセルされ「一度も検証されていない `main` コミット」が生まれる。`main` は `go install` による配布元なので、これは避ける。

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
  # group にワークフロー名を含め、将来追加するワークフローと相互キャンセルしない。
  group: ${{ github.workflow }}-${{ github.ref }}
  # main への push はキャンセルしない（未検証の main コミットを作らない）。
  cancel-in-progress: ${{ github.event_name == 'pull_request' }}

jobs:
  guard:
    # self-hosted runner を使うジョブの前段ゲート。GitHub ホストランナーで動かし、
    # 許可したトリガー以外では「失敗」して後続を止める（skip ではないため
    # required status check として fork PR のマージを機械的に止められる）。
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - name: トリガーと head リポジトリを検証する
        env:
          EVENT_NAME: ${{ github.event_name }}
          HEAD_REPO: ${{ github.event.pull_request.head.repo.full_name }}
          BASE_REPO: ${{ github.repository }}
        run: |
          case "$EVENT_NAME" in
            push)
              exit 0
              ;;
            pull_request)
              if [ "$HEAD_REPO" = "$BASE_REPO" ]; then
                exit 0
              fi
              echo "fork ($HEAD_REPO) からの PR では self-hosted runner のジョブを実行しない" >&2
              exit 1
              ;;
          esac
          echo "許可していないトリガー ($EVENT_NAME) では self-hosted runner のジョブを実行しない" >&2
          exit 1

  lint:
    needs: guard
    runs-on: [self-hosted, linux, x64]
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@fbc6f3992d24b796d5a048ff273f7fcc4a7b6c09 # v5
        with:
          persist-credentials: false
      - uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6
        with:
          go-version-file: go.mod
          cache: false
      - run: make fmt-check
      - run: make vet
      - run: make lint
      - run: make linterly
      - run: go tool lefthook validate

  test:
    needs: guard
    runs-on: [self-hosted, linux, x64]
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@fbc6f3992d24b796d5a048ff273f7fcc4a7b6c09 # v5
        with:
          persist-credentials: false
      - uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6
        with:
          go-version-file: go.mod
          cache: false
      - run: make test

  build:
    needs: guard
    runs-on: [self-hosted, linux, x64]
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@fbc6f3992d24b796d5a048ff273f7fcc4a7b6c09 # v5
        with:
          persist-credentials: false
      - uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6
        with:
          go-version-file: go.mod
          cache: false
      - run: make build
```

**`actions/setup-go` のキャッシュは `cache: false` で無効にする。** self-hosted runner ではジョブ間でホストが変わらず、モジュールキャッシュ（`GOMODCACHE`）とビルドキャッシュ（`GOCACHE`）はホスト側にそのまま残る。`actions/checkout` の `clean: true` は作業ディレクトリを掃除するだけでこれらには触れない。したがって `setup-go` のキャッシュ機構は、既に手元にあるものを tar で固めて保存し次回展開し直すだけの重複であり、速度上の利点がない。

加えて、同一の runner ホストで**実障害が記録されている**: runner が 13 台同居した状態で `setup-go` の `tar -xf cache.tzst` / `unzstd` が D state のまま 26 分滞留し、load average 47 に達してジョブが timeout で cancelled になった。gsr-helper は tool ディレクティブで golangci-lint の依存閉包を抱えるため `go.sum` が 98KB あり、キャッシュ blob が大きくこの問題を踏みやすい。

**アクションはフルコミット SHA でピン留めする。** `actions/checkout@v5` のような可変タグは、タグの付け替えや上流アカウントの侵害で参照先のコードが差し替わる。[セキュリティ設計](../architecture/security.md#パスワード不要-sudo-の要求への対応)のとおり self-hosted runner のワークフローは実質 root 相当で動くため、この脅威モデルでは被害がホスト全体に及ぶ。版の追従は `.github/dependabot.yml` の `github-actions` エコシステムに任せる（人手ではタグの移動を追えない）。SHA の隣にはバージョンをコメントで残し、読んだときに版が分かるようにする。

```yaml
version: 2

updates:
  # ワークフローのアクションはフルコミット SHA でピン留めしているため、
  # 新しい版への追従は Dependabot に任せる（人手ではタグの移動を追えない）。
  - package-ecosystem: github-actions
    directory: /
    schedule:
      interval: weekly
```

**`actions/checkout` には `persist-credentials: false` を指定する。** 既定では `GITHUB_TOKEN` をローカルの `.git/config` に書き込み、post-job cleanup で削除する。しかし `cancel-in-progress` によるキャンセルでは cleanup が完走しない可能性があり、self-hosted runner は**作業ディレクトリを再利用する**ため、トークンが残留する窓が開く（次回の `actions/checkout` が行う `git clean -ffdx` は `.git` 自体を対象にしないので自動的には消えない）。CI のどの step も認証付きの git 操作を必要としない。

**全ジョブに `timeout-minutes` を設定する。** 指定が無いと GitHub 既定の 6 時間までジョブが runner を占有する。オンラインの runner が 1 台しかない状況では、1 ジョブのハングが CI 全体を止める。

`lint` ジョブは `make check` を 1 step で呼ばず、`make fmt-check` / `make vet` / `make lint` / `make linterly` を**個別の step として列挙する**。どのチェックで落ちたかが run の一覧から分かるためである。そのぶん「実行すべきチェックの集合」が Makefile の `check` と CI の step 列の 2 箇所に存在するため、**`check` にターゲットを追加する際は CI の step も更新する**必要がある。

加えて `go tool lefthook validate` を step として実行し、`lefthook.yml` の構文退行を CI でも機械検知する（`make check` には含まれない。フック設定はテスト対象のコードではないため、Makefile の `check` ではなく CI の step に置く）。

**`build` ジョブは `cmd/gsr-helper` が存在しない段階でも成功する。** `make build` は `go build ./...` で全パッケージのコンパイルを検証し、エントリポイントの生成は `cmd/gsr-helper` があるときだけ行う（Makefile 側でディレクトリの有無を判定する）。エントリポイントは Issue #3 の成果物であり、#3 で `cmd/gsr-helper` が追加されると同じ `make build` がそのまま単一バイナリ `gsr-helper` の生成まで行う。ディレクトリの有無で分岐させるのは、`go build ./...` だけではリンク済みの配布物が得られず、`go build -o $(BIN) $(CMD)` だけでは対象パッケージが無い間 `directory not found` で失敗する（実測: `go build` が exit 1、`make` が exit 2）ためである。

### 設定ファイルの不変条件をテストで守る

`.github/workflows/ci.yml` / `Makefile` / `lefthook.yml` / `.golangci.yml` / `.linterly.yml` は、取り決めを破ってもコンパイルエラーにならず、通常のテストでも検知できない。とくに **self-hosted runner を使うジョブを 1 本追加した人が `needs: guard` を書き忘れると、fork ガードを迂回する退行が静かに入る**。`actionlint` はカスタムルールを持てないため、この種の不変条件は検出できない。

同じことが `docs/` 配下のドキュメントにも当てはまる。仕様書のコードブロックが設定ファイルの実体から乖離しても、改訂履歴の版番号が重複・逆順になっても、コンパイルエラーにも通常のテストの失敗にもならない。

そこで `internal/buildconfig` に**設定ファイルとドキュメントの不変条件を守る回帰テスト**を置く。実行時のコードを持たないテスト専用のパッケージで、一時ディレクトリに最小のモジュールを作って `make` を実際に走らせるもの、設定ファイルを読んで内容を検証するもの、`docs/` 配下の Markdown を読んで内容を検証するものからなる。**`make test` の一部として CI（`test` ジョブ）と pre-push フックの双方で実行される**ため、CI 専用の step を足すより検知が早い。

| 守っている不変条件 | 破ったときに落ちるテスト |
|---|---|
| self-hosted ジョブは必ず `needs: guard` を持つ | `TestCISelfHostedJobsDependOnGuard` |
| `guard` は許可リスト形（`push` と同一リポジトリの `pull_request` 以外は失敗する） | `TestCIGuardScriptAllowsOnlySameRepositoryEvents` |
| アクションはフルコミット SHA でピン留めされている | `TestCIActionsArePinnedToCommitSHA` |
| 全ジョブに `timeout-minutes` がある | `TestCIJobsHaveTimeout` |
| `make fmt-check` が入れ子 worktree と `testdata/` を対象にしない | `TestFmtCheckSkipsNestedWorktree` / `TestFmtCheckSkipsTestdata` |
| `make test` が競合を検出する | `TestMakeTestDetectsDataRace` |
| 仕様書のコードブロックが設定ファイルの実体と一致する | `TestSetupDocEmbedsConfigFilesVerbatim` |
| ドキュメントの改訂履歴の版番号が重複せず昇順である | `TestDocRevisionHistoryVersionsUniqueAndAscending` |

この表は網羅ではない。設定やドキュメントに新しい取り決めを入れたときは、同じ場所にテストを足す。

**この表が挙げるのは `internal/buildconfig` に置いたものだけである。** 同じ「ドキュメントと実装の一致をテストで守る」性格の検査は他のパッケージにもあり、**検査は検査対象の隣に置く**——`internal/ui/page/pagetest/import_test.go` の `TestSharedPackagesMatchDoc` / `TestDirectoryTreeMatchesShared` / `TestImplementedListCoversSharedPackages`（[TUI コンポーネント設計](../ui/atomic-design.md)の共有部品の列挙 3 箇所と `shared` マップの一致）や `TestNoProductionCodeImportsTestFixtures` / `TestOnlyTabsetImportsTabs`（依存の向き）がそれにあたる。`internal/buildconfig` へ集めるのは、**どのパッケージにも属さない取り決め**（`Makefile` / CI ワークフロー / `.linterly.yml` / 全文書に共通の改訂履歴の規則）だけである。パッケージ固有の不変条件をここへ寄せると、対象を触る Issue が検査の存在に気付けない。

### self-hosted runner を使う前提

CI は GitHub ホストランナーではなく self-hosted runner で実行する。本ツールが管理する対象そのものの上で CI が回るため、以下を前提とする。

| 項目 | 前提 |
|------|------|
| runner の所在 | org（`ousiassllc`）レベルに登録された runner を使う。リポジトリレベルには登録しない |
| ラベル | `self-hosted` / `linux` / `x64` の 3 つを AND で要求する。1 つでも欠けるとジョブはエラーにならず無期限に `queued` で止まる（[ランナーホストのセットアップ](../operations/runner-host-setup.md#ラベル)） |
| OS | Linux（amd64）。対象 OS と一致するため、GitHub ホストランナーでは検証できない `systemctl` / `journalctl` 前提の挙動もそのまま確認できる |
| ワークスペース | ジョブ間で作業ディレクトリが再利用される。`actions/checkout` の既定（`clean: true`）に依存し、ビルド成果物を残す前提のステップを書かない |
| ツールの導入 | Go は `actions/setup-go` がツールキャッシュへ導入する。ホストに Go を事前インストールしない（バージョンの二重管理を避ける） |
| C コンパイラ | `make test` は `-race` 付きで実行され、`-race` は cgo を必要とするため `gcc` がホストに必要である。`setup-go` は C コンパイラを導入しない（[ランナーホストのセットアップ](../operations/runner-host-setup.md#c-コンパイラ)） |
| 並列実行 | `lint` / `test` / `build` の 3 ジョブが同時に走るため、runner は 3 台以上を稼働させる。台数が足りない場合はジョブが順番待ちになるだけで失敗はしない。前段の `guard` は GitHub ホストランナーで動くため self-hosted の台数を消費しない |
| 権限 | CI ジョブは runner の実行ユーザー権限で動く。`sudo` を必要とするテストを CI に置かない（`Executor` のテスト実装で代替する。[非機能要件 / 保守性・テスト](../requirements/non-functional.md#保守性テスト)） |

#### fork からの PR で self-hosted ジョブを起動しない

self-hosted runner でワークフローを実行することは、**そのワークフローに runner の実行ユーザー権限を与える**ことを意味する。[セキュリティ設計](../architecture/security.md#パスワード不要-sudo-の要求への対応)のとおり `NOPASSWD: ALL` の付与は「その runner で実行される任意のワークフローに実質 root を与える」ことに等しい。fork からの PR は第三者が書いた任意のコードを含むため、そのまま self-hosted runner で走らせるとホストが第三者の実行環境になる。

**本リポジトリは private であり、fork も無効である**（実測: `gh api repos/ousiassllc/gsr-helper` が `"visibility": "private"` / `"allow_forking": false`）。当初 public だったが、org（`ousiassllc`）レベルに登録した runner が **public リポジトリのジョブを引き取らず `queued` のまま停止した**ため private へ切り替えた。org の runner group は既定で public リポジトリへ runner を提供しない設定であり、これが直接の原因だった。private 化により runner がジョブを拾えるようになり、同時に**外部の第三者が fork PR を送る経路そのものが無くなる**。

したがって以下の脅威モデルは、**リポジトリへのアクセス権を持つ範囲（org メンバー・コラボレーター）**を対象とする。ただし将来 public へ戻す場合はそのまま外部第三者に対する分析として読める内容なので、記述は残す。

**防御は多層で構成する。ワークフロー側のゲートはその 1 層にすぎず、単体では境界にならない。**

| 層 | 手段 | 位置づけ | 現状 |
|----|------|---------|------|
| 一次防御 | リポジトリを private にし fork を無効化する | 外部第三者が fork PR を送る経路そのものを塞ぐ | 適用中（`allow_forking: false`） |
| 一次防御 | org runner group の対象リポジトリ限定 | 同じ runner を掴めるリポジトリを絞る。既定では public リポジトリへ提供しないため、public へ戻す場合は runner group 側を明示的に許可しない限り CI が `queued` で止まる | 手順は[ランナーホストのセットアップ](../operations/runner-host-setup.md#runner-group-の対象リポジトリ)に記録。設定変更は org 管理者の作業 |
| 一次防御 | fork PR の承認ポリシー | public へ戻す場合の実質的な境界。「全外部貢献者に承認必須」にする | **private では設定できない**（実測: `gh api repos/ousiassllc/gsr-helper/actions/permissions/fork-pr-contributor-approval` が 422 `Fork PR approval is not allowed for private repositories.`）。public へ戻す際に `all_external_contributors` を設定する |
| 補助 | ワークフローの `guard` ジョブ | 事故防止と runner 負荷削減。トリガーの追加ミスと善意の fork PR による誤起動を止める。required status check に**指定して初めて**マージも機械的に止められる | **ジョブは適用中だが、required status check への指定は未設定**（実測: `gh api repos/ousiassllc/gsr-helper/branches/main/protection` が 404 `Branch not protected`、`gh api repos/ousiassllc/gsr-helper/rulesets` が `[]`）。指定はリポジトリ設定側の作業 |

ワークフロー側は、self-hosted runner を使う全ジョブの前段に **GitHub ホストランナー上で動く `guard` ジョブ**を置き、`needs: guard` で依存させる（定義は[ワークフロー定義](#ワークフロー定義)の `ci.yml` を参照）。`guard` の判定は次のとおりである。

- `push`（`main`）では常に成功する
- `pull_request` では **head が同一リポジトリのブランチである場合のみ**成功する
- **それ以外のトリガーではすべて失敗する**（許可リスト形）
- 判定に使う値は `run:` 内へ式を直接埋め込まず `env:` 経由で渡す。head リポジトリ名を通したスクリプトインジェクションの余地を残さないためである

**ジョブ単位の `if:` ではなくゲートジョブにする理由。** `if:` で条件を満たさないジョブは `skipped` になるが、GitHub のドキュメントは「スキップされたジョブはステータスを Success として報告する。required check であっても PR のマージを妨げない」「required status check は保護ブランチへ変更を加える前に `successful` / `skipped` / `neutral` のいずれかである必要がある」と明記している。つまり `if:` で skip する設計では、将来 `lint` / `test` / `build` を required status check に指定しても、**CI が一度も走っていないのに 3 つとも緑になりマージ可能に見える**。`guard` は skip ではなく**失敗**するため、required status check に指定すればマージを機械的に止められる。したがって **required status check には `guard` を指定する**（`lint` / `test` / `build` は `needs: guard` により skip されるので、それらを指定してもマージは止まらない）。**これは指定して初めて効く運用であり、現時点では上表のとおり未設定である。**

**許可リスト形にする理由。** 従前の条件 `github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository` は「`pull_request` でなければ無条件に実行する」**ブロックリスト形**だった。`merge_group` や `workflow_dispatch` を `on:` に足すと左辺が true になって短絡し、fork チェックが評価されないまま実行される。`guard` は `push` と同一リポジトリの `pull_request` だけを許可し、それ以外は既定で失敗するため、トリガーを足したときに「実行しない」側へ倒れる。**トリガーを追加する際は `guard` の許可リストも更新する**（merge queue を導入する場合は `merge_group` を `on:` と `guard` の双方に足す。足さないと required status check が報告されずキューが詰まる）。

**このゲートもセキュリティ境界にはならない。** `pull_request` イベントでは、ワークフロー定義自体がマージコミット側（= PR の内容を含む側）から取られる（GitHub のドキュメントは `pull_request_target` を「`pull_request` イベントのようにマージコミットのコンテキストではなく、ベースリポジトリの既定ブランチのコンテキストで実行される」と対比して説明している）。つまり **fork 側で `.github/workflows/ci.yml` の `guard` ジョブごと削除でき、その改変版が `pull_request` の実行に使われる。**したがってゲートは事故防止と runner 負荷削減に有効で、マージゲートとしては required status check に指定した場合に限り働く（前段・上表のとおり現時点では未設定）が、悪意ある第三者を止めるのはリポジトリ設定側である。

fork からの PR を検証する場合は、内容を確認した上で同一リポジトリ内のブランチへ取り込み、そのブランチの PR で CI を通す。`pull_request_target` は使わない（fork の PR に対してベース側の権限でワークフローが動くため、この対策の意味が失われる）。

**実際の fork PR での実測は行えない。** private かつ `allow_forking: false` のため fork を作成する経路が存在しない。代わりに `guard` の判定ロジックは `internal/buildconfig` の回帰テストが `ci.yml` から実スクリプトを取り出して実行し、`push` / 同一リポジトリの PR / fork からの PR / 未許可トリガーを検証している。public へ戻す際は、承認ポリシーの設定とあわせて実 fork PR での確認を行う。

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
    - depguard
    - errorlint
    - gocritic
    - gosec
    - nolintlint
    - revive
  settings:
    depguard:
      rules:
        charm:
          deny:
            # Charm は charm.land/<name>/v2 に揃える。v1 系と混ぜると
            # bubbletea のキー入力 Msg の型が噛み合わず、幅計算と
            # カラープロファイル判定も二重になる。
            - pkg: github.com/charmbracelet
              desc: Charm は charm.land/<name>/v2 を使う（github.com/charmbracelet/* は v2 系の間接依存であり直接 import しない）
    nolintlint:
      # 不要になった抑制を検出する（置き換え完了時の除去漏れを防ぐ）
      allow-unused: false
      # 抑制には理由コメントを必須にする
      require-explanation: true
      # //nolint 単独を禁止し //nolint:gosec のようにリンター名を要求する
      require-specific: true
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

issues:
  # 抑制やエラーを棚卸しできるよう、同種の指摘を打ち切らない
  max-issues-per-linter: 0
  max-same-issues: 0
```

`default: standard` で有効になる標準セット（`errcheck` / `govet` / `ineffassign` / `staticcheck` / `unused`）に、以下を追加する。

| リンター | 追加する理由 |
|---------|------------|
| `gosec` | **root 権限で動作するツールであり、パス操作と外部コマンド実行の誤りが致命的になる。**削除処理のパス検証（[セキュリティ設計](../architecture/security.md)）を機械的に補強する |
| `errorlint` | エラーの比較・ラップの誤り（`==` による比較、`%v` でのラップ）を検出する。部分的な失敗を集約して返す設計上、エラーの取り扱いミスが表面化しにくい |
| `revive` | 命名と可読性の検査。「clear naming, small functions」の方針を機械的に支える |
| `gocritic` | 冗長な記述・非効率な記述の検出。パーサとマージ処理が中心のコードベースで効きやすい |
| `nolintlint` | `//nolint` 自体の検査。抑制方針（行単位・理由必須・リンター名必須）を機械的に強制し、不要になった抑制の除去漏れも検出する |
| `depguard` | import できるモジュールパスの制限。Charm の v1 系（`github.com/charmbracelet/*`）を禁止する |

`formatters` に `gofmt` を入れることで、`golangci-lint run` でも整形漏れを検出できる。`make fmt-check` と役割が重なるが、どちらか一方だけを実行しても検出できる状態にしておく。

`nolintlint` は 3 つの設定をすべて有効にする。

| 設定 | 値 | 検出されるもの |
|------|----|--------------|
| `allow-unused` | `false` | 対象の指摘が既に消えているのに残っている `//nolint`。暫定抑制の除去漏れを検出する |
| `require-explanation` | `true` | 理由コメントのない `//nolint:gosec` |
| `require-specific` | `true` | リンター名のない `//nolint` 単独指定 |

抑制方針を「書いておく規約」から**機械的に強制されるもの**へ引き上げるための設定である。とくに `allow-unused: false` は、**暫定として入れた抑制が不要になっても誰も気づかない**事故を防ぐ。golangci-lint は既定では不要な `//nolint` を報告しないため、「別 Issue で置き換えたら外す」と書いた抑制はそのまま残り続ける。

`depguard` は `github.com/charmbracelet` 以下の import を禁止する。[非機能要件 / 依存ライブラリ](../requirements/non-functional.md#依存ライブラリ)のとおり **Charm の 4 つ（bubbletea / bubbles / lipgloss / huh）は `charm.land/<name>/v2` に揃える**規定であり、`github.com/charmbracelet/*` が `go.mod` に現れるのは v2 系が内部で使う間接依存としてである。とくに `github.com/charmbracelet/lipgloss v1.1.0` は golangci-lint の推移依存として実際に `go.mod` にあるため、**禁止しないと誤って import してもビルドが通る**。v1 と v2 では bubbletea のキー入力 `Msg` の型が異なり、lipgloss のように型が近いものは混ざっても気付きにくい（幅計算とカラープロファイル判定も二重になる）。

**既知の制約: `depguard` はブランク import（`_ "github.com/charmbracelet/lipgloss"`）を検出しない**（実測）。実害のある使い方は通常の import になるため許容する。

`issues.max-issues-per-linter` / `max-same-issues` はどちらも `0`（無制限）にする。既定は 50 / 3 で、**同種の指摘が 4 件以上あると出力が打ち切られる**ため、抑制やエラーの棚卸しで件数を数え上げられない。終了コードには影響しないが、`golangci-lint run` の出力だけで実態を確認できる状態にしておく。

### 抑制の方針

- **抑制は行単位で行い、必ず理由を書く。** `//nolint:gosec // 引数は runner ディレクトリ配下であることを検証済み` のように、なぜ安全かを書く。ファイル単位・パッケージ単位の抑制は使わない。
- **`gosec` の G204（可変引数での外部コマンド実行）は `internal/exec` に集中する。** 外部プロセス実行は Executor 1 本に集約する設計（[アーキテクチャ設計](../architecture/overview.md#外部コマンドの実行)）のため、抑制箇所も 1 箇所に収まる。ドメイン層に G204 の抑制が現れた場合は、**抑制ではなく設計違反**として `exec` 層経由に直す。
- **この方針は現在満たされている。** かつて `internal/runner` の `systemctl show` 呼び出しに置いていた暫定の G204 抑制は、集約先の `internal/exec` が実装された時点で除去した。現在 G204 に相当する抑制は `internal/exec/command/command.go` の 1 箇所だけで、これは方針どおりの位置である。
- **テストファイル（`_test.go`）に対する `errcheck` / `gosec` の除外は、上記「ファイル単位・パッケージ単位の抑制は使わない」方針の明示的な例外である。** `.golangci.yml` の `exclusions.rules` で `path: _test\.go` に対して除外している。テストではエラーの取り扱い（後片付けの `Close` など）とパス操作（一時ディレクトリ配下のパス組み立て）を緩め、テストの記述量を抑えることが目的である。本番コードのパス検証とエラー処理の厳しさは落とさない。**この 2 リンター・`_test.go` 限定以外の除外は `exclusions` に追加しない。**
- **ツリーに現存する `//nolint` は次の 11 件（うち G304 が 9 件）で、すべて本番コードの行単位・理由付き・`gosec` 指定である。** いずれも方針上許容する恒久的な抑制で、除去の予定はない。

  | ファイル | 件数 | 抑制している検査 | 対象と根拠 |
  |---|---|---|---|
  | `internal/exec/command/command.go` | 1 | 可変引数での外部コマンド実行（G204） | シェルを経由せず実行ファイルと引数配列を直接渡すため、メタ文字によるコマンド注入が成立しない。`-` で始まる値によるオプションインジェクションは入力検証（`internal/setup`）の責務 |
  | `internal/ui/page/action/allow.go` | 1 | ハードコードされた資格情報（G101）の誤検知 | 認証を促す画面上の説明文であり、資格情報を含まない |
  | `internal/runner/config.go` | 3 | 変数を使ったファイル読み取り（G304） | `.runner` / `bin/runnerversion` / `.service`。下記「事前条件に依拠する根拠」 |
  | `internal/runner/procs/procs.go` | 1 | 同上（G304） | `/proc/<PID>/cmdline`。下記「数値検証による根拠」 |
  | `internal/appconfig/config.go` | 1 | 同上（G304） | `path` は利用者が指定した設定ファイルの位置そのもの（読み込みが本関数の目的）。`Clean` 済みで、内容は `Config` の形にのみデコードする |
  | `internal/audit/open.go` | 2 | 同上（G304） | `path` は非特権ユーザーが持つ設定ファイル（`audit_log`）由来の実質的な外部入力。`O_EXCL` / `O_NOFOLLOW` と fd 上の検証で安全性を担保する |
  | `internal/logs/tail.go` | 2 | 同上（G304） | 追従するログのパス。利用者が Logs タブの一覧から選ぶもので、その一覧は `logs.List` が runner ディレクトリ配下の `_diag` から `Runner_*.log` / `Worker_*.log` だけを列挙したものである。2 件は `Tail` の最初の `os.Open` と、ログが同名で作り直されたときに開き直す `reopen` の `os.Open` で、**開くのは同じ `path` 変数**なので根拠も同一である。下記「事前条件に依拠する根拠」 |

  **この表は `//nolint` の「行」を数えたものであり、G コード別の件数を `golangci-lint` の出力から機械的に検算することはできない。** `nolintlint` の `require-specific` が要求するのはリンター名であって規則 ID ではないため、ツリー内の抑制はいずれも G コードを書かない `//nolint:gosec` の形である。上表の「抑制している検査」列は、各行の nolint の理由コメントと対象コードから読み取ったものである。G コード別に裏取りしたい場合は抑制を外して `go tool golangci-lint run` を走らせる。

  **`_test.go` に実際の抑制指示（有効な `//nolint` ディレクティブ）は 1 つも無い。** `internal/buildconfig` の 2 つのテストには `//nolint` という文字列が現れるが、いずれもこのリポジトリのコードに対する抑制ではない。`golangci_test.go` のものは `nolintlint` が雑な抑制を実際に落とすことを確かめる**検証用フィクスチャの文字列リテラル**であり、`nolint_inventory_test.go` のものは上表をツリーと突き合わせるガードテスト自身の**doc コメント・検出に使う正規表現・失敗メッセージ**である。どちらも棚卸しの件数には影響しない。突き合わせを行う `countNolintInTree` が `_test.go` を走査対象から除いているためである。

  **G304 の抑制の安全性の根拠は一括では説明できない。** 9 件は次の 4 種類に分かれる（内訳は 5 + 1 + 1 + 2 件で、表の G304 の件数と一致する）。
  - **事前条件に依拠する根拠（5 件）。** `internal/runner/config.go` の 3 件はパスが「ディレクトリ + 固定名」で組み立てられているが、**そのディレクトリが外部入力でないことは呼び出し側の事前条件に依拠する**。`LoadConfig` は exported で `dir` を呼び出し側が自由に渡せるうえ、`Discover` の `Options.Roots`（「追加の走査ルート。既定ルートに追加される」）により**走査ルート自体を呼び出し側が指定できる**設計であり、`collectDirs` は `IsRunnerDir` で絞るだけなので dir が外部入力由来になる経路は閉じていない。したがって「呼び出し側が `Discover` が解決した runner ディレクトリを渡す」という事前条件のもとで安全である、というのが正確な根拠であり、これを doc コメントと nolint の理由に明記している。

    `internal/logs/tail.go` の 2 件も**同じ種類**である。`Tail` は exported で `path` を丸ごと呼び出し側から受け取るため、パスの安全性は「`logs.List` が列挙したログのパスを渡す」という事前条件に依拠する。`List` は `<runner.Dir>/_diag` を `os.ReadDir` で読み、`kindOf` が受け付ける `Runner_*.log` / `Worker_*.log` だけを残すので、その事前条件のもとでは runner ディレクトリ配下の診断ログ以外を開くことはない。**`internal/runner/config.go` との違いは、パスを自分で組み立てるか丸ごと受け取るかだけ**であり、根拠の形（呼び出し側の事前条件）は同じなので分類も同じにする。2 件のうち後者は `reopen` にあり、runner がログを同名で作り直したときに開き直す処理である。**開き直しでパスは変わらない**（`Tail` が受け取った `path` をそのまま使う）ため、追加の根拠は要らない。
  - **数値検証による根拠（1 件）。** `internal/runner/procs/procs.go` の 1 件は `/proc/<PID>/cmdline` であり「探索済みディレクトリ + 固定名」ではない。`<PID>` は `strconv.Atoi` で数値であることを検証済みのものだけを使う、という別の根拠で安全である。
  - **明示指定のパスと読み取り方に依拠する根拠（1 件）。** `internal/appconfig/config.go` の 1 件は `os.Open` をそのまま使い、`O_NOFOLLOW` も開いた fd 上の検証も行わない。`path` が利用者の指定した設定ファイルの位置そのもの（読み込みが本関数の目的）であること、`Clean` 済みであること、`io.LimitReader` による上限（`maxConfigSize`）付きで読むこと、内容を `Config` の形にのみデコードすることが根拠である。
  - **開いた後の検証による根拠（2 件）。** `internal/audit/open.go` の 2 件は、パスが非特権ユーザーの持つ設定ファイル（`audit_log`）由来の実質的な外部入力であり、外部入力でないとは主張しない。新規作成時は `O_EXCL` / `O_NOFOLLOW` で「今この呼び出しで作った」ことを保証し、既存ファイルを開く場合は `O_NOFOLLOW` と直後の `validateAuditFile` による開いた fd 上の検証で安全性を担保する。
- 抑制がさらに増えてきた場合は `.golangci.yml` の `exclusions` にルールとして書き、経緯をこのドキュメントに残す。
- **抑制の棚卸しは `go tool golangci-lint run` の出力をそのまま使える。** `.golangci.yml` の `issues.max-issues-per-linter` / `max-same-issues` を `0`（無制限）にしているため、同種の指摘が打ち切られない。既定（50 / 3）のままだと、nolint を全て外した状態で同じリンター・同じルールの指摘が 4 件以上あると 3 件で打ち切られ、この節が挙げる抑制の件数を出力から裏取りできない。上表のファイルパスと件数は現在のツリーと一致している。

### go vet

`golangci-lint` にも `govet` が含まれるが、CI では `make vet` を別ステップとして残す。golangci-lint の設定ミスや導入失敗時にも標準ツールのチェックが動くようにするためである。

## Format

**gofmt のみを使う。** Go 標準ツールチェーンで完結し、追加の依存もエディタ設定の追従も不要である。

| 用途 | コマンド |
|------|---------|
| 整形する | `make fmt` |
| 整形漏れを検出する | `make fmt-check` |

`gofmt -l` は未整形ファイルを列挙するだけで終了コードが 0 のままなので、`fmt-check` では出力が空であることを検証している。

**`gofmt` にはディレクトリではなくファイルを渡す。** `gofmt` はパッケージ単位ではなくファイルシステムを再帰するため、ディレクトリを渡すと作業用に切った入れ子の git worktree（`.claude/worktrees/` 配下の別ブランチのチェックアウト）や `testdata/` の `.go` ファイルまで拾う。`go list` が返す `{{.Dir}}` はディレクトリなので、リポジトリルートに `.go` ファイルが 1 本置かれてルート自体がパッケージになった時点で、`gofmt` がルート以下すべてを再帰してしまう。そのため対象は `go list` の `.GoFiles` / `.CgoFiles` / `.TestGoFiles` / `.XTestGoFiles` / `.IgnoredGoFiles` を展開した**ファイル単位**で解決する。この 5 つは `go fmt` が整形する集合と同一で（`go fmt -n ./...` が出力する `gofmt -l -w` の引数と一致する）、ビルドタグで除外されたファイルを含み `testdata/` を含まない。`fmt` と `fmt-check` で対象がずれると、`make fmt` では直せないのに `fmt-check` が落ち続けるデッドロックになる。

**`fmt-check` の `gofmt` は `$(GO) env GOROOT` から解決する。** `make fmt`（= `$(GO) fmt`）は GOROOT 配下の `gofmt` を起動するため、`fmt-check` が PATH 上の `gofmt` を使うと、`GO=/opt/go1.25/bin/go` のように差し替えた環境で整形と検査に別バージョンが動き、`make fmt` を何度実行しても `fmt-check` が落ちる状態になりうる。[バージョンの単一情報源](#go-バージョンを二重管理しない)の方針にも合わせ、両者を同じバイナリに固定する。

**`go list` の失敗と対象 0 件は明示的に失敗させる。** コマンド置換の終了ステータスを捨てると、`go.mod` の破損等で `go list` が失敗しても検査ゲートが静かに通ってしまう。また対象が 0 件だと `gofmt` が引数なしで起動して標準入力を読むため、端末から `make check` を実行すると無言でハングする。どちらも `exit 1` で止める。

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

# 実行ごとに GitHub Releases へ外向き HTTP が出るのを止める。
update_check: false
```

- **上限はデフォルト値のまま使う。** 上限に当たった場合は数値を上げるのではなく、分割を検討する。分割できない正当な理由がある場合のみ、理由をこのドキュメントに記録してから変更する。
- `warning_threshold: 10` は**行数ではなくパーセント**である。上限 300 行に対して `300 × (1 + 10 / 100) = 330` 行が境界になり、301〜330 行は `warn`、331 行以上が `error` になる。上限を超えた時点で即座に失敗させず、分割の猶予を持たせるための設定である。
- **`warning_threshold: 10` はコメントアウトしてはいけない。** 値そのものは linterly v0.3.3 の既定値と同一で（`internal/config/config.go` の `DefaultWarningThreshold = 10`。`count_mode: all` / `default_excludes: true` も同様に既定値）、`max_lines_per_file` / `max_lines_per_directory` と同じ「デフォルト値を使う」という理由でコメントアウトできそうに見えるが、この 1 行は **`rules:` セクションを非空に保つ構造上の役割**を担っている。`rules:` 配下を全てコメントアウトすると必須チェック（`v.IsSet("rules")`）に引っかかり、実測で `Error: "rules" section is required` の **exit 2** になる。
- `count_mode: all`（コメント・空行を含む全行を数える）は変更しない。
- `language: ja` は出力メッセージの言語指定である。
- **`update_check: false` は外向き HTTP を止めるための指定である。** 既定は `true` で、linterly v0.3.3 は `rootCmd.PersistentPreRun` から毎回 `startUpdateCheck()` を呼び、GitHub Releases へ問い合わせる。`make linterly` / `make check` / pre-commit / CI のすべてで発生するため、self-hosted runner の不要な egress と、新バージョン検出時に CI ログへ混入する想定外の出力の原因になる（実測: `strace -f -e trace=connect` で `linterly check` の外向き接続が 3 件 → 0 件になった）。`--no-update-check` / `LINTERLY_NO_UPDATE_CHECK` でも抑止できるが、設定ファイルに書けば実行経路をまたいで 1 箇所で済む。

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

`default_excludes: true` は `.git/` や `dist/` 等を自動で除外するが、その既定リストに `go.mod` / `go.sum` / `*.md` / `.sweep/` は含まれないため、この 4 つは明示的に追記している。**linterly は `.gitignore` を読まない**（除外は `default_excludes` と `.linterlyignore` だけで決まる）。したがって `.linterlyignore` に明示的に書く必要があるのは**`default_excludes` の既定リストに無いもの**だけであり、gitignore 済みかどうかは判断基準にならない。

たとえば作業用 worktree の置き場である `.claude/` は、`.idea/` / `.vscode/` / `.cursor/` / `.gemini/` と並ぶ AI・エディタの作業ディレクトリとして既定リストに**含まれる**ため、`.linterlyignore` への追記は不要である。実測では `.claude/worktrees/` 配下に 501 行のファイルを置いても検出されず `linterly check` は exit 0 で、`--no-default-excludes` を付けた場合のみ `error` になった。`.gitignore` に `.claude/worktrees/` を追加しているのは git の追跡から外すためであって、linterly のためではない。

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
| pre-commit | gofmt（自動修正）・golangci-lint（HEAD からの差分のみ）・linterly | 数秒で終わる静的チェックのみ。コミットを軽く保つ |
| pre-push | `make test`（競合検出あり） | 壊れたコードをリモートに上げない |

`lefthook.yml`:

```yaml
# hook スクリプトからは PATH 上の lefthook が優先されるため、go.mod でピン留めした
# バージョンを明示的に指定する。
lefthook: go tool lefthook

pre-commit:
  parallel: false
  commands:
    fmt:
      priority: 1
      glob: "*.go"
      run: gofmt -w {staged_files}
      stage_fixed: true
    lint:
      priority: 2
      glob: "*.go"
      run: go tool golangci-lint run --new-from-rev=HEAD
    linterly:
      priority: 3
      run: make linterly

pre-push:
  commands:
    test:
      run: make test
```

### make を経由するかどうかの基準

**対象をコミット内容に絞る必要があるコマンドは直接呼び、モジュール全体が対象のコマンドは make ターゲットを経由する。**

| command | 呼び方 | 理由 |
|---------|-------|------|
| `fmt` | 直接（`gofmt -w {staged_files}`） | `{staged_files}` へのスコープが必要。`make fmt`（= `go fmt ./...`）はモジュール全体を整形するため、`stage_fixed` が staged 以外のファイルまで stage してしまう |
| `lint` | 直接（`--new-from-rev=HEAD` 付き） | HEAD からの差分に絞る必要がある。`make lint` は CI 用の全体チェックであり、フラグを足すと CI と手元でチェック範囲が変わる |
| `linterly` | `make linterly` | 対象を絞れない。`linterly check [path]` はパスを 1 つしか取らず、ディレクトリ単位の行数上限は作業ツリー全体を見ないと判定できない |
| `test` | `make test` | 対象を絞る必要がない。テストの実行方法（`-race` 等）を Makefile の 1 箇所で管理できる |

この基準により、**スコープの制約が無いコマンドはコマンド列の二重管理が起きない**。`make check` に新しいターゲットを足したときにフックへ反映するかは、この表の基準で判断する。

### pre-commit の lint は HEAD からの差分だけを見る

`go tool golangci-lint run --new-from-rev=HEAD` として、**HEAD 時点で既に存在する指摘を無視する**。作業ツリー全体を無条件に検査すると、コミット済みの既存指摘が 1 件あるだけで**以後すべてのコミットが落ち続ける**（実測: 既存コミットに `errcheck` 違反を 1 件入れると、無関係でクリーンな別ファイルのコミットも失敗した。`--new-from-rev=HEAD` では成功する）。`.go` の**削除のみ**のコミットも同様に通る（実測: `fmt` は対象ファイルが無くスキップされ、`lint` は新規指摘なしで成功する）。

**残る制約: 完全に未ステージのファイルの変更も「HEAD からの差分」に含まれる。** lefthook が未ステージ変更を隠すのは同一ファイル内に staged と unstaged が混在するケースだけで、丸ごと未ステージのファイルは隠されない。そのファイルの変更は HEAD との差分なので `--new-from-rev=HEAD` でも指摘され、しかも `fmt` は `{staged_files}` に入らないそのファイルを直さない。作業中のファイルを切り離したい場合は `git stash --keep-index` を使う。

**全体チェックは CI が担う。** `make lint`（`--new-from-rev` なし）は CI の `lint` ジョブで実行されるため、差分に絞ることで検査が抜け落ちる範囲は CI で塞がれる。

### 実行順は priority で固定する

`parallel: false` は同時実行を止めるだけで（lefthook の既定値でもあるため `lefthook dump` の出力からは消える）、順序は決めない。順序は command 単位の **`priority` フィールド**（lefthook v1.13.6 の `internal/config/command.go`）が名前比較より優先して評価され、`priority` が同じものの間で `commands` のキー名の比較になる。名前比較は数値プレフィックスを数値順に扱うため厳密な辞書順でもない。

そのため `fmt` = 1 / `lint` = 2 / `linterly` = 3 と **`priority` を明示する**。`fmt` の整形結果を `lint` が読むという依存関係を、command のキー名に依存せず固定できる（実測: `priority` 未指定のときに `fmt` を `zfmt` にリネームすると実行順が `lint` → `linterly` → `zfmt` に変わり、整形前のコードを読んだ `lint` が gofmt 違反で先に落ちた）。

### lefthook のバージョンは go.mod に固定する

**`lefthook: go tool lefthook` を明示する。** lefthook が生成する hook スクリプトの探索順は `$LEFTHOOK_BIN` → 設定の `lefthook` → **PATH の `lefthook`** → node_modules → `go tool lefthook` である。この指定が無いと、開発マシンに `lefthook` がグローバルインストールされている場合に `go.mod` でピン留めしたバージョンではなくそちらが実行される（実測: PATH に v2.1.6 がある環境で hook のバナーが `lefthook v2.1.6` になった。指定後は `lefthook v1.13.6`）。[開発ツールのバージョン管理](#開発ツールのバージョン管理)の方針を hook 実行時にも効かせるための指定である。`min_version` は最小バージョンしか強制できないため代わりにはならない。

**`lefthook:` を変更したら `make hooks` を再実行する。** この値は hook スクリプトの生成時に埋め込まれるため、設定を変えただけでは既存の hook スクリプトに反映されない。

### 運用上の注意

- **`make hooks` は入れ子の git worktree 内では実行しない。** git worktree では `.git` が `gitdir:` 参照のファイルになり、`git rev-parse --git-path hooks` はリポジトリ共有の `<リポジトリルート>/.git/hooks` を返す（実測）。そのため worktree 内での `make hooks` はメインの作業ツリーと将来のすべての worktree に同時に効く。**メインの作業ツリーで一度登録すれば全 worktree に効く**ため、登録はそこで行う。あわせて、`lefthook.yml` を持たないブランチで `make hooks` を実行すると**テンプレートの `lefthook.yml` が生成される**ため、登録はこのファイルがあるブランチで行う。
- `fmt` は `stage_fixed: true` により整形結果を自動で staging に戻す。整形漏れでコミットが失敗する状況を作らない。
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
| 1.3 | 2026-08-21 | `make fmt` / `make fmt-check` の対象をモジュール内パッケージに限定（`$(GO) fmt ./...` / `gofmt -l $$($(GO) list -f '{{.Dir}}' ./...)`）し、Format 節に理由を追記。`.linterlyignore` に `.sweep/` を追加し、冒頭コメントを「手書きの Go ソースコード」に修正。「抑制の方針」に `internal/runner/systemd.go` の G204 暫定抑制を記録 | `gofmt` はパッケージではなくファイルシステムを再帰するため、`.` 指定では作業用の入れ子 git worktree 配下まで対象に含み、`make fmt` が別ブランチのファイルを書き換えていた。`.sweep/spinoff-draft.jsonl` は追記型 JSONL でいずれ上限を超えるが、linterly は `.gitignore` を読まず、かつ `default_excludes` の既定リストにも含まれないため明示除外が必要。仕様に反する G204 抑制がコード側コメントにしか記録されておらず、仕様書だけでは追えなかったため |
| 1.4 | 2026-08-21 | 「Git Hooks」節に既知の制約（pre-commit の `lint` / `linterly` は作業ツリー全体を見る）と `make hooks` の注意（共有 `.git/hooks` への書き込み、`lefthook.yml` の自動生成、既存フックの `*.old` 退避、`prepare-commit-msg` の生成）を追記し、`parallel: false` の説明を「実行順は `commands` のキー名の辞書順で決まる」旨に修正。`.gitignore` に `lefthook-local.yml` と `.claude/worktrees/` を追加 | lefthook を実際に導入して挙動を実測したところ、`fmt` だけが `{staged_files}` にスコープされ `lint` / `linterly` は作業ツリー全体を読むため、無関係な未ステージファイルの整形崩れでコミットが落ちる（かつ `fmt` はそのファイルを直さない）ことを確認した。lefthook が未ステージ変更を隠すのは同一ファイル内に staged と unstaged が混在する場合だけである。また `fmt` を `zfmt` にリネームすると実行順が `lint` → `linterly` → `zfmt` に変わり、`parallel: false` が順序の必要条件にすぎないことを確認した。`git rev-parse --git-path hooks` は worktree からでも共有の `<リポジトリルート>/.git/hooks` を返すため、入れ子 worktree での `make hooks` がメインの作業ツリーに副作用を出す。`lefthook-local.yml` は lefthook 標準のローカル上書きファイルで、置かれた場合に誤コミットされるため。`.claude/worktrees/` は `impl-wt` / `refine` 系スキルが作る作業用 worktree の置き場で、誤コミットを防ぐために追跡から外す（linterly は `.claude/` を `default_excludes` に含むため行数チェックへの影響はない） |
| 1.5 | 2026-08-21 | 「fork からの PR で self-hosted ジョブを起動しない」節を書き換え、`if` 条件を多層防御の 1 層（一次防御は fork PR の承認ポリシーと org runner group の対象リポジトリ限定）と位置づけ、`skipped` が required status check では success 扱いになること・fork PR のマージを機械的に止める場合はゲートジョブが必要なこと・トリガー追加時のガード見直しと許可リスト形への移行候補を追記。「CI/CD」節に初版では `build` ジョブが失敗する旨を追記 | CI を実際に導入して確認したところ、従前の記述は fork ガードの実効性を過大に書いていた。`pull_request` はワークフロー定義をマージコミット側から取るため fork 側で `if:` 行を削除した改変版が実行され得る（`if` は悪意ある第三者に対する境界にならない）。`skipped` は required status check に対して success として報告されるため、fork PR が CI 未実行のまま緑になりマージ可能に見える。承認ポリシーは実測で `first_time_contributors` であり、一度コミットが取り込まれたユーザーは以後承認不要になる。public リポジトリ + self-hosted runner の組み合わせでは、[セキュリティ設計](../architecture/security.md#パスワード不要-sudo-の要求への対応)の言うとおり `NOPASSWD: ALL` 付与時に実質 root を渡すことになるため、防御の位置づけを正確に書く必要があった。あわせて `make build` が `cmd/gsr-helper` 未作成で失敗すること（Issue #3 で解消）を実測で確認したため |
| 1.6 | 2026-08-22 | 「タスクランナー」節の「3 経路から同じターゲットを呼ぶ」を実態（CI と手元は Makefile 経由、Git Hooks はコマンドを直接実行）に修正し、「Git Hooks」節にも同趣旨を追記。CI/CD 節に `concurrency` の説明と、`lint` ジョブの step を個別に列挙する理由を追記。「抑制の方針」に `_test.go` の `errcheck` / `gosec` 除外が方針の明示的な例外であることと、G304 抑制 4 件（`internal/runner/config.go` 3 件・`internal/runner/procs.go` 1 件）を追記。「Linterly」節の除外規則を「`default_excludes` の既定リストに無いものだけを明示する」に正確化し、`language: ja` を説明。ターゲット一覧表の `make fmt-check` 行を実装に同期。セットアップ手順に「Git Hooks」節への参照を追加。`ManagedBy` 関連を `internal/runner/managed_by.go` へ分割したことと Issue #2 のスコープを超えて `internal/runner` を変更した理由を記録。`internal/runner/config.go` の `LoadConfig` の nolint 理由を事前条件に依拠する形へ修正。改訂履歴 1.4 の行に `.claude/worktrees/` を `.gitignore` へ追加した旨と linterly の `default_excludes` に関する補足を追記 | PR レビューで、仕様書の記述が同じ差分で導入した設定ファイルおよびコードと食い違う箇所が指摘されたため。Git Hooks は `fmt` の `{staged_files}` スコープのため make を経由できない（`lint` / `linterly` / `test` を寄せるかは Issue #18）。`.claude/` は linterly の `default_excludes` に含まれるため `.linterlyignore` への追記は不要であることを実測で確認した（`--no-default-excludes` 指定時のみ検出される）。`concurrency` は self-hosted runner の稼働台数と実行中ジョブのキャンセル挙動に直結し、`main` の連続 push で中間コミットの CI 結果が残らない副作用がある（見直しは Issue #16）。`ManagedBy` の切り出しは `discover.go` が 329 行で linterly の `warn` 帯（301〜330 行）に入っていたためで、終了コードは 0 であり `make check` は分割前でも通っていた。「上限に当たった場合は数値を上げるのではなく分割を検討する」方針に従った対応であり、`error`（331 行以上）を避けるための必須対応ではない。Issue #2 の影響範囲は「`internal/` 配下のソースコードは変更しない」としていたが、`.golangci.yml` の導入で gosec 5 件・revive 4 件が出るため、`make check` を通す最小対応としてコメント追加と純粋な移動のみを行った。`LoadConfig` は exported で `dir` を呼び出し側が自由に渡せるため、nolint の理由を無条件の断定から doc コメントの事前条件に依拠する形へ直した |
| 1.7 | 2026-08-22 | 「ディレクトリ構造」の `Makefile` の説明を「CI / 手元で共用。Git Hooks は経由しない」に修正。「抑制の方針」の G304 抑制 4 件の根拠を、`config.go` の 3 件は呼び出し側の事前条件に依拠する条件付きの記述へ、`procs.go` の 1 件は PID の数値検証という別の根拠へ分離し、抑制の棚卸し時に `--max-same-issues=0 --max-issues-per-linter=0` が必要である旨を追記。「Linterly」節に `warning_threshold` をコメントアウトしてはいけない理由を追記。「Git Hooks」節の作業ツリー参照の対処先を Issue #18 と明記し、`parallel: false` の順序説明に lefthook の `priority` フィールドを併記。CI/CD 節の fork ガードの対処先を Issue #17 と明記し、`github.ref` の「（実測）」を仕様に基づく記述へ修正。ターゲット一覧表の `make fmt-check` の注記を「CI / `make check` 用」に修正。改訂履歴 1.3 の `.sweep/` 除外理由と 1.6 の変更内容を本文と整合させた | 2 周目の PR レビューで、1 周目（1.6）の修正が一部の記述に及んでいない・根拠ラベルが実態と合わない・参照先 Issue の番号が欠けている点が指摘されたため。G304 抑制の親記述は、`Discover` の `Options.Roots` により走査ルート自体を呼び出し側が指定できる設計を踏まえると「パスに外部入力が入らない」と無条件に断定できず、コード側（`internal/runner/config.go` の nolint 理由）と食い違っていた。`golangci-lint` は `issues.max-same-issues` / `max-issues-per-linter` の既定（3 / 50）で同種の指摘を打ち切るため、既定のままでは G304 4 件を数え上げられない（設定への `issues` 追加は Issue #15 の範囲）。`.linterly.yml` の `warning_threshold` は既定値と同値だが `rules:` を非空に保つ役割があり、既定値だからという理由でコメントアウトすると `rules section is required` で exit 2 になることを実測した。lefthook v1.13.6 には command 単位の `priority` があり命名に依存せず順序を固定できるため、「命名の維持」は `priority` 未指定である現状の前提にすぎない。本リポジトリの CI run は self-hosted runner に引き取られず `queued` のままでジョブコンテキストが観測されていないため、`github.ref` に「（実測）」と付けるのは根拠ラベルとして誤りだった |
| 1.8 | 2026-08-22 | `make build` を `go build ./...`（全パッケージのコンパイル検証）＋ `cmd/gsr-helper` が存在する場合のみ単一バイナリを生成する形に変更し、ターゲット一覧表・Makefile 定義・CI/CD 節の記述を実装に同期した | PR #19 の CI で `build` ジョブが `stat ./cmd/gsr-helper: directory not found` により exit 2 で失敗した。エントリポイントの実装は Issue #3 のスコープであり Issue #2 では追加できないため、パッケージが未作成の段階でも通り、かつ Issue #3 で `cmd/gsr-helper` が追加された後はそのままバイナリ生成まで行う形に `build` ターゲットを直した |
| 1.9 | 2026-08-22 | `make fmt-check` の対象解決をディレクトリ単位からファイル単位（`go list` の `.GoFiles` / `.CgoFiles` / `.TestGoFiles` / `.XTestGoFiles` / `.IgnoredGoFiles`）へ変更し、`gofmt` を `$(GO) env GOROOT` 由来の `$(GOFMT)` に固定。`go list` の失敗と対象 0 件を `exit 1` にした。ターゲット一覧表・Makefile 定義・「Format」節を実装に同期し、`internal/buildconfig` に Makefile の回帰テストを追加 | 1.3 の修正（`gofmt -l $$($(GO) list -f '{{.Dir}}' ./...)`）は、リポジトリルートに `.go` ファイルが 1 本置かれてルート自体がパッケージになると `gofmt` が `.claude/worktrees/` 配下まで再帰して無効化される。`gofmt -l` は `testdata/` も検査するが `go fmt ./...` は対象外にするため、未整形のフィクスチャを置くと `make fmt` で直せないのに `fmt-check` が落ちるデッドロックになる。`fmt` が GOROOT の `gofmt`、`fmt-check` が PATH の `gofmt` を使う非対称も、`GO` を差し替えた環境で整形と検査のツールチェーンをずらす。さらにコマンド置換の終了ステータスを捨てていたため `go.mod` 破損時に検査が静かに通り、対象 0 件では `gofmt` が引数なしで起動して標準入力を読み無言でハングすることを実測した（`timeout 5 gofmt -l` が exit 124）。Issue #14 |
| 1.10 | 2026-08-22 | fork ガードを全ジョブの `if:` 条件から、GitHub ホストランナー上で動く `guard` ジョブ + `needs: guard` へ置き換え、判定を許可リスト形（`push` と同一リポジトリの `pull_request` だけを許可し、それ以外は失敗）にした。「fork からの PR で self-hosted ジョブを起動しない」節を private + `allow_forking: false` の実態と多層防御の現状表に更新し、required status check には `guard` を指定する運用・実 fork PR での実測が行えない理由を明記。ワークフロー定義のコードブロックを実体と同期し、`internal/buildconfig` に `guard` スクリプトの回帰テストと仕様書コードブロックの同期テストを追加。あわせて `docs/operations/runner-host-setup.md` に「runner group の対象リポジトリ」節を追加（同 1.1） | 従前の `if:` 条件は「`pull_request` でなければ無条件に実行する」ブロックリスト形で、`merge_group` / `workflow_dispatch` を `on:` に足すと左辺が真になって短絡し fork チェックが評価されないまま実行される。また `if:` で skip されたジョブは required status check に対して success として報告されるため、CI が一度も走っていない PR が緑になりマージ可能に見える。skip ではなく失敗するゲートジョブにすれば、この 2 点をワークフロー定義の側で閉じられる。fork PR 承認ポリシーの `all_external_contributors` 化は実測で private リポジトリには設定できず（`fork-pr-contributor-approval` API が 422 `Fork PR approval is not allowed for private repositories.`）、`allow_forking: false` のため実 fork PR での確認経路も存在しないため、public へ戻す場合の手順として記録した。runner group の対象リポジトリ限定は `admin:org` スコープが無く API から確認できない（403）ため、運用手順側へ記録した。Issue #17 |
| 1.11 | 2026-08-22 | `lefthook.yml` の実行モデルを 3 点決着させた。(1) pre-commit の `lint` を `go tool golangci-lint run --new-from-rev=HEAD` にして HEAD からの差分だけを対象にし、残る制約（丸ごと未ステージのファイルは対象に含まれる）と全体チェックを CI が担うことを明記。(2) `lefthook: go tool lefthook` を追加して hook 実行時のバージョンを `go.mod` に固定。(3) make を経由するかどうかの基準を表にし、`linterly` / `test` を `make linterly` / `make test` へ寄せた。あわせて `priority: 1/2/3` を明示して実行順を命名から切り離し、「Git Hooks」節を小見出しに整理。「タスクランナー」節と「ディレクトリ構造」の記述を実態に同期 | (1) 作業ツリー全体を無条件に検査すると、コミット済みの既存指摘が 1 件あるだけで無関係でクリーンなコミットも落ち続ける（実測: 既存コミットに `errcheck` 違反を 1 件入れると別ファイルのクリーンなコミットが失敗し、`--new-from-rev=HEAD` では成功した。`.go` の削除のみのコミットも通るようになった）。(2) hook スクリプトの探索順は PATH 上の `lefthook` が `go tool lefthook` より先であり、グローバルインストールがある環境ではピン留めが効かない（実測: 指定前は hook のバナーが `lefthook v2.1.6`、指定後は `lefthook v1.13.6`）。この値は hook 生成時に埋め込まれるため変更後は `make hooks` の再実行が必要である。(3) `fmt` は `{staged_files}`、`lint` は `--new-from-rev=HEAD` というコミット内容へのスコープが必要で make ターゲットでは表現できないが、`linterly` は `check [path]` がパスを 1 つしか取らずディレクトリ単位の行数上限も全体を見ないと判定できないため絞れず、`test` は絞る必要がない。この 2 つを make 経由にすればコマンド列の二重管理が消え、テストの実行方法を Makefile の 1 箇所で管理できる。`priority` は lefthook v1.13.6 に存在し名前比較より優先されるため、`fmt` → `lint` の依存を命名に頼らず固定できる。サンドボックスの clone で pre-commit（正常・lint 違反でコミット中止・既存違反の無視・削除のみのコミット）と pre-push（失敗テストで push 拒否）の 5 ケースを実測した。Issue #18 |
| 1.12 | 2026-08-22 | `make test` を `go test -race ./...` にし、競合検出を別ターゲットに分けない方針と実測値・pre-push で実行する判断を「テストは常に競合検出付きで実行する」節に記録。「必要なもの」と self-hosted runner の前提に C コンパイラを追加し、`internal/buildconfig` に競合を実際に検出できることの回帰テストを追加。あわせて `docs/operations/runner-host-setup.md` に「C コンパイラ」節を追加（同 1.2） | `internal/runner` の `systemctl show` 並列化以降、複数 goroutine から呼ばれる箇所が増えたのに `make test` に `-race` が無く、CI では競合が検出されないまま緑になる状態だった。ターゲットを分けると CI と手元で競合検出の有無が分岐するため、`make test` 自体に付けて CI・`make check`・pre-push の 3 経路すべてに効かせた。実測でビルドキャッシュあり 1.5 秒 → 21 秒に増えるが、増分のほぼ全部は `internal/exec/command` がテストバイナリ自身を子プロセスとして起動する設計に由来する（競合検出付きバイナリの起動コストが 1 回約 1 秒）。push はコミットより頻度が低いため pre-push では許容し、pre-commit には入れない。`-race` は cgo を必要とするため（実測: `CGO_ENABLED=0 go test -race` が `-race requires cgo`）、開発マシンと runner ホストの前提に C コンパイラを追加した。Issue #21 |
| 1.13 | 2026-08-22 | `.golangci.yml` に `nolintlint`（`allow-unused: false` / `require-explanation: true` / `require-specific: true`）と `issues.max-issues-per-linter: 0` / `max-same-issues: 0` を追加し、`.linterly.yml` に `update_check: false` を追加。「golangci-lint」節に nolintlint の設定表と `issues` の説明を、「Linterly」節に `update_check` の説明を追記し、「抑制の方針」の棚卸し手順を実態（フラグ不要）に更新。`internal/buildconfig` に nolintlint が雑な抑制を実際に落とすことの回帰テストと、両設定ファイルの回帰テストを追加 | 抑制方針「行単位で行い必ず理由を書く」は規約として書かれているだけで機械的な強制が無く、「別 Issue で置き換えたら除去する」と書いた暫定抑制も、不要になった時点で golangci-lint は何も報告しない（既定では不要な `//nolint` を検出しない）。`allow-unused: false` で除去漏れが検出できる。実測で 3 つの設定すべてが機能することを確認した（リンター名なし → `should mention specific linter`、理由なし → `should provide explanation`、不要 → `is unused`）。ツリーに現存する `//nolint` はすべて specific かつ理由付きで、いずれも実際に対象の指摘を抑制しているため `make lint` は exit 0 のまま（本文「抑制の方針」の件数・ファイルパスは PR #25 以降の移動に追随できておらず、その棚卸しは Issue #44 の範囲）。`issues` の 2 項目は既定（50 / 3）だと同じリンター・同じルールの指摘が 4 件以上あると 3 件で打ち切られ、この節が挙げる抑制を出力から裏取りできないため無制限にした。`update_check` は既定 `true` で実行のたびに GitHub Releases へ外向き HTTP が出る（実測: `strace -f -e trace=connect` で `linterly check` の外向き接続が 3 件 → 0 件）。行数上限はデフォルト値のままである。Issue #15 |
| 1.14 | 2026-08-22 | `.golangci.yml` に `depguard` を追加し、`github.com/charmbracelet` 以下の import を理由付きで禁止。「golangci-lint」節にリンター表の行と禁止理由・既知の制約（ブランク import は検出しない）を追記し、`internal/buildconfig` に禁止パスの import が実際に落ちることの回帰テストを追加。あわせて `docs/requirements/non-functional.md` に機械的強制である旨を追記（同 1.7） | [非機能要件](../requirements/non-functional.md#依存ライブラリ)は Charm 4 つを `charm.land/<name>/v2` に揃え `github.com/charmbracelet/*` を直接 import しないと定めているが、強制が無かった。`github.com/charmbracelet/lipgloss v1.1.0` は golangci-lint の推移依存として実際に `go.mod` にあるため、誤って import してもビルドが通ってしまう。bubbletea の v1 / v2 はキー入力 `Msg` の型が異なり、lipgloss のように型が近いものは混ざっても気付きにくい。既存コード（`internal/ui` 配下は `charm.land/*` のみ）は違反 0 件で通る。回帰テストはローカルスタブを `replace` で解決する一時モジュールを使い、ネットワークに出ずに禁止パスの import を再現する。Issue #23 |
| 1.15 | 2026-08-22 | CI ワークフローの運用・セキュリティ設定を 6 点見直した。全ジョブに `timeout-minutes` を設定、`actions/checkout` / `actions/setup-go` をフルコミット SHA でピン留めして `.github/dependabot.yml`（`github-actions`）を追加、`actions/checkout` に `persist-credentials: false` を指定、`setup-go` を `cache: false`、`concurrency` を `${{ github.workflow }}-${{ github.ref }}` + `cancel-in-progress: ${{ github.event_name == 'pull_request' }}` に変更、`lint` ジョブへ `go tool lefthook validate` を追加。あわせて「設定ファイルの不変条件をテストで守る」節を追加し、`internal/buildconfig` に該当する回帰テストを追加 | `timeout-minutes` が無いとハングしたジョブが GitHub 既定の 6 時間 runner を占有し、オンラインの runner が 1 台では CI 全体が止まる。可変タグ参照はタグの付け替えや上流の侵害でコードが差し替わり、self-hosted runner の脅威モデルでは被害が root 相当まで増幅するため SHA へ固定し、追従は Dependabot に委ねた。`persist-credentials` の既定 true は post-job cleanup に依存するが、`cancel-in-progress` によるキャンセルでは完走しない可能性があり、作業ディレクトリを再利用する self-hosted ではトークン残留の窓が開く（CI のどの step も認証付き git 操作を必要としない）。`setup-go` のキャッシュは、モジュール・ビルドキャッシュがホストに残る self-hosted では保存と展開の重複でしかなく、同一ホストで `tar -xf cache.tzst` が D state で 26 分滞留し load average 47 でジョブが cancelled になった実障害がある（`go.sum` 98KB で blob が大きい）。`concurrency` は group がリテラルだとリポジトリ内の全ワークフローで共有され将来相互キャンセルし、`cancel-in-progress: true` を無条件にすると連続マージで未検証の `main` コミットが生まれる。fork ガードの不変条件は `actionlint` のカスタムルールでは書けないため、`make test` に含まれる `internal/buildconfig` の回帰テストで守る（CI 専用 step より早く、pre-push でも効く）。Issue #16 |
| 1.16 | 2026-08-22 | 多層防御表の `guard` 行の「現状」を required status check への指定が未設定である実測に訂正し、ゲートの位置づけを本文でもマージゲートは指定して初めて効く条件付きの記述に揃えた。「抑制の棚卸し」の箇条書きから現存しない `internal/runner/systemd.go` の G204 抑制への言及と件数の断定を外し、同じ断定が残っていた改訂履歴 1.13 の記述も同じ粒度へ直した。`dependabot.yml` のコードブロックを実体と同期 | 実測で `gh api repos/ousiassllc/gsr-helper/branches/main/protection` が 404 `Branch not protected`、`rulesets` が `[]` であり、`guard` を required status check として扱う記述は運用の実態と食い違っていた。抑制の件数・ファイルパスは PR #25 以降の移動に追随できておらず（棚卸しは Issue #44 の範囲）、本文で断定を外した以上、改訂履歴側に「G304 抑制 4 件を裏取りできなかった」という断定を残すと同一版の中で断定と否認が同居する。`dependabot.yml` は仕様書のコードブロックと実体が食い違っており、`internal/buildconfig` の同期テストの対象にも入っていなかった |
| 1.17 | 2026-08-22 | 1.12 の変更理由から、削除済みの `ScanUnits` の名指しを外した | `internal/runner` の再公開面を絞って `Discover` を唯一の入口にしたため、存在しない識別子を指したままになっていた（Issue #42） |
| 1.18 | 2026-08-22 | 「抑制の方針」の棚卸しを現在のツリーに合わせて全面的に更新。除去済みの `internal/runner/systemd.go` の G204 暫定抑制への言及を「方針は満たされている（抑制は `internal/exec/command/command.go` の 1 箇所）」に置き換え、G304 4 件の記述を現存する 9 件すべての表に差し替えた（`internal/runner/procs.go` → `internal/runner/procs/procs.go` の移動、`internal/appconfig` / `internal/audit` の抑制の追加を反映）。G コード別の件数を出力から機械的に検算できない理由と、`_test.go` に実際の抑制指示が無いこと（`internal/buildconfig` に現れる `//nolint` はフィクスチャ文字列とガードテスト自身のコメント・正規表現・メッセージであり、`countNolintInTree` が `_test.go` を除くため棚卸しに影響しない）を明記し、棚卸しが Issue #44 の範囲だとする但し書きを削除 | 記述が PR #25 の移動・分割に追随しておらず、存在しないファイル（`internal/runner/systemd.go` / `internal/runner/procs.go`）と存在しない抑制を指していた。`nolintlint` の `require-specific` はリンター名しか要求しないためツリー内の抑制はすべて裸の `//nolint:gosec` であり、「4 件」という G コード別の数え方は出力から検算できない（Issue #44） |
| 1.19 | 2026-08-22 | 「抑制の方針」の棚卸しの表で `internal/ui/page/actions.go` を `internal/ui/page/action/allow.go` に訂正 | 可否の判定を `ui/page/action` へ分離した際にファイルが移動しており（[TUI コンポーネント設計](../ui/atomic-design.md) 1.12）、棚卸しが存在しないファイルを挙げたままになっていた。`TestSetupDocNolintInventoryMatchesTree` がこのずれを検出した |
| 1.20 | 2026-08-23 | 「抑制の方針」の棚卸しを Logs タブ（Issue #9）の実装後のツリーに合わせた。総数を 10 件から **11 件**（うち G304 が 9 件）に改め、`internal/logs/tail.go` の行を 1 件から 2 件へ更新（`Tail` の最初の `os.Open` と、同名で作り直されたログを開き直す `reopen` の `os.Open`）。「G304 の抑制の安全性の根拠」の 4 分類に件数（5 / 1 / 1 / 2）を書き添え、`internal/logs/tail.go` の 2 件を「事前条件に依拠する根拠」に分類したうえで、`internal/runner/config.go` との違い（パスを組み立てるか丸ごと受け取るか）と `reopen` で追加の根拠が要らない理由（開き直してもパスは変わらない）を明記 | Logs タブの実装で `internal/logs/tail.go` に G304 の抑制が入り、さらにログの入れ替え（inode 変更）への追従で `reopen` の 1 件が加わって計 2 件になった。表の行は追加されていたが件数は 1 のままで、`TestSetupDocNolintInventoryMatchesTree` が落ちる状態だった。**より問題なのは分類の側で**、4 分類の説明は合計 7 件しか扱っておらず、`internal/logs/tail.go` の抑制はどの分類にも属さないまま表にだけ載っていた。この節は「なぜこの抑制が安全か」を後から検算するための唯一の場所であり、分類に載らない抑制は**理由コメントを読む以外に安全性を確かめる手段が無い**。件数を各分類に書き添えたのは、表の合計と分類の合計が一致することを目視で突き合わせられるようにするためで、次に抑制が増えたときの取りこぼしを同じ形で防ぐ |
| 1.21 | 2026-08-23 | 「タスクランナー」節のターゲット一覧表と Makefile のコードブロックに `make run`（`build` 後に生成したバイナリを `ARGS` 付きで起動する）を反映 | Makefile に `run` ターゲットと `ARGS` 変数が追加されたのに仕様書へ反映されておらず、`internal/buildconfig` の同期テスト（`TestSetupDocEmbedsConfigFilesVerbatim`）が失敗したまま既定ブランチに入っていた |
| 1.22 | 2026-08-22 | 「fork からの PR で self-hosted ジョブを起動しない」節を private 前提に更新。public から private へ切り替えた経緯と理由（org runner group が既定で public リポジトリへ runner を提供せず CI が `queued` で止まった）を明記し、脅威モデルの対象をアクセス権を持つ範囲に限定。runner group の層に public 既定の制約を追記 | org レベルに 12 台の runner が登録・1 台稼働している状態でも CI run が 15 分以上 `queued` のまま引き取られず、private 化した直後に同じ run が実行されたことで原因を確定したため。1.5 / 1.7 の記述は public 前提のままで実態と食い違っていた |
| 1.23 | 2026-08-23 | 改訂履歴表の重複した版番号 `1.8` のうち「fork からの PR で self-hosted ジョブを起動しない」節を private 前提に更新した行を `1.22` へ振り直して表の末尾へ移し、`1.7` の行を `1.8`（`make build`）の前へ戻した。あわせて採番の規則（版番号は変更が入った時点で採番するため、日付が版番号の順と一致しないことがある）を表の直後に明記し、`internal/buildconfig` に改訂履歴の版番号が重複せず昇順であることの回帰テストを追加 | 別々の変更に同じ `1.8` が付き、`1.7` が `1.8` の後ろに並んでいたため、表を版番号で引けなかった（Issue #59）。振り直し先に `1.9` 以降を使わず末尾の `1.22` を割り当てたのは、既存行を繰り下げると他の行の変更理由が版番号で参照している箇所（`1.12` / `1.13`）まで書き換えることになり、「変更内容・変更理由は書き換えない」という前提を満たせないためである |
| 1.24 | 2026-08-23 | `internal/buildconfig` の責務を「設定ファイルとドキュメントの不変条件を守る回帰テスト」へ広げ、「設定ファイルの不変条件をテストで守る」節の本文と CI 構成表の「設定の不変条件」行を実態に合わせた。不変条件テスト一覧表に `TestDocRevisionHistoryVersionsUniqueAndAscending` の行を追加 | 1.23 で追加した改訂履歴の回帰テストは `docs/` 配下の Markdown を読むテストであり、「ビルド設定ファイルの回帰テストだけを置く」というパッケージの定義（`internal/buildconfig/doc.go`）と本節の記述の範囲外だった。同パッケージには既に仕様書のコードブロックを検査する `TestSetupDocEmbedsConfigFilesVerbatim` があり、ドキュメントを読むテストは既存の性格の延長であるため、パッケージを分けずに定義側を実態へ追随させた |
| 1.25 | 2026-08-24 | 「設定ファイルの不変条件をテストで守る」の表の直後に、この表が `internal/buildconfig` に置いたものだけを挙げること・同じ性格の検査が `internal/ui/page/pagetest` にもあること・`buildconfig` へ集めるのは**どのパッケージにも属さない取り決め**だけであることを明記した | 表が「網羅ではない」とだけ断っており、**どこまでがこの表の範囲か**が読み取れなかった。共有部品の列挙を守る検査（Issue #130 で 3 本になった）をここへ足すべきか、対象の隣に置くべきかを次の Issue が判断できない。パッケージ固有の不変条件を `buildconfig` へ寄せると、対象を触る Issue が検査の存在に気付けない（Issue #130） |

**版番号は表への追加順ではなく、その変更が入った時点で採番している。** 1.22 の日付が直前の 1.21 より古いのはこのためである。1.22 の行はもともと重複した `1.8` として記録されており（`feat/#1` の取り込み時に 2 つの `1.8` を両方残したまま解消した）、重複を解消する際に、既に使われている 1.9〜1.21 と衝突しない番号として 1.22 を割り当てた。既存行の版番号を繰り下げないのは、他の行の変更理由が版番号で参照している箇所（1.12 / 1.13）まで書き換えることになるためである。

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

package job

import (
	"context"
	"os"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/setup/tarball"
)

// Deps は外部資源をたどるのに要るもの。
type Deps struct {
	// Exec は外部コマンドの実行手段。
	Exec exec.Executor
	// Secrets は取得したトークンの預け先。監査ログとエラー文言のマスクに効く。
	Secrets *gh.Secrets
	// NewClient は API クライアントの生成。nil なら gh のトークンを借りて作る。
	// テストで httptest のサーバへ向けるために差し替える。
	NewClient func(ctx context.Context, d Deps) (*gh.Client, error)
	// Fetch は tarball の取得。nil なら tarball.Fetch。
	Fetch func(ctx context.Context, in tarball.Info, dir string) (string, error)
}

// client は API クライアントを作る。
//
// 借りたトークンは Secrets へ預ける。以後に発行する外部コマンドの監査ログと
// エラー文言から、値一致マスク（exec/mask の段 2）で消えるようにするためである。
func (d Deps) client(ctx context.Context) (*gh.Client, error) {
	if d.NewClient != nil {
		return d.NewClient(ctx, d)
	}

	tok, err := gh.Token(ctx, d.Exec)
	if err != nil {
		return nil, err
	}
	d.Secrets.Add(tok)

	return gh.New(tok)
}

// fetch は tarball を取得する。
func (d Deps) fetch(ctx context.Context, in tarball.Info, dir string) (string, error) {
	if d.Fetch != nil {
		return d.Fetch(ctx, in, dir)
	}
	return tarball.Fetch(ctx, nil, in, dir, nil)
}

// LatestVersion は runner 本体の最新バージョンを返す（FR-20）。
//
// 実行前プレビューに出す値と、実際に展開する tarball を同じ問い合わせの系統から
// 決めるために、計画を組む前にこれを引く。
func LatestVersion(ctx context.Context, d Deps) (string, error) {
	c, err := d.client(ctx)
	if err != nil {
		return "", err
	}
	return c.LatestRunnerVersion(ctx)
}

// PrepPhase は台に紐付かない準備段階に使う Progress.Phase。
const PrepPhase = "tarball 取得"

// PrepIndex は台に紐付かない準備段階を表す Progress.Index。
const PrepIndex = -1

// Input は Run の入力。
type Input struct {
	// Deps は外部資源。
	Deps Deps
	// Plan は実行する計画。
	Plan setup.Plan
	// Scope は tarball の取得情報を引くスコープ。
	Scope scope.Scope
	// Drain はドレイン停止の手段。nil なら svc.Drain。
	Drain func(ctx context.Context, ex exec.Executor, r runner.Runner) error
	// Progress は進捗の通知先。nil 可。
	Progress func(setup.Progress)
}

// Run は短命トークンと tarball を用意してから計画を実行する。
//
// 呼ぶのは承認のあとだけである（FR-16 の流れ）。承認前に呼ぶと、キャンセルした
// 場合にも有効な短命トークンを発行してしまう。
func Run(ctx context.Context, in Input) (setup.Result, error) {
	c, err := in.Deps.client(ctx)
	if err != nil {
		return failed(in.Plan, err), err
	}

	tokens := newTokenCache(c, in.Deps.Secrets)
	defer tokens.forget()

	ball, err := in.tarball(ctx, c)
	if err != nil {
		return failed(in.Plan, err), err
	}
	if ball != "" {
		defer func() { _ = os.Remove(ball) }()
	}

	return setup.Apply(ctx, setup.ApplyInput{
		Exec:     in.Deps.Exec,
		Plan:     in.Plan,
		Token:    "",
		TokenFor: tokens.forPlan(in.Plan, in.Scope),
		Tarball:  ball,
		Drain:    in.Drain,
		Progress: in.Progress,
	})
}

// tarball は必要なら tarball を取得して展開元のパスを返す。不要なら空文字。
func (in Input) tarball(ctx context.Context, c *gh.Client) (string, error) {
	if !in.Plan.NeedsTarball {
		return "", nil
	}

	in.notify(setup.Progress{
		Index: PrepIndex, Total: len(in.Plan.Units), Name: "",
		Phase: PrepPhase, Done: false, Err: nil,
	})

	list, err := c.RunnerDownloads(ctx, in.Scope)
	if err != nil {
		return "", err
	}

	d, err := gh.PickDownload(list, "linux", gh.HostArch())
	if err != nil {
		return "", err
	}

	dir, err := os.MkdirTemp("", "gsr-helper-runner-")
	if err != nil {
		return "", err
	}

	return in.Deps.fetch(ctx, tarball.Info{
		URL: d.URL, Filename: d.Filename, SHA256: d.SHA256,
	}, dir)
}

// notify は進捗を通知する。nil の場合は何もしない。
func (in Input) notify(p setup.Progress) {
	if in.Progress != nil {
		in.Progress(p)
	}
}

// failed は 1 台も着手できなかった場合の結果を組み立てる。
func failed(plan setup.Plan, err error) setup.Result {
	return setup.Result{
		Succeeded: nil, Failed: "", Phase: "", Err: err, Remaining: plan.Names(),
	}
}

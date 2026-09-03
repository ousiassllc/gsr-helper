// Package hostcaps は起動時に 1 回だけホストの能力（root / systemd / docker /
// journal / GitHub トークン）を判定する。
//
// appconfig から分離しているのは次の 2 点による。
//   - 能力判定は外部コマンドを発行する副作用であり、設定ファイルの読み書きとは
//     独立した責務である。設定を読むだけの呼び出し側にプロセス起動の実装を
//     引き込まない。
//   - 判定は起動シーケンス上にあり時間予算を持つ（docs/requirements/
//     non-functional.md の「起動から一覧表示まで 1 秒以内」）。その予算に関する
//     定数と縮退の方針をこのパッケージに閉じる。
package hostcaps

import (
	"bytes"
	"context"
	"os"
	"sync"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/appconfig/confpath"
	"github.com/ousiassllc/gsr-helper/internal/exec"
)

const (
	// defaultProbeTimeout は能力判定 1 コマンドあたりの上限。
	//
	// exec の既定（30 秒）より短くしているのは、能力判定が起動シーケンス上にあり
	// 「起動から一覧表示まで 1 秒以内」（docs/requirements/non-functional.md）を
	// 目標にしているためである。応答しない docker daemon で起動を待たせない。
	defaultProbeTimeout = 500 * time.Millisecond

	// detectBudget は Detect 全体の予算。
	//
	// トークン判定は sudo 経路 → 直接実行の 2 本を逐次で発行しうるため、1 コマンド
	// あたりの上限だけでは合計 1 秒かかり、上記の 1 秒目標に収まらない。
	// 全体にも期限を掛けて超過分は「能力なし」に倒す（縮退動作で扱える）。
	//
	// 予算を 1 秒より内側に取るのは、Detect が cmd/gsr-helper の起動シーケンス上で
	// 同期的に呼ばれるため、この予算がそのまま初回描画までの時間に乗るからである。
	// 本来は先に描画してから能力判定の結果を後から反映する方が正しいが、それには
	// UI 側の変更が必要なので、ここでは予算を目標の内側に収める形で守る。
	detectBudget = 800 * time.Millisecond

	// dockerServerVersionFormat は docker info から daemon の版だけを取り出す指定。
	dockerServerVersionFormat = "{{.ServerVersion}}"
)

// Caps は起動時に 1 回判定する能力。UI の操作可否の判定に使う。
//
// トークンそのものは持たない。値を保持できるフィールドを作らないことで
// 「ファイルにも画面にもログにも残さない」（security.md）を構造として守る。
type Caps struct {
	Root        bool
	Systemd     bool
	Docker      bool
	Journal     bool
	GitHubToken bool
	// SudoUser は SUDO_USER の値。設定・ログの配置先の決定に使う。
	SudoUser string
}

// TokenFunc はトークンを取得できるかを判定する関数。
//
// 取得した値そのものは返さない。呼び出し側に値を渡さないことで、判定のためだけに
// 取り出したトークンが Caps や画面に載る経路を作らない（security.md「保持と出力」）。
//
// timeout は 1 コマンドあたりの上限である。判定はトークンの取得元を優先順に
// 試すため複数のコマンドを逐次で発行しうる。全体の上限（detectBudget）だけでは、
// 1 本目が応答しないときに 2 本目を試す余地が無くなる。
//
// 実装は internal/gh が持つ（gh.HasToken）。取得の優先順を security.md の
// 1 箇所の規定に対して 1 つの実装で保つため、このパッケージは判定手段を
// 持たず注入だけを受ける。
type TokenFunc func(ctx context.Context, ex exec.Executor, timeout time.Duration) bool

// Options は Detect の設定。
type Options struct {
	// HasToken はトークン有無の判定。**nil のときトークン無しとして扱う。**
	//
	// 既定の判定を持たないのは、取得の優先順（security.md「取得の優先順」）の
	// 実装を internal/gh の 1 箇所に閉じるためである。判定手段を渡し忘れた
	// 起動は「認証されていない」として縮退し、追加・削除がグレーアウトする。
	HasToken TokenFunc
	// Timeout は 1 コマンドあたりの上限。0 のとき defaultProbeTimeout。
	// 全体の所要時間は Timeout に関わらず detectBudget で打ち切る。
	Timeout time.Duration
}

// probes は Detect が使う判定手段。テストで差し替える。
type probes struct {
	lookPath func(string) (string, error)
	geteuid  func() int
	getenv   func(string) string
	hasToken TokenFunc
}

// Detect は能力を判定する。
//
// 判定に失敗した能力は「無い」として扱い、エラーを返さない。能力不足は縮退動作で
// 扱う設計（docs/architecture/overview.md の縮退の表）なので、ここで起動を止めない。
func Detect(ctx context.Context, ex exec.Executor, opts Options) Caps {
	p := probes{
		lookPath: exec.LookPath,
		geteuid:  os.Geteuid,
		getenv:   os.Getenv,
		hasToken: opts.HasToken,
	}
	return detect(ctx, ex, p, probeTimeout(opts.Timeout))
}

// probeTimeout は 1 コマンドあたりの上限を決める。0 以下は既定値にする。
func probeTimeout(d time.Duration) time.Duration {
	if d <= 0 {
		return defaultProbeTimeout
	}
	return d
}

// detect は差し替え可能な判定手段を受け取る本体。
//
// 外部コマンドは最大 3 本（docker info と、トークン判定の sudo 経路・直接実行）。
// docker とトークンの判定は互いに独立なので並行実行し、さらに全体に detectBudget の
// 期限を掛ける。逐次だと最悪 3 本分待つことになり、起動時間の目標に収まらない。
func detect(ctx context.Context, ex exec.Executor, p probes, timeout time.Duration) Caps {
	ctx, cancel := context.WithTimeout(ctx, detectBudget)
	defer cancel()

	// 書き込み先が別変数なので競合しない（-race で検証している）。
	var docker, token bool
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		docker = dockerAlive(ctx, ex, p, timeout)
	}()
	go func() {
		defer wg.Done()
		token = detectToken(ctx, ex, p.hasToken, timeout)
	}()
	wg.Wait()

	return Caps{
		Root:        p.geteuid() == 0,
		Systemd:     available(p, "systemctl"),
		Docker:      docker,
		Journal:     available(p, "journalctl"),
		GitHubToken: token,
		SudoUser:    confpath.SudoUserFrom(p.getenv),
	}
}

// detectToken はトークンの有無を判定する。判定手段が無ければ「無い」に倒す。
func detectToken(ctx context.Context, ex exec.Executor, fn TokenFunc, timeout time.Duration) bool {
	if fn == nil {
		return false
	}
	return fn(ctx, ex, timeout)
}

// available は name が PATH 上にあるかを返す。
//
// os/exec を直接 import せず internal/exec 経由で調べる（依存の規則）。
// 関数値 p.lookPath を通すのは、テストでホストの PATH に依存せず差し替えるためである
// （CI は systemd も docker もある self-hosted runner なので実物を見てはいけない）。
func available(p probes, name string) bool {
	_, err := p.lookPath(name)
	return err == nil
}

// dockerAlive は docker が存在し daemon が応答するかを判定する。
//
// docker が無いときはコマンドを発行しない（不在は LookPath で分かるため、
// 監査ログに無駄なレコードを残さない）。
// --format で値を取り出せたことを条件にするのは、docker info が daemon 不応答でも
// 版によって終了コード 0 を返すことがあるためである。
func dockerAlive(ctx context.Context, ex exec.Executor, p probes, timeout time.Duration) bool {
	if !available(p, "docker") {
		return false
	}
	return runProbe(ctx, ex, "caps.docker", timeout, "docker", "info", "--format", dockerServerVersionFormat)
}

// runProbe は能力判定の 1 コマンドを実行し、終了コード 0 かつ標準出力が非空かを返す。
//
// ctx に timeout（既定は defaultProbeTimeout の 500 ms）の期限を掛け直すのは、
// exec の既定 30 秒では起動時間の目標に間に合わないためである。
// 監査ログの action もここで設定する。
func runProbe(
	ctx context.Context,
	ex exec.Executor,
	action string,
	timeout time.Duration,
	name string,
	args ...string,
) bool {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ctx = exec.WithOptions(ctx, exec.Options{Action: action})

	res, err := ex.Run(ctx, name, args...)
	if err != nil {
		return false
	}
	return res.ExitCode == 0 && len(bytes.TrimSpace(res.Stdout)) > 0
}

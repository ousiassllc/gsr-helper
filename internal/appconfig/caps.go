package appconfig

import (
	"bytes"
	"context"
	"os"
	"sync"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/exec"
)

const (
	// defaultProbeTimeout は能力判定 1 コマンドあたりの上限。
	//
	// exec の既定（30 秒）より短くしているのは、能力判定が起動シーケンス上にあり
	// 「起動から一覧表示まで 1 秒以内」（docs/requirements/non-functional.md）を
	// 目標にしているためである。応答しない docker daemon で起動を待たせない。
	defaultProbeTimeout = time.Second

	// detectBudget は Detect 全体の予算。
	//
	// トークン判定は sudo 経路 → 直接実行の 2 本を逐次で発行しうるため、1 コマンド
	// あたりの上限だけでは合計 2 秒かかり、上記の 1 秒目標を大きく超える。
	// 全体にも期限を掛けて超過分は「能力なし」に倒す（縮退動作で扱える）。
	detectBudget = 1500 * time.Millisecond

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

// Options は Detect の設定。
type Options struct {
	// HasToken はトークン有無の判定。nil のとき既定の判定を使う。
	// internal/gh の実装ができたらそれに差し替える。
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
	hasToken := p.hasToken
	if hasToken == nil {
		hasToken = func(c context.Context, e exec.Executor) bool {
			return hasTokenDefault(c, e, p, timeout)
		}
	}

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
		token = hasToken(ctx, ex)
	}()
	wg.Wait()

	return Caps{
		Root:        p.geteuid() == 0,
		Systemd:     available(p, "systemctl"),
		Docker:      docker,
		Journal:     available(p, "journalctl"),
		GitHubToken: token,
		SudoUser:    sudoUserFromEnv(p.getenv),
	}
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
// ctx に defaultProbeTimeout（既定 1 秒）の期限を掛け直すのは、exec の既定 30 秒では
// 起動時間の目標に間に合わないためである。監査ログの action もここで設定する。
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

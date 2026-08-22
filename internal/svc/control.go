package svc

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// 監査ログの action。破壊的操作を記録から落とさないため、どの関数も必ずこのいずれかを
// ctx に載せてから Executor を呼ぶ（docs/architecture/security.md の「監査ログ」）。
//
// **exec.Options.SkipAudit は使わない。** 記録しないことが許されるのは再検出が発行する
// 読み取り専用コマンドだけで、本パッケージが出すのはすべて状態を変えるコマンドである。
const (
	actionStart        = "svc.start"
	actionStop         = "svc.stop"
	actionKill         = "svc.kill"
	actionRestart      = "svc.restart"
	actionEnable       = "svc.enable"
	actionDisable      = "svc.disable"
	actionDaemonReload = "svc.daemon-reload"
)

// ErrNoUnit は systemd ユニット名が分からない runner に systemd 操作を求めたことを表す。
//
// ユニット名が空のまま systemctl を打たない。引数の足りない systemctl は使い方の
// エラーで落ちるだけだが、「対象不明のまま制御を試みた」レコードが監査ログに残り、
// 利用者には終了コードしか返らない。呼ぶ前に弾いて理由を返す。
var ErrNoUnit = errors.New("systemd ユニット名が分からないため操作できません")

// ErrNoKillTarget は強制停止する対象（プロセスもユニットも）が 1 つも無いことを表す。
//
// **1 本もコマンドを発行せずに成功を返さないため**に要る。Kill は「PID があれば kill、
// ユニット名があれば systemctl stop」の 2 段で、どちらの条件も満たさないと発行が
// 0 本になる。0 本の結果をまとめると errors.Join は nil を返すので、何もしていない
// のに「成功」として報告され、利用者は止まっていない runner を止まったものとして扱う。
// 未稼働かつサービス未インストール（runner.ManagedUnknown）の runner がこれに当たる。
var ErrNoKillTarget = errors.New("強制停止の対象がありません（プロセスもユニットも見つかりません）")

// errExit はコマンドが非ゼロで終了したことを表す。
//
// 実プロセス実装（internal/exec/command）は非ゼロ終了を *command.ExitError として
// error でも返すため、この番兵に当たるのは終了コードだけで失敗を表す Executor である。
// err と ExitCode の両方を見るのは、どちらで失敗を表すかが Executor の実装によって
// 変わりうるためである（internal/runner/systemd の runFailure と同じ理由）。
var errExit = errors.New("コマンドが失敗しました")

// Start は runner のサービスを開始する。
func Start(ctx context.Context, ex exec.Executor, r runner.Runner) error {
	return unitCommand(ctx, ex, r, actionStart, "start")
}

// Stop は runner のサービスを停止する。実行中のジョブは中断されうる。
func Stop(ctx context.Context, ex exec.Executor, r runner.Runner) error {
	return unitCommand(ctx, ex, r, actionStop, "stop")
}

// Restart は runner のサービスを再起動する。
func Restart(ctx context.Context, ex exec.Executor, r runner.Runner) error {
	return unitCommand(ctx, ex, r, actionRestart, "restart")
}

// Enable は runner のサービスを自動起動有効にする。
func Enable(ctx context.Context, ex exec.Executor, r runner.Runner) error {
	return unitCommand(ctx, ex, r, actionEnable, "enable")
}

// Disable は runner のサービスを自動起動無効にする。
func Disable(ctx context.Context, ex exec.Executor, r runner.Runner) error {
	return unitCommand(ctx, ex, r, actionDisable, "disable")
}

// DaemonReload は systemd にユニットファイルの変更を読み直させる（FR-35）。
//
// 対象 runner を取らないのはホスト全体に効く操作だからで、監査ログの runner も空になる。
func DaemonReload(ctx context.Context, ex exec.Executor) error {
	return run(ctx, ex, exec.Options{Action: actionDaemonReload}, "systemctl", "daemon-reload")
}

// Kill は runner のプロセスを強制停止する。実行中のジョブは中断される。
//
// **systemd 管理かどうかに依らず動く。** docs/ui/screens.md の「無効な操作の表示」は
// 判定不能・run.sh 直起動のいずれでも X を塞がない。worker のプロセスに直接作用する
// 操作であり、ユニットの有無とは独立に効くためである。
//
// 手順は 2 段である。
//
//  1. Runner.Listener と Runner.Worker の PID を集めて kill -KILL を 1 回発行する。
//     **systemctl kill を使わない。** 対象を main プロセス以外へ広げるフラグの綴りが
//     systemd のバージョンで変わり（--kill-who / --kill-whom）、既定のままでは main
//     プロセスしか落とせずに worker が生き残るためである。PID が 1 つも無ければ
//     この段は飛ばす。
//  2. ユニット名があれば systemctl stop を続けて発行する。プロセスを落としただけでは
//     systemd 側が「停止した」と記録せず、Restart= が設定されたユニットでは
//     すぐに戻ってくるためである。
//
// **1 が失敗しても 2 は試み、両方の結果をまとめて返す。** プロセスを落とし損ねたことと
// ユニットを止め損ねたことは別の失敗であり、片方で打ち切ると残った方が黙って生き残る。
//
// **どちらの段も対象を持たない場合は ErrNoKillTarget を返す。** 1 本もコマンドを
// 発行せずに成功を返さないためである。発行が 0 本のまま errors.Join(nil...) を返すと
// 何もしていない実行が「成功」として報告され、止まっていない runner を止まったものと
// して扱わせてしまう。
func Kill(ctx context.Context, ex exec.Executor, r runner.Runner) error {
	o := exec.Options{Action: actionKill, Runner: r.Name()}

	pids := killTargets(r)
	if len(pids) == 0 && r.UnitName == "" {
		return fmt.Errorf("%s: %w", r.Name(), ErrNoKillTarget)
	}

	var errs []error
	if len(pids) > 0 {
		errs = append(errs, run(ctx, ex, o, "kill", append([]string{"-KILL"}, pids...)...))
	}
	if r.UnitName != "" {
		errs = append(errs, run(ctx, ex, o, "systemctl", "stop", r.UnitName))
	}
	return errors.Join(errs...)
}

// killTargets は強制停止する PID を 10 進表記で返す。
//
// Listener を先に置くのは、受け口を先に落として新しいジョブが割り当てられる余地を
// 縮めるためである。kill(1) は引数の順にシグナルを送る。
//
// **1 以下の PID は落とす。** kill(1) は 0 を「呼び出し元のプロセスグループ全員」、
// -1 を「送信可能な全プロセス」と解釈するため、検出が PID を取り損ねてゼロ値を
// 返した場合に本ツール自身やホスト全体を巻き込む。PID 1 は init であり runner では
// ないので、同じ検査で弾いておく。
func killTargets(r runner.Runner) []string {
	pids := make([]string, 0, len(r.Workers)+1)
	if r.Listener != nil && r.Listener.PID > 1 {
		pids = append(pids, strconv.Itoa(r.Listener.PID))
	}
	for _, w := range r.Workers {
		if w.PID > 1 {
			pids = append(pids, strconv.Itoa(w.PID))
		}
	}
	return pids
}

// unitCommand は systemctl <verb> <ユニット名> を 1 回発行する。
func unitCommand(ctx context.Context, ex exec.Executor, r runner.Runner, action, verb string) error {
	if r.UnitName == "" {
		return fmt.Errorf("%s: %w", r.Name(), ErrNoUnit)
	}
	o := exec.Options{Action: action, Runner: r.Name()}
	return run(ctx, ex, o, "systemctl", verb, r.UnitName)
}

// run は 1 コマンドを発行し、失敗していれば理由を返す。
//
// 監査ログのメタ情報（どの操作の一部としての実行か・対象 runner）は ctx に載せて渡す
// （exec.Options の doc）。**Options.Dir は設定しない。** systemctl も kill も作業
// ディレクトリに依存せず、消えた runner ディレクトリを指定すると起動そのものが
// 失敗するためである。
func run(ctx context.Context, ex exec.Executor, o exec.Options, name string, args ...string) error {
	res, err := ex.Run(exec.WithOptions(ctx, o), name, args...)
	if ferr := failure(res, err); ferr != nil {
		return fmt.Errorf("%s: %w", cmdline(name, args), ferr)
	}
	return nil
}

// failure は実行結果から失敗の理由を返す。失敗していなければ nil。
//
// 終了コードだけで失敗を表す Executor のために標準エラー出力を添える。原因を読む
// 唯一の手掛かりであり、これを落とすと利用者には数字しか残らない。マスクをかけない
// のは、本パッケージが発行するのが systemctl と kill だけで、引数にも出力にも秘密情報を
// 伴わないためである（マスクの対象は docs/architecture/security.md の「監査ログでの
// マスク」を参照）。
func failure(res exec.Result, err error) error {
	switch {
	case err != nil:
		return err
	case res.ExitCode != 0:
		if s := strings.TrimSpace(string(res.Stderr)); s != "" {
			return fmt.Errorf("%w（終了コード %d）: %s", errExit, res.ExitCode, s)
		}
		return fmt.Errorf("%w（終了コード %d）", errExit, res.ExitCode)
	default:
		return nil
	}
}

// cmdline は失敗したコマンドを 1 行で表す。
func cmdline(name string, args []string) string {
	return strings.Join(append([]string{name}, args...), " ")
}

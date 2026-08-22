package runner

import (
	"context"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner/procs"
	"github.com/ousiassllc/gsr-helper/internal/runner/systemd"
)

// このファイルは下位パッケージ（procs / systemd）の型と入口を runner から再公開する。
// runner は 3 経路（ディスク走査・/proc・systemd）の探索結果を集約するパッケージで
// あり、呼び出し側（UI）は集約結果の Runner だけを扱う。そのため /proc と systemd の
// 下位パッケージを個別に import せずに済むよう、型と入口だけをここに置く。
// パースなどの実装詳細は各下位パッケージに閉じたままにする。
//
// runner 自身のコードは下位パッケージを直接呼ぶ（procs.Scan / systemd.Scan など）。
// ここの別名は外部の呼び出し側のためにある。

// Process は検出した runner プロセス。procs.Process の別名。
type Process = procs.Process

// ProcKind は runner プロセスの種別。procs.Kind の別名。
type ProcKind = procs.Kind

// ProcKind の取り得る値。procs 側の定数をそのまま再公開する。
const (
	ProcListener = procs.Listener // Runner.Listener: ジョブを待ち受ける常駐プロセス
	ProcWorker   = procs.Worker   // Runner.Worker: ジョブ 1 件ごとに起動される
)

// SvcState は systemd ユニットの状態。systemd.State の別名。
type SvcState = systemd.State

// ScanProcesses は /proc を走査して Runner.Listener / Runner.Worker を集める。
func ScanProcesses() ([]Process, error) { return procs.Scan() }

// ScanUnits は actions.runner.* の systemd ユニットとその状態を集める。
// ex が nil のときは systemd を参照せず何も返さない（systemctl が無い環境での縮退）。
func ScanUnits(ctx context.Context, ex exec.Executor) ([]SvcState, []error) {
	return systemd.Scan(ctx, ex)
}

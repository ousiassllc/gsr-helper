package runner

import (
	"github.com/ousiassllc/gsr-helper/internal/runner/procs"
	"github.com/ousiassllc/gsr-helper/internal/runner/systemd"
)

// このファイルは下位パッケージ（procs / systemd）の型だけを runner から再公開する。
// runner は 3 経路（ディスク走査・/proc・systemd）の探索結果を集約するパッケージで
// あり、呼び出し側（UI）は集約結果の Runner / Result だけを扱う。その中に現れる型を
// 名指しするために下位パッケージを個別に import させたくないので、集約結果に現れる
// 型だけをここに置く。パースなどの実装詳細は各下位パッケージに閉じたままにする。
//
// 入口は Discover だけである。下位パッケージの Scan を別名で再公開しないのは、
// 個別の経路を外から呼ぶと 3 経路の突き合わせ（Discover）を通らない結果が生まれ、
// systemd.ErrListUnits の解釈のような Discover 内の判断（discover.go の unitsListed）を
// 呼び出し側が再実装することになるためである。同じ理由で ErrListUnits も再公開しない。
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

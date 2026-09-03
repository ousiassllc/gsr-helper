package runner

import (
	"path/filepath"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/runner/procs"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/runner/systemd"
)

// Runner は 1 つの runner インスタンス。
//
// **コピーはポインタの指す先を共有する。** Svc / Listener は写しても同じ実体を指し、
// Workers も同じ配列を指す。1 周期ぶんの Result は全タブへ同時に配られるため
// （internal/ui の page.StateMsg）、受け取った側が書き換えれば他のタブの表示まで
// 変わる。**Discover の戻り値は読み取り専用として扱い、並べ替えや絞り込みは
// 写しを作ってから行うこと**（App.tabs / organism.Table と同じ約束）。
//
// 周期をまたいだ共有は無い。Discover は毎回 .runner を読み直して新しい実体を
// 組み立てるので、前の周期の Result を持ち続けても後の周期に書き換えられない
// （discover_test.go の TestDiscoverReturnsFreshInstancesEachCycle）。
type Runner struct {
	Dir     string // シンボリックリンク解決済みの絶対パス
	Config  Config
	Scope   scope.Scope
	Version string
	WorkDir string

	// UnitName は <dir>/.service に記録された systemd ユニット名。
	// svc.sh install 済みなら設定される。
	UnitName string
	// RunAsUser は runner を実行するユーザー。特定できなければ空。
	RunAsUser string

	Managed  ManagedBy
	Svc      *systemd.State  // systemd ユニットが対応する場合のみ
	Listener *procs.Process  // 稼働中の Runner.Listener
	Workers  []procs.Process // 実行中ジョブの Runner.Worker
}

// Name は runner 名を返す。.runner が読めていない場合はディレクトリ名。
func (r Runner) Name() string {
	if r.Config.AgentName != "" {
		return r.Config.AgentName
	}
	return filepath.Base(r.Dir)
}

// Running は Listener が稼働しているかを返す。
func (r Runner) Running() bool { return r.Listener != nil }

// Busy はジョブを実行中かを返す。
func (r Runner) Busy() bool { return len(r.Workers) > 0 }

// JobElapsed は実行中ジョブの経過時間を返す。複数ある場合は最も古いものを返す。
// runner 単位の代表値なので、Worker ごとの経過時間は procs.Process.Elapsed を使う。
func (r Runner) JobElapsed() time.Duration { return longestElapsed(r.Workers, time.Now()) }

// longestElapsed は now を基準に最も長い経過時間を返す。時刻を引数で受ける純粋関数に
// して、経過時間の計算だけを固定値で検証できるようにする。起動時刻が取れなかった
// プロセス（ゼロ値）と now より未来のプロセスは 0 として扱い、負の値を返さない。
func longestElapsed(workers []procs.Process, now time.Time) time.Duration {
	var longest time.Duration
	for _, p := range workers {
		if p.Started.IsZero() {
			continue
		}
		if e := now.Sub(p.Started); e > longest {
			longest = e
		}
	}
	return longest
}

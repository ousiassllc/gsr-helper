package runner

import (
	"path/filepath"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// Runner は 1 つの runner インスタンス。
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
	Svc      *SvcState // systemd ユニットが対応する場合のみ
	Listener *Process  // 稼働中の Runner.Listener
	Workers  []Process // 実行中ジョブの Runner.Worker
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
// runner 単位の代表値なので、Worker ごとの経過時間は Process.Elapsed を使う。
func (r Runner) JobElapsed() time.Duration { return longestElapsed(r.Workers, time.Now()) }

// longestElapsed は now を基準に最も長い経過時間を返す。時刻を引数で受ける純粋関数に
// して、経過時間の計算だけを固定値で検証できるようにする。起動時刻が取れなかった
// プロセス（ゼロ値）と now より未来のプロセスは 0 として扱い、負の値を返さない。
func longestElapsed(procs []Process, now time.Time) time.Duration {
	var longest time.Duration
	for _, p := range procs {
		if p.Started.IsZero() {
			continue
		}
		if e := now.Sub(p.Started); e > longest {
			longest = e
		}
	}
	return longest
}

package runner

import (
	"context"
	"fmt"
	"sort"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner/procs"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/runner/systemd"
)

// Options は探索の設定。
type Options struct {
	Roots []string // 追加の走査ルート。既定ルートに追加される
	// Depth はルート配下を掘る深さ。0 以下は未指定として defaultDepth を使う。
	// 「掘らない」を 0 で表せないため、設定値をそのまま渡さないこと。
	// scan_depth: 0 のような設定を既定にするか拒否するかは appconfig 側の責務。
	Depth int
	// Exec は systemd 参照に使う Executor。nil のとき systemd 参照を行わない
	// （systemctl が無い環境の縮退。可否の判定は appconfig の Caps が担う）。
	Exec exec.Executor
}

// Result は探索結果。
type Result struct {
	Runners []Runner
	// OrphanUnits は actions.runner.* ユニットのうち、対応する runner
	// ディレクトリが見つからなかったもの。ディレクトリだけ消した場合に残る。
	OrphanUnits []systemd.State
	// Warnings は探索中の部分的な失敗。1 件の失敗で全体を止めないため集約する。
	Warnings []error
}

// Discover はこのホスト上の runner を探索する。
//
// 収集元は 3 つあり、runner ディレクトリの実パスをキーに突き合わせる。
//  1. ディスク走査（.runner を持つディレクトリ）
//  2. systemd ユニット（WorkingDirectory / .service ファイル）
//  3. 稼働プロセス（Runner.Listener / Runner.Worker の実行ファイルパス）
//
// 2 と 3 からもディレクトリを回収するため、走査ルート外の runner でも
// 動いていれば検出できる。
func Discover(ctx context.Context, opts Options) Result {
	var res Result

	running, err := procs.Scan()
	if err != nil {
		res.Warnings = append(res.Warnings, err)
	}
	for i := range running {
		running[i].Dir = normalizeDir(running[i].Dir)
	}

	units, warns := systemd.Scan(ctx, opts.Exec)
	res.Warnings = append(res.Warnings, warns...)
	for i := range units {
		units[i].WorkingDir = normalizeDir(units[i].WorkingDir)
	}
	// WorkingDirectory 経由の照合は先着優先なので、ユニット名で並べて
	// 紐付け結果と孤児ユニットの順序を決定的にする。
	sort.Slice(units, func(i, j int) bool { return units[i].Unit < units[j].Unit })

	dirs := collectDirs(opts, running, units)

	runners := make([]Runner, 0, len(dirs))
	for _, dir := range dirs {
		cfg, err := LoadConfig(dir)
		if err != nil {
			res.Warnings = append(res.Warnings, err)
			continue
		}
		sc, err := scope.Parse(cfg.GitHubURL)
		if err != nil {
			// どの runner の警告か分かるようディレクトリを添える
			// （LoadConfig の警告と同じ形にする）。
			res.Warnings = append(res.Warnings, fmt.Errorf("%s: %w", dir, err))
		}
		runners = append(runners, Runner{
			Dir:      dir,
			Config:   cfg,
			Scope:    sc,
			Version:  readVersion(dir),
			WorkDir:  resolveWorkDir(dir, cfg.WorkFolder),
			UnitName: readUnitName(dir),
		})
	}

	res.OrphanUnits = attach(runners, running, units)

	// 実行ユーザーは紐付け後に決める。attach を 3 引数の純粋関数に保つため、
	// ユーザー名の解決（NSS 参照）はここに置く。
	lookup := newUserLookup()
	for i := range runners {
		runners[i].RunAsUser = resolveRunAsUser(runners[i], lookup)
	}

	sortRunners(runners)
	res.Runners = runners
	return res
}

// attach は runner にプロセスと systemd ユニットを紐付け、対応する runner が
// 見つからなかったユニットを返す。
// Runner.Dir と procs.Process.Dir / systemd.State.WorkingDir は正規化済みであることを
// 前提とする。
func attach(runners []Runner, running []procs.Process, units []systemd.State) []systemd.State {
	byDir := make(map[string]*Runner, len(runners))
	byUnit := make(map[string]*Runner, len(runners))
	for i := range runners {
		byDir[runners[i].Dir] = &runners[i]
		if u := runners[i].UnitName; u != "" {
			byUnit[u] = &runners[i]
		}
	}

	for _, p := range running {
		if p.Dir == "" {
			continue // 照合キーが無い。byDir[""] を引かないよう先に弾く
		}
		r, ok := byDir[p.Dir]
		if !ok {
			continue
		}
		switch p.Kind {
		case procs.Listener:
			attachListener(r, p)
		case procs.Worker:
			r.Workers = append(r.Workers, p)
		}
	}

	orphans := attachUnits(byUnit, byDir, units)

	for i := range runners {
		// Worker の順序は /proc の読み取り順（辞書順なので "10" < "9"）に依存する。
		// 表示とジョブ経過時間を再現可能にするため PID 昇順に整える。
		workers := runners[i].Workers
		sort.Slice(workers, func(a, b int) bool { return workers[a].PID < workers[b].PID })
		runners[i].Managed = managedBy(runners[i])
	}
	return orphans
}

// attachListener は Listener を紐付ける。再起動の途中などで複数見えた場合は
// 起動時刻が新しい方を採用し、古いプロセスの情報を残さない。
func attachListener(r *Runner, p procs.Process) {
	if r.Listener != nil && !p.Started.After(r.Listener.Started) {
		return
	}
	proc := p
	r.Listener = &proc
}

// attachUnits はユニットを runner に紐付け、孤児ユニットを返す。
// UnitName（.service ファイル）を第一、WorkingDirectory を第二の照合キーと
// するため 2 パスに分ける。1 パスで回すと、あるユニットの WorkingDirectory 一致が
// 別のユニットの UnitName 一致を上書きしうる。
func attachUnits(byUnit, byDir map[string]*Runner, units []systemd.State) []systemd.State {
	matched := make([]bool, len(units))
	for i, u := range units {
		if r, ok := byUnit[u.Unit]; ok {
			st := u
			r.Svc = &st
			matched[i] = true
		}
	}

	var orphans []systemd.State
	for i, u := range units {
		if matched[i] {
			continue
		}
		if u.Load == "" {
			// Load が空なのは systemctl show に失敗したユニット（systemd.Scan が
			// Unit だけ埋めて残すプレースホルダ）。WorkingDirectory が分からない
			// だけで、対応ディレクトリが消えたわけではないので FR-05 の孤児に
			// しない。失敗自体は systemd.Scan が警告として返しているので、
			// ここで二重に報告もしない。
			continue
		}
		r, ok := byDir[u.WorkingDir]
		if u.WorkingDir == "" || !ok {
			orphans = append(orphans, u)
			continue
		}
		if r.Svc != nil {
			// 同じ runner を指すユニットが複数あるだけ。ディレクトリは
			// 見つかっているので FR-05 の孤児（対応ディレクトリなし）ではない。
			continue
		}
		st := u
		r.Svc = &st
	}
	return orphans
}

// sortRunners はスコープ→名前の順に並べる。
func sortRunners(runners []Runner) {
	sort.Slice(runners, func(i, j int) bool {
		si, sj := runners[i].Scope.String(), runners[j].Scope.String()
		if si != sj {
			return si < sj
		}
		return runners[i].Name() < runners[j].Name()
	})
}

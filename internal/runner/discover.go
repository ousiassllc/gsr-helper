package runner

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ManagedBy は runner の起動方式。
type ManagedBy int

const (
	ManagedUnknown    ManagedBy = iota // 停止中でサービス登録もされていない
	ManagedSystemd                     // svc.sh install 済み
	ManagedStandalone                  // run.sh を直接起動している
)

func (m ManagedBy) String() string {
	switch m {
	case ManagedSystemd:
		return "systemd"
	case ManagedStandalone:
		return "run.sh"
	default:
		return "-"
	}
}

// Runner は 1 つの runner インスタンス。
type Runner struct {
	Dir     string // シンボリックリンク解決済みの絶対パス
	Config  Config
	Scope   Scope
	Version string
	WorkDir string

	Managed  ManagedBy
	Svc      *SvcState // systemd ユニットが対応する場合のみ
	Listener *Process  // 稼働中の Runner.Listener
	Workers  []Process // 実行中ジョブの Runner.Worker

	// unitName は <dir>/.service に記録された systemd ユニット名。
	// svc.sh install 済みなら設定される。
	unitName string
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
func (r Runner) JobElapsed() time.Duration {
	var longest time.Duration
	for _, w := range r.Workers {
		if e := w.Elapsed(); e > longest {
			longest = e
		}
	}
	return longest
}

// Options は探索の設定。
type Options struct {
	Roots []string // 追加の走査ルート。既定ルートに追加される
	Depth int      // ルート配下を掘る深さ。0 のとき defaultDepth
}

const defaultDepth = 2

// Result は探索結果。
type Result struct {
	Runners []Runner
	// OrphanUnits は actions.runner.* ユニットのうち、対応する runner
	// ディレクトリが見つからなかったもの。ディレクトリだけ消した場合に残る。
	OrphanUnits []SvcState
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

	procs, err := ScanProcesses()
	if err != nil {
		res.Warnings = append(res.Warnings, err)
	}
	for i := range procs {
		procs[i].Dir = normalizeDir(procs[i].Dir)
	}

	units, err := ScanUnits(ctx)
	if err != nil {
		res.Warnings = append(res.Warnings, err)
	}
	for i := range units {
		units[i].WorkingDir = normalizeDir(units[i].WorkingDir)
	}

	dirs := collectDirs(opts, procs, units)

	runners := make([]Runner, 0, len(dirs))
	for _, dir := range dirs {
		cfg, err := LoadConfig(dir)
		if err != nil {
			res.Warnings = append(res.Warnings, err)
			continue
		}
		scope, err := ParseScope(cfg.GitHubURL)
		if err != nil {
			res.Warnings = append(res.Warnings, err)
		}
		runners = append(runners, Runner{
			Dir:      dir,
			Config:   cfg,
			Scope:    scope,
			Version:  readVersion(dir),
			WorkDir:  resolveWorkDir(dir, cfg.WorkFolder),
			unitName: readUnitName(dir),
		})
	}

	res.OrphanUnits = attach(runners, procs, units)
	sortRunners(runners)
	res.Runners = runners
	return res
}

// attach は runner にプロセスと systemd ユニットを紐付け、対応する runner が
// 見つからなかったユニットを返す。
// Runner.Dir と Process.Dir / SvcState.WorkingDir は正規化済みであることを前提とする。
func attach(runners []Runner, procs []Process, units []SvcState) []SvcState {
	byDir := make(map[string]*Runner, len(runners))
	byUnit := make(map[string]*Runner, len(runners))
	for i := range runners {
		byDir[runners[i].Dir] = &runners[i]
		if u := runners[i].unitName; u != "" {
			byUnit[u] = &runners[i]
		}
	}

	for _, p := range procs {
		r, ok := byDir[p.Dir]
		if !ok {
			continue
		}
		switch p.Kind {
		case ProcListener:
			// Listener は 1 プロセスだが、取りこぼしても古い方を残さないよう上書きする。
			proc := p
			r.Listener = &proc
		case ProcWorker:
			r.Workers = append(r.Workers, p)
		}
	}

	var orphans []SvcState
	for _, u := range units {
		// .service ファイル経由と WorkingDirectory 経由の両方で照合する。
		r, ok := byUnit[u.Unit]
		if !ok {
			r, ok = byDir[u.WorkingDir]
		}
		if !ok {
			orphans = append(orphans, u)
			continue
		}
		st := u
		r.Svc = &st
	}

	for i := range runners {
		runners[i].Managed = managedBy(runners[i])
	}
	return orphans
}

// managedBy は起動方式を判定する。systemd ユニットが無いのにプロセスが
// 動いていれば run.sh 直起動とみなす。
func managedBy(r Runner) ManagedBy {
	switch {
	case r.Svc != nil:
		return ManagedSystemd
	case r.Listener != nil:
		return ManagedStandalone
	default:
		return ManagedUnknown
	}
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

// collectDirs は走査・プロセス・ユニットの各経路から runner ディレクトリを集める。
func collectDirs(opts Options, procs []Process, units []SvcState) []string {
	depth := opts.Depth
	if depth <= 0 {
		depth = defaultDepth
	}

	seen := map[string]bool{}
	var dirs []string
	add := func(dir string) {
		dir = normalizeDir(dir)
		if dir == "" || seen[dir] || !IsRunnerDir(dir) {
			return
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}

	for _, root := range append(DefaultRoots(), opts.Roots...) {
		for _, d := range findRunnerDirs(root, depth) {
			add(d)
		}
	}
	for _, p := range procs {
		add(p.Dir)
	}
	for _, u := range units {
		add(u.WorkingDir)
	}

	sort.Strings(dirs)
	return dirs
}

// defaultRootGlobs は runner の一般的な設置場所。
// 誤検出と走査コストを抑えるため、広すぎるパターンは置かない。
var defaultRootGlobs = []string{
	"/home/*/actions-runner*",
	"/home/*/runners",
	"/root/actions-runner*",
	"/opt/actions-runner*",
	"/opt/runner*",
	"/opt/*/actions-runner*",
	"/srv/actions-runner*",
	"/srv/*/actions-runner*",
	"/var/lib/actions-runner*",
	"/usr/local/actions-runner*",
}

// DefaultRoots は既定の走査ルートを展開して返す。
func DefaultRoots() []string {
	var roots []string
	for _, g := range defaultRootGlobs {
		matches, err := filepath.Glob(g)
		if err != nil {
			continue // パターン不正のみ。実行時には起きない
		}
		roots = append(roots, matches...)
	}
	return roots
}

// findRunnerDirs は root 配下から runner ディレクトリを探す。
// runner ディレクトリを見つけたらその配下は掘らない（_work が巨大になるため）。
func findRunnerDirs(root string, depth int) []string {
	if depth < 0 {
		return nil
	}
	fi, err := os.Stat(root)
	if err != nil || !fi.IsDir() {
		return nil
	}
	if IsRunnerDir(root) {
		return []string{root}
	}
	if depth == 0 {
		return nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var found []string
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "_work" || e.Name() == "_diag" {
			continue
		}
		found = append(found, findRunnerDirs(filepath.Join(root, e.Name()), depth-1)...)
	}
	return found
}

// normalizeDir はディレクトリパスを比較可能な形に正規化する。
func normalizeDir(dir string) string {
	if dir == "" {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	// プロセスの cwd が削除済みの場合 " (deleted)" が付くため解決に失敗する。
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return filepath.Clean(abs)
}

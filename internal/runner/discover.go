package runner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"sort"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner/procs"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/runner/systemd"
)

// Options は探索の設定。
type Options struct {
	Roots []string // 走査ルート。SkipDefaultRoots が false なら既定ルートに追加される
	// SkipDefaultRoots は既定の走査ルート（DefaultRoots）を使わない指定。
	// 既定は false で、FR-01 の既定ルートに Roots を足したものを走査する。
	// true にすると Roots だけを走査する。
	//
	// 既定を「使う」側に置くのは、指定を忘れた呼び出しが runner を見落とす側に
	// 倒れないようにするためである。true にしても FR-02 の補完（稼働プロセスと
	// systemd ユニット由来のディレクトリ回収）は止まらない。走査ルートは
	// 「どこを掘るか」の指定であって、検出全体の範囲ではない。
	SkipDefaultRoots bool
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

// scanProcs は稼働プロセスの収集元。テストが実ホストの /proc に依存せずに
// Discover を通せるよう差し替え可能にしてある。procs 側の scan(root) と同じ理由で
// 非公開にする（外から個別の経路を差し替えられるようにすると、3 経路の突き合わせを
// 通らない結果が生まれる）。
var scanProcs = procs.Scan

// Discover はこのホスト上の runner を探索する。
//
// 収集元は 3 つあり、runner ディレクトリの実パスをキーに突き合わせる。
//  1. ディスク走査（.runner を持つディレクトリ）
//  2. systemd ユニット（WorkingDirectory / .service ファイル）
//  3. 稼働プロセス（Runner.Listener / Runner.Worker の実行ファイルパス）
//
// 2 と 3 からもディレクトリを回収するため、走査ルート外の runner でも
// 動いていれば検出できる。
//
// Options.Exec が nil のときは systemd を参照せず、警告も返さない
// （systemctl が無い環境での縮退）。3 秒ごとのポーリングで同じ警告が積み上がるのを
// 避けるためであり、systemd の可否は起動時に 1 回判定した Caps としてヘッダに出る。
// このとき systemd 由来の情報（Runner.Svc / Result.OrphanUnits）は空になるが、
// それは「ユニットが登録されていない」ことを意味しない。ユニット一覧そのものが
// 取れなかった場合（list-units の失敗。systemd.ErrListUnits で区別する）も同じで、
// Discover はこれを 0 件と読み替えず、起動方式を systemd 管理と断定しない側に倒す
// （unitsListed）。1 ユニットの状態取得（show）の失敗は一覧は取れているので区別し、
// ユニット名だけのプレースホルダとして残す。
// この解釈は Discover の内側で完結するため、呼び出し側が ErrListUnits を見る必要は
// ない。警告は Result.Warnings にまとめて返る。
func Discover(ctx context.Context, opts Options) Result {
	var res Result

	running, err := scanProcs()
	if err != nil {
		res.Warnings = append(res.Warnings, err)
	}
	for i := range running {
		running[i].Dir = normalizeDir(running[i].Dir)
	}
	res.Warnings = append(res.Warnings, missingProcDirWarnings(running)...)

	units, warns := systemd.Scan(ctx, opts.Exec)
	res.Warnings = append(res.Warnings, warns...)
	for i := range units {
		units[i].WorkingDir = normalizeDir(units[i].WorkingDir)
	}
	// WorkingDirectory 経由の照合は先着優先なので、ユニット名で並べて
	// 紐付け結果と孤児ユニットの順序を決定的にする。
	sort.Slice(units, func(i, j int) bool { return units[i].Unit < units[j].Unit })
	// unitsListed は systemd のユニット一覧が取れたか。Executor が無い（systemctl が
	// 無い環境での縮退）か list-units 自体が失敗した場合は、ユニットが紐付かない
	// ことを「登録されていない」と読み替えられない。1 ユニットの show 失敗は一覧は
	// 取れているので含めない（ErrListUnits で区別する）。
	unitsListed := opts.Exec != nil && !slices.ContainsFunc(warns, func(e error) bool {
		return errors.Is(e, systemd.ErrListUnits)
	})

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

	orphans, attachWarns := attach(runners, running, units, unitsListed)
	res.OrphanUnits = orphans
	res.Warnings = append(res.Warnings, attachWarns...)

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

// missingProcDirWarnings は稼働中の runner プロセスのうち、導出した runner
// ディレクトリが実在しないものについての警告を返す。
//
// 稼働したまま runner ディレクトリを削除するとこの状態になる。.runner が読めない
// ので Runners には出せないが（IsRunnerDir が false になり collectDirs が落とす）、
// 稼働中の runner が一覧から黙って消えるのは FR-05 の孤児ユニットと同じ
// 「片側だけ消えた」異常系なので、警告として報告する。
// running の Dir は normalizeDir 済みであることを前提とする。
func missingProcDirWarnings(running []procs.Process) []error {
	seen := map[string]bool{}
	var warns []error
	for _, p := range running {
		// Dir が空なのは exe が bin 配下でなく cwd も読めなかった場合。
		// 「ディレクトリが消えた」ことの根拠にならないので報告しない。
		if p.Dir == "" || seen[p.Dir] {
			continue
		}
		// 実在しないことだけを異常とする。権限不足などの他の失敗は runner
		// ディレクトリが消えた根拠にならないため黙って見送る。
		if _, err := os.Stat(p.Dir); !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		seen[p.Dir] = true
		warns = append(warns, fmt.Errorf(
			"%s: runner ディレクトリが見つかりません。%s（PID %d）が稼働したまま"+
				"ディレクトリが削除された可能性があります", p.Dir, p.Kind, p.PID))
	}
	return warns
}

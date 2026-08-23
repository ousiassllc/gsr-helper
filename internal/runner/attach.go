package runner

import (
	"fmt"
	"sort"

	"github.com/ousiassllc/gsr-helper/internal/runner/procs"
	"github.com/ousiassllc/gsr-helper/internal/runner/systemd"
)

// 3 つの収集元（ディスク走査・systemd ユニット・稼働プロセス）を突き合わせて
// 1 台の Runner に組み上げる部分を集める。Discover 本体（discover.go）から分けて
// いるのは、**変わる理由が違う**ためである。あちらが決めるのは「どこを見るか」
// （走査ルート・縮退の条件）で、こちらが決めるのは「同じ runner とみなす条件」
// （照合キーの優先順位・孤児の判定・起動方式の決め方）である。1 ファイル 300 行の
// 上限（docs/ui/atomic-design.md）に収める際の切れ目もここになる（Issue #109）。

// attach は runner にプロセスと systemd ユニットを紐付け、対応する runner が
// 見つからなかったユニット（FR-05 の孤児）と、紐付けなかったユニットについての
// 警告を返す。
// Runner.Dir と procs.Process.Dir / systemd.State.WorkingDir は正規化済みであることを
// 前提とする。
// unitsListed は systemd のユニット一覧が取れたかどうかで、起動方式の判定に渡す。
func attach(runners []Runner, running []procs.Process, units []systemd.State,
	unitsListed bool,
) ([]systemd.State, []error) {
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

	orphans, warns := attachUnits(byUnit, byDir, units)

	for i := range runners {
		// Worker の順序は /proc の読み取り順（辞書順なので "10" < "9"）に依存する。
		// 表示とジョブ経過時間を再現可能にするため PID 昇順に整える。
		workers := runners[i].Workers
		sort.Slice(workers, func(a, b int) bool { return workers[a].PID < workers[b].PID })
		runners[i].Managed = managedBy(runners[i], unitsListed)
	}
	return orphans, warns
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

// loadNotFound は systemd にユニットの実体が無いことを表す LoadState の値。
// svc.sh uninstall 後に参照だけが残っている場合、list-units --all はこの状態の
// ユニットを返す。
const loadNotFound = "not-found"

// attachUnits はユニットを runner に紐付け、孤児ユニットと警告を返す。
// UnitName（.service ファイル）を第一、WorkingDirectory を第二の照合キーと
// するため 2 パスに分ける。1 パスで回すと、あるユニットの WorkingDirectory 一致が
// 別のユニットの UnitName 一致を上書きしうる。
func attachUnits(byUnit, byDir map[string]*Runner, units []systemd.State) ([]systemd.State, []error) {
	var warns []error
	// handled は扱いの決まったユニット。紐付けたものと、実体が無いため紐付けないと
	// 決めたものの両方を立てて、第二パスの対象から外す。
	handled := make([]bool, len(units))
	for i, u := range units {
		if u.Load == loadNotFound {
			// 実体の無いユニットはどちらの照合キーでも紐付けない。紐付けると
			// svc.sh uninstall 済みの runner が systemd 管理として表示され、
			// 存在しないユニットに対してサービス制御を提示してしまう
			// （RunAsUser の解決も同じ理由で not-found を除いている）。
			// 対応する runner ディレクトリが無いわけではないので FR-05 の孤児
			// にもせず、残骸が黙って消えないよう警告として出す。
			handled[i] = true
			warns = append(warns, fmt.Errorf(
				"%s: systemd にユニットの実体がありません（LoadState=%s）。"+
					"svc.sh uninstall 後に参照だけが残っている可能性があります",
				u.Unit, loadNotFound))
			continue
		}
		if r, ok := byUnit[u.Unit]; ok {
			st := u
			r.Svc = &st
			handled[i] = true
		}
	}

	var orphans []systemd.State
	for i, u := range units {
		if handled[i] {
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
			// 見つかっているので FR-05 の孤児（対応ディレクトリなし）ではないが、
			// 黙って落とすと重複した・古いユニットファイルが UI から見えなく
			// なるため警告として出す。
			warns = append(warns, fmt.Errorf(
				"%s: runner ディレクトリ %s には既にユニット %s が紐付いているため"+
					"無視します。重複した、または古いユニットファイルが残っている"+
					"可能性があります",
				u.Unit, u.WorkingDir, r.Svc.Unit))
			continue
		}
		st := u
		r.Svc = &st
	}
	return orphans, warns
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

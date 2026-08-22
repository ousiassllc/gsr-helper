package svc

import (
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// 確認ダイアログに出す「実行するコマンド全文」の出どころを集める。
//
// 表示のためだけの文字列であっても、組み立てを UI 側に置いてはならない。
// screens.md の確認フローは実行コマンド全文の提示を必須としており、表示と実行の
// 出どころが分かれると、片方だけを直したときに**利用者が承認した内容と実際に走る
// コマンドが食い違う**。承認の意味そのものが失われるため、実行側（control.go）と
// 同じ関数（cmdline / killTargets）から組む。

// Enabled は runner の自動起動が有効かを返す。
//
// OpEnable が enable / disable のどちらになるかの判定をここに置くのは、表示
// （CommandLine）と実行（呼び出し側の分岐）が同じ根拠を使うためである。UnitFileState は
// enabled / disabled / static の 3 値を取り（internal/runner/systemd の State.FileState）、
// **enabled 以外はすべて「有効ではない」として enable 側へ倒す**。static は enable の
// 対象にならないユニットだが、そこで disable を打つと「有効でもないものを無効化した」
// という記録だけが残る。Svc が nil（ユニットの状態を取れていない）場合も同じ扱いにする。
func Enabled(r runner.Runner) bool {
	return r.Svc != nil && r.Svc.FileState == "enabled"
}

// CommandLine は操作が発行するコマンドを実行順に 1 行ずつ返す。
//
// ユニット名が分からない runner では systemctl の行を返さない。実行側も同じ条件で
// 発行せずに ErrNoUnit を返す（unitCommand）ため、出さないことが実態と合う。
//
// OpDrain が返すのは待機の**後**に発行する停止コマンドである。待機そのものは
// コマンドを発行しない（/proc の走査だけである。Drainer.Drain）。ドレイン停止は
// 確認ダイアログを経ないので現状この行を読むのは待機画面ではないが、対象を
// 落とすと「ドレインは何も実行しない」と読めてしまうため揃えてある。
func CommandLine(op Op, r runner.Runner) []string {
	switch op {
	case OpStart:
		return unitLine(r, "start")
	case OpStop, OpDrain:
		return unitLine(r, "stop")
	case OpRestart:
		return unitLine(r, "restart")
	case OpEnable:
		if Enabled(r) {
			return unitLine(r, "disable")
		}
		return unitLine(r, "enable")
	case OpKill:
		return killLines(r)
	default:
		return nil
	}
}

// unitLine は systemctl <verb> <ユニット名> の 1 行を返す。ユニット名が無ければ空。
func unitLine(r runner.Runner, verb string) []string {
	if r.UnitName == "" {
		return nil
	}
	return []string{cmdline("systemctl", []string{verb, r.UnitName})}
}

// killLines は強制停止が発行する 2 段のコマンドを返す（Kill の doc の手順）。
func killLines(r runner.Runner) []string {
	out := make([]string, 0, 2)
	if pids := killTargets(r); len(pids) > 0 {
		out = append(out, cmdline("kill", append([]string{"-KILL"}, pids...)))
	}
	return append(out, unitLine(r, "stop")...)
}

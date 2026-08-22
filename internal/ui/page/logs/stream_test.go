package logs

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	dlogs "github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 購読の開始・切り替え・停止と、`journalctl` の呼び出しを検証する。

// closed はチャネルが制限時間内に閉じたかを返す。
func closed(ch <-chan dlogs.Line) bool {
	deadline := time.After(cmdTimeout)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

// ログへの追記が本文に現れる（FR-24 のライブテール）。
func TestTailPicksUpAppendedLines(t *testing.T) {
	st, r := withLogs(t)
	m := activated(t, st, 3)

	f, err := os.OpenFile(m.target.file.Path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("追記できない: %v", err)
	}
	if _, err := f.WriteString("[ERROR] appended later\n"); err != nil {
		t.Fatalf("追記できない: %v", err)
	}
	_ = f.Close()
	_ = r

	m = pumpUntil(t, m, m.wait(), func(m Model) bool { return len(m.lines) >= 4 })
	if got := m.View().Content; !strings.Contains(got, "appended later") {
		t.Errorf("追記が本文に出ていない:\n%s", got)
	}
}

// J は journalctl の追従へ切り替える。発行するコマンドは exec のテスト実装で検証する
// （受け入れ条件「journalctl 呼び出しが internal/exec のテスト実装で検証されている」）。
func TestJournalUsesExecutor(t *testing.T) {
	st, _ := withLogs(t)
	fake := exec.NewFake()
	fake.SetFunc(func(string, []string) (exec.Result, error) {
		return exec.Result{Stdout: []byte("[ERROR] unit failed\n"), Stderr: nil, ExitCode: 0}, nil
	})
	st.Exec = fake

	m := activated(t, st, 3)

	next, cmd := step(t, m, press("J"))
	if !next.target.journal {
		t.Fatal("journalctl へ切り替わっていない")
	}
	next = pumpUntil(t, next, cmd, func(m Model) bool { return len(m.lines) >= 1 })

	calls := fake.Calls()
	if len(calls) == 0 {
		t.Fatal("journalctl を実行していない")
	}
	want := "journalctl -u actions.runner.foo.build01-1.service -n 200 --no-pager"
	if got := calls[0].String(); got != want {
		t.Errorf("発行コマンド = %q, want %q", got, want)
	}
	if got := next.View().Content; !strings.Contains(got, "unit failed") {
		t.Errorf("journalctl の出力が本文に出ていない:\n%s", got)
	}
	if got := next.header(); !strings.Contains(got, "actions.runner.foo.build01-1.service") {
		t.Errorf("見出し = %q, want ユニット名を含む", got)
	}

	// もう一度押すとログファイルへ戻る。
	back, _ := step(t, next, press("J"))
	if back.target.journal {
		t.Error("2 度目の J でログファイルへ戻っていない")
	}
}

// 対象を切り替えると前の購読は畳まれ、古い購読の行は取り込まない。
func TestSwitchingTargetDropsStaleLines(t *testing.T) {
	st, _ := withLogs(t)
	m := activated(t, st, 3)

	old := m.stream
	rows := m.tbl.Shown(sectionLogs)
	if len(rows) < 2 {
		t.Fatalf("一覧の行数 = %d, want 2 以上（前提が崩れている）", len(rows))
	}

	cmd := m.open(target{runner: rows[1].runner, file: rows[1].file, journal: false})
	if !closed(old.lines) {
		t.Error("前の購読が畳まれていない")
	}

	stale := lineMsg{gen: old.gen, line: dlogs.NewLine("stale"), ok: true}
	next, _ := step(t, m, stale)
	if len(next.lines) != 0 {
		t.Errorf("畳んだ購読の行を取り込んでいる: %v", next.lines)
	}

	next = pumpUntil(t, next, cmd, func(m Model) bool { return len(m.lines) >= 1 })
	if got := next.View().Content; strings.Contains(got, "stale") {
		t.Errorf("古い購読の行が本文に混ざっている:\n%s", got)
	}
}

// 裏へ回ると購読を畳み、状態（対象と行）は保つ（page.DeactivateMsg の doc）。
func TestDeactivateStopsStreamKeepingState(t *testing.T) {
	st, _ := withLogs(t)
	m := activated(t, st, 3)

	lines := m.stream.lines
	name := m.target.file.Path
	next, _ := step(t, m, page.DeactivateMsg{})

	if !closed(lines) {
		t.Error("裏へ回っても購読が畳まれていない")
	}
	if next.active {
		t.Error("裏へ回ったのに前面扱いのままである")
	}
	if next.target.file.Path != name || len(next.lines) == 0 {
		t.Error("裏へ回ったときに状態まで捨てている")
	}

	// 戻ったら張り直す。**そのとき持っている行は捨てる。** dlogs.Tail は購読のたびに
	// 末尾を読み直して再送出するので、捨てないと同じ行が本文に二重に並ぶ。
	//
	// 行数の下限（>= 3）では留まらないのは、捨てていなければ入った時点で条件が成立し、
	// 二重取り込みを 1 手も進まずに見逃すためである。捨てたこと（0 行）を先に確かめ、
	// 取り直したあとに元と同じ行数へ戻ることまで見る。
	want := len(next.lines)
	back, cmd := step(t, next, page.ActivateMsg{})
	if back.stream.lines == nil {
		t.Fatal("前面に戻っても購読を張り直していない")
	}
	if len(back.lines) != 0 {
		t.Fatalf("張り直す前に持っていた行を捨てていない: %d 行", len(back.lines))
	}
	back = pumpUntil(t, back, cmd, func(m Model) bool { return len(m.lines) >= want })
	if len(back.lines) != want {
		t.Errorf("戻ったあとの行数 = %d, want %d", len(back.lines), want)
	}
	seen := make(map[string]bool, len(back.lines))
	for _, l := range back.lines {
		if seen[l.Text] {
			t.Fatalf("同じ行が二重に取り込まれている: %q", l.Text)
		}
		seen[l.Text] = true
	}
}

// 終了の通知でも購読を畳む（裏に居ても届くので畳み損ねを閉じられる）。
func TestShutdownStopsStream(t *testing.T) {
	st, _ := withLogs(t)
	m := activated(t, st, 3)

	lines := m.stream.lines
	if _, cmd := step(t, m, page.ShutdownMsg{}); cmd != nil {
		t.Error("後始末の Cmd を返している（畳みは Update の中で済ませる）")
	}
	if !closed(lines) {
		t.Error("終了しても購読が畳まれていない")
	}
}

// `l` の用件は直近ジョブの Worker ログを開く（screens.md の `l`）。
func TestShowLogMsgOpensLatestWorker(t *testing.T) {
	st, r := withLogs(t)
	m := newTab(t, st)

	next, cmd := step(t, m, page.ShowLogMsg{Runner: r})
	if next.target.file.Name != "Worker_20260821-120433-utc.log" {
		t.Fatalf("開いた対象 = %q, want 直近の Worker ログ", next.target.file.Name)
	}
	if next.focus != focusBody {
		t.Error("`l` で開いたのに本文のペインへ移っていない")
	}
	next = pumpUntil(t, next, cmd, func(m Model) bool { return len(m.lines) >= 3 })
}

// Worker ログが無い runner では対象を変えず、理由を状態行に出す。
func TestShowLogMsgWithoutWorkerLog(t *testing.T) {
	empty := testRunner("build02-1", t.TempDir())
	st := pagetest.State(80, 20, empty)
	m := newTab(t, st)

	next, cmd := step(t, m, page.ShowLogMsg{Runner: empty})
	if !next.target.empty() {
		t.Error("Worker ログが無いのに対象を切り替えた")
	}
	if got := chromeOf(t, cmd).Status; !strings.Contains(got, "Worker") {
		t.Errorf("状態行 = %q, want Worker ログが無い旨", got)
	}
}

// 行は上限を超えたら古い側から捨てる（追従を放置してもメモリが伸び続けない）。
func TestLinesAreCapped(t *testing.T) {
	lines := make([]dlogs.Line, 0, maxLines)
	for range maxLines {
		lines = append(lines, dlogs.NewLine("old"))
	}
	got := appendLine(lines, dlogs.NewLine("new"))

	if len(got) != maxLines {
		t.Fatalf("行数 = %d, want %d", len(got), maxLines)
	}
	if got[len(got)-1].Text != "new" {
		t.Errorf("末尾 = %q, want new", got[len(got)-1].Text)
	}
}

// 購読が失敗して終わると、その理由を取り込んで状態行に出す（waitEnd / endStream）。
//
// 行のチャネルが閉じた合図（lineMsg の ok が偽）から waitEnd を経て endMsg が届くまでを
// まとめて見る。**実際に journalctl を失敗させる形は採らない。** 連続失敗には再試行の縮退が
// 入っており理由が返るまで数秒かかるので、取り込みの検証としては割に合わない。
func TestStreamEndErrorReachesStatus(t *testing.T) {
	st, _ := withLogs(t)
	m := activated(t, st, 3)

	// 本物の購読を畳み、「行のチャネルは閉じ、理由だけが届く」終わり方に差し替える。
	m.stop()
	want := errors.New("ログを読めなくなりました")
	lines, errc := make(chan dlogs.Line), make(chan error, 1)
	close(lines)
	errc <- want
	m.stream = stream{gen: m.stream.gen, cancel: nil, lines: lines, err: errc}

	next, cmd := step(t, m, lineMsg{gen: m.stream.gen, line: dlogs.Line{}, ok: false})
	if cmd == nil {
		t.Fatal("行のチャネルが閉じても終わった理由を待ちに行っていない")
	}
	next = pumpUntil(t, next, cmd, func(m Model) bool { return m.err != nil })

	if !errors.Is(next.err, want) {
		t.Fatalf("取り込んだ理由 = %v, want %v", next.err, want)
	}
	if got := chromeOf(t, next.chrome()).Status; !strings.Contains(got, want.Error()) {
		t.Errorf("状態行 = %q, want 追従が失敗した理由を含む", got)
	}
}

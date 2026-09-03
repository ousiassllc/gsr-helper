package logs

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	dlogs "github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/logs/filerow"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 一覧・見出し・本文の組み立て（FR-23 / FR-25）を検証する。購読の開始と停止は
// stream_test.go、キーの解釈は keys_test.go にある。

// withLogs は `_diag` にログを持つ runner 1 台ぶんの共有状態と runner を返す。
func withLogs(t *testing.T) (page.StateMsg, runner.Runner) {
	t.Helper()

	r := pagetest.DiagRunner("build01-1", t.TempDir())
	base := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	writeLog(t, r, "Runner_20260821-120000-utc.log", "runner log\n", base)
	writeLog(t, r, "Worker_20260821-120433-utc.log",
		"[2026-08-21 12:04:35Z INFO  Worker] Job started\n"+
			"[2026-08-21 12:05:01Z WARN  StepRunner] Step timeout approaching\n"+
			"[2026-08-21 12:05:44Z ERROR JobRunner] Process completed with exit code 1\n",
		base.Add(5*time.Minute))
	return pagetest.State(80, 20, r), r
}

// activated は前面に出て一覧を取り込み、最新のログを開き終えたタブを返す。
//
// 張った購読の後始末は step が仕込む（helper_test.go の track）ので、呼ぶ側は畳まなくてよい。
func activated(t *testing.T, st page.StateMsg, lines int) Model {
	t.Helper()

	m := newTab(t, st)
	next, cmd := step(t, m, page.ActivateMsg{})
	return pumpUntil(t, next, cmd, func(m Model) bool { return len(m.lines) >= lines })
}

// 一覧は runner をまたいで更新時刻の新しい順に並び、サイズを出す（FR-23）。
func TestListsLogsNewestFirstWithSize(t *testing.T) {
	st, _ := withLogs(t)
	m := activated(t, st, 3)

	rows := m.tbl.Shown(filerow.Section)
	if len(rows) != 2 {
		t.Fatalf("一覧の行数 = %d, want 2", len(rows))
	}
	if rows[0].File.Kind != dlogs.KindWorker {
		t.Errorf("先頭 = %q, want 更新時刻が新しい Worker ログ", rows[0].File.Name)
	}

	// 列見出しの "SIZE" は列が出ていることしか示さず、**セルが空でも通る**。実ファイルを
	// stat して、そのバイト表記（一覧が使う atom.Bytes）まで画面に出ていることを見る。
	// 期待値をハードコードせず stat から組み立てるのは、フィクスチャの本文を書き換えても
	// 検証が壊れないようにするためである。
	fi, err := os.Stat(rows[0].File.Path)
	if err != nil {
		t.Fatalf("ログを stat できない: %v", err)
	}

	view := m.View().Content
	want := []string{"build01-1", "Worker_20260821-120433-utc.log", "SIZE", atom.Bytes(fi.Size())}
	for _, w := range want {
		if !strings.Contains(view, w) {
			t.Errorf("画面に %q が無い:\n%s", w, view)
		}
	}
}

// 前面に出た時点で最新のログを開き、本文に行が出る（FR-24）。
func TestOpensNewestLogAndTails(t *testing.T) {
	st, _ := withLogs(t)
	m := activated(t, st, 3)

	if m.target.file.Name != "Worker_20260821-120433-utc.log" {
		t.Fatalf("開いた対象 = %q, want 最新の Worker ログ", m.target.file.Name)
	}
	if got := m.View().Content; !strings.Contains(got, "Process completed with exit code 1") {
		t.Errorf("本文に末尾の行が出ていない:\n%s", got)
	}
}

// 見出しは runner 名・ログ名・追従の状態を並べる（screens.md の Logs タブ）。
func TestHeaderShowsTargetAndFollow(t *testing.T) {
	st, _ := withLogs(t)
	m := activated(t, st, 3)

	got := m.header()
	for _, want := range []string{"build01-1", "Worker_20260821", followOn} {
		if !strings.Contains(got, want) {
			t.Errorf("見出し = %q, want %q を含む", got, want)
		}
	}
}

// ERROR / WARN の行は強調して描く（FR-25）。
//
// **見るのは描いた結果（本文のペイン）である。** styleLine を直に呼ぶだけでは、本文の組み立て
// （content.go）が装飾を捨てて素の行を積むようになっても検証が通ってしまう。期待値を
// molecule.LogLine で組み立てて突き合わせるのは、装飾の有無だけでなく中身（重大度ごとに違う配色）
// まで縛れるためである。本文は viewport なので、高さで切れうる View().Content ではなく body.View()
// を見る。素の行・WARN・ERROR が別の装飾になっていることは最後に別途押さえる。
func TestHighlightsErrorAndWarn(t *testing.T) {
	st, _ := withLogs(t)
	// 色を有効にした配色でのみ装飾の有無を判定できる（pagetest.Styles は色なし）。
	st.Styles = token.NewStyles(true, true)
	m := activated(t, st, 3)

	body := m.body.View()
	for role, text := range map[token.RoleToken]string{
		token.RolePlain: "[2026-08-21 12:04:35Z INFO  Worker] Job started",
		token.RoleWarn:  "[2026-08-21 12:05:01Z WARN  StepRunner] Step timeout approaching",
		token.RoleFail:  "[2026-08-21 12:05:44Z ERROR JobRunner] Process completed with exit code 1",
	} {
		if want := molecule.LogLine(text, role, st.Styles); !strings.Contains(body, want) {
			t.Errorf("本文に %q が無い（本文 = %q）", want, body)
		}
	}

	plain := m.styleLine(dlogs.Line{Text: "info", Level: dlogs.LevelPlain})
	warn := m.styleLine(dlogs.Line{Text: "info", Level: dlogs.LevelWarn})
	fail := m.styleLine(dlogs.Line{Text: "info", Level: dlogs.LevelError})
	if plain != "info" || warn == plain || fail == plain || warn == fail {
		t.Errorf("重大度ごとの装飾が分かれていない（plain=%q warn=%q fail=%q）", plain, warn, fail)
	}
}

// ログが 1 つも無いときは、その旨と開き方を出す（空画面にしない）。
func TestEmptyStateMessages(t *testing.T) {
	r := pagetest.DiagRunner("build01-1", t.TempDir())
	st := pagetest.State(80, 20, r)

	m := newTab(t, st)
	next, cmd := step(t, m, page.ActivateMsg{})
	m = pumpUntil(t, next, cmd, func(m Model) bool { return len(m.tbl.Shown(filerow.Section)) == 0 })

	got := m.View().Content
	if !strings.Contains(got, emptyMessage) {
		t.Errorf("画面に一覧が空である旨が無い:\n%s", got)
	}
	if !strings.Contains(got, noTargetMessage) {
		t.Errorf("画面に本文の案内が無い:\n%s", got)
	}
}

// listsDiag は Cmd の束に `_diag` の列挙が含まれるかを返す。
func listsDiag(t *testing.T, cmd tea.Cmd) bool {
	t.Helper()

	cmds, err := cmdtest.Expand(cmd, cmdtest.CmdTimeout)
	if err != nil {
		t.Fatalf("Cmd の束を展開できない: %v", err)
	}
	for _, c := range cmds {
		if c == nil {
			continue
		}
		tm, isTab := c().(page.TabMsg)
		if !isTab {
			continue
		}
		if _, isFiles := tm.Msg.(filesMsg); isFiles {
			return true
		}
	}
	return false
}

// `_diag` を取り直すのは前面に居る間だけである（FR-24 のファイル追加の検知）。
//
// **両方の向きを 1 つのテストで見る。** 「取り直す」だけを見ると常に列挙する実装が、
// 「裏では列挙しない」だけを見ると一切列挙しない実装が、それぞれ緑のまま通る。
// 裏でも列挙すると、見ていない一覧のために 3 秒ごとに runner 台数ぶんの readdir を出す。
func TestRelistsOnlyWhileActive(t *testing.T) {
	st, _ := withLogs(t)

	back := newTab(t, st)
	if _, cmd := step(t, back, st); listsDiag(t, cmd) {
		t.Error("裏に居るのに `_diag` を列挙している")
	}

	front := activated(t, st, 3)
	if _, cmd := step(t, front, st); !listsDiag(t, cmd) {
		t.Error("前面に居るのに `_diag` を取り直していない")
	}
}

// 本文に配れる高さが尽きるときは一覧を削って本文を残す（render.go の resize）。
//
// 一覧（2 件 + 見出しで 3 行）と見出し 1 行だけで埋まる高さを配ると、削らなければ本文は
// 0 行になって 1 行も読めない。**本文が Logs タブの主役である**ことを、末尾の行が本文の
// ペインに出ていることで押さえる。高さを配り直すのは共有状態を受けたときなので、一覧を
// 取り込んだあとにもう一度 StateMsg を流す。
func TestResizeShrinksListWhenBodyStarves(t *testing.T) {
	st, _ := withLogs(t)
	st.BodyH = 4
	m := activated(t, st, 3)
	m, _ = step(t, m, st)

	if got := m.body.View(); !strings.Contains(got, "Process completed with exit code 1") {
		t.Errorf("一覧を削らずに本文が潰れている（本文 = %q）", got)
	}
}

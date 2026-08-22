package logs

import (
	"strings"
	"testing"
	"time"

	dlogs "github.com/ousiassllc/gsr-helper/internal/logs"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 一覧・見出し・本文の組み立て（FR-23 / FR-25）を検証する。購読の開始と停止は
// stream_test.go、キーの解釈は keys_test.go にある。

// withLogs は `_diag` にログを持つ runner 1 台ぶんの共有状態と runner を返す。
func withLogs(t *testing.T) (page.StateMsg, runner.Runner) {
	t.Helper()

	r := testRunner("build01-1", t.TempDir())
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

	rows := m.tbl.Shown(sectionLogs)
	if len(rows) != 2 {
		t.Fatalf("一覧の行数 = %d, want 2", len(rows))
	}
	if rows[0].file.Kind != dlogs.KindWorker {
		t.Errorf("先頭 = %q, want 更新時刻が新しい Worker ログ", rows[0].file.Name)
	}

	view := m.View().Content
	for _, want := range []string{"build01-1", "Worker_20260821-120433-utc.log", "SIZE"} {
		if !strings.Contains(view, want) {
			t.Errorf("画面に %q が無い:\n%s", want, view)
		}
	}
}

// 前面に出た時点で最新のログを開き、本文に行が出る（FR-24）。
func TestOpensNewestLogAndTails(t *testing.T) {
	st, _ := withLogs(t)
	m := activated(t, st, 3)
	defer step(t, m, page.ShutdownMsg{})

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
func TestHighlightsErrorAndWarn(t *testing.T) {
	st, _ := withLogs(t)
	// 色を有効にした配色でのみ装飾の有無を判定できる（pagetest.Styles は色なし）。
	st.Styles = token.NewStyles(true, true)
	m := activated(t, st, 3)

	plain := m.styleLine(dlogs.Line{Text: "info", Level: dlogs.LevelPlain})
	warn := m.styleLine(dlogs.Line{Text: "info", Level: dlogs.LevelWarn})
	fail := m.styleLine(dlogs.Line{Text: "info", Level: dlogs.LevelError})

	if plain != "info" {
		t.Errorf("通常の行 = %q, want 装飾なし", plain)
	}
	if warn == plain || fail == plain {
		t.Errorf("WARN / ERROR が強調されていない（warn=%q fail=%q）", warn, fail)
	}
	if warn == fail {
		t.Errorf("WARN と ERROR が同じ装飾になっている: %q", warn)
	}
}

// ログが 1 つも無いときは、その旨と開き方を出す（空画面にしない）。
func TestEmptyStateMessages(t *testing.T) {
	r := testRunner("build01-1", t.TempDir())
	st := pagetest.State(80, 20, r)

	m := newTab(t, st)
	next, cmd := step(t, m, page.ActivateMsg{})
	m = pumpUntil(t, next, cmd, func(m Model) bool { return len(m.tbl.Shown(sectionLogs)) == 0 })

	got := m.View().Content
	if !strings.Contains(got, emptyMessage) {
		t.Errorf("画面に一覧が空である旨が無い:\n%s", got)
	}
	if !strings.Contains(got, noTargetMessage) {
		t.Errorf("画面に本文の案内が無い:\n%s", got)
	}
}

// 裏に居る間は `_diag` を列挙し直さない（見ていない一覧のために readdir を出さない）。
func TestDoesNotListWhileInactive(t *testing.T) {
	st, _ := withLogs(t)
	m := newTab(t, st)

	_, cmd := step(t, m, st)
	for _, c := range pagetest.Expand(cmd) {
		if c == nil {
			continue
		}
		if tm, ok := c().(page.TabMsg); ok {
			if _, isFiles := tm.Msg.(filesMsg); isFiles {
				t.Error("裏に居るのに `_diag` を列挙している")
			}
		}
	}
}

// 前面に居る間は共有状態が届くたびに `_diag` を取り直す（FR-24 のファイル追加の検知）。
func TestRelistsWhileActive(t *testing.T) {
	st, _ := withLogs(t)
	m := activated(t, st, 3)

	_, cmd := step(t, m, st)
	found := false
	for _, c := range pagetest.Expand(cmd) {
		if c == nil {
			continue
		}
		if tm, ok := c().(page.TabMsg); ok {
			if _, isFiles := tm.Msg.(filesMsg); isFiles {
				found = true
			}
		}
	}
	if !found {
		t.Error("前面に居るのに `_diag` を取り直していない")
	}
}

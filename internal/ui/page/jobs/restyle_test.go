package jobs_test

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/jobs"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// colorState は色を有効にした共有状態を返す。
//
// jobs_test.go の testState は色を切っている（期待文字列に ANSI 列を混ぜないため）
// が、配色の追随はスタイルが実際に色を吐かないと確かめられない。
func colorState(dark bool) page.StateMsg {
	st := testState(busyRunner("build01-1", 2), busyRunner("build01-7", 1))
	st.Styles = token.NewStyles(dark, true)
	st.Dark = dark
	return st
}

// 背景色が濃色 → 淡色に切り替わると、Jobs タブの一覧も新しい配色で描き直される。
//
// 端末の背景色は tea.BackgroundColorMsg で起動後に届き、切り替わることもある。App は
// token.Styles を作り直して StateMsg で配り直すが、一覧は配色を行へ焼き込むため
// setState が table.Model.Restyle を呼ばないと中身だけが古い明暗のまま残る。周囲の枠
// だけが淡色になり、濃色向けの薄い色が白背景に残って読めなくなる（Issue #28）。
//
// Runners タブと同じ配線が Jobs タブにも要る。片方だけ直すと、タブを切り替えた先で
// 同じ欠陥が出る。
func TestBackgroundColorChangeRestylesJobs(t *testing.T) {
	dark := colorState(true)
	m, _ := jobs.New(1, dark).Update(dark)
	// カーソルを動かしてから切り替える（再スタイルで位置が失われないこと）。
	m, _ = m.Update(press("j"))

	darkCursor := token.NewStyles(true, true).Cursor.Render(token.IconCursor)
	if body := m.View().Content; !strings.Contains(body, darkCursor) {
		t.Fatalf("濃色のカーソルが出ていない（テストの前提が崩れている）:\n%q", body)
	}

	light := colorState(false)
	m, _ = m.Update(light)
	body := m.View().Content

	lightCursor := token.NewStyles(false, true).Cursor.Render(token.IconCursor)
	if !strings.Contains(body, lightCursor) {
		t.Errorf("背景色を切り替えても一覧が淡色の配色にならない:\n%q", body)
	}
	if strings.Contains(body, darkCursor) {
		t.Errorf("濃色向けの配色が一覧に残っている:\n%q", body)
	}
	// 再スタイルで行は失われない。
	for _, want := range []string{"build01-1", "build01-7"} {
		if !strings.Contains(body, want) {
			t.Errorf("再スタイルで %q の行が失われた:\n%q", want, body)
		}
	}
}

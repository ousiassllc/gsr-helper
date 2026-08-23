package disk

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// colorState は色を有効にした共有状態を返す。
//
// 他のテストは色を切っている（期待文字列に ANSI 列を混ぜないため）が、配色の追随は
// スタイルが実際に色を吐かないと確かめられない。
func colorState(dark bool) page.StateMsg {
	st, _ := dockerState()
	st.Styles = token.NewStyles(dark, true)
	st.Dark = dark
	return st
}

// 背景色が濃色 → 淡色に切り替わると、Disk タブの一覧も新しい配色で描き直される。
//
// 端末の背景色は tea.BackgroundColorMsg で起動後に届き、切り替わることもある。App は
// token.Styles を作り直して StateMsg で配り直すが、一覧は配色を行へ焼き込むため
// setState が table.Model.Restyle を呼ばないと中身だけが古い明暗のまま残る。周囲の枠
// だけが淡色になり、濃色向けの薄い色が白背景に残って読めなくなる（Issue #28）。
func TestBackgroundColorChangeRestylesList(t *testing.T) {
	dark := colorState(true)
	m := activate(t, newModel(t, dark))
	m, _ = send(t, m, press("space"))

	darkCursor := token.NewStyles(true, true).Cursor.Render(token.IconCursor)
	if body := m.View().Content; !strings.Contains(body, darkCursor) {
		t.Fatalf("濃色のカーソルが出ていない（テストの前提が崩れている）:\n%q", body)
	}

	light := colorState(false)
	m, _ = send(t, m, light)
	body := m.View().Content

	lightCursor := token.NewStyles(false, true).Cursor.Render(token.IconCursor)
	if !strings.Contains(body, lightCursor) {
		t.Errorf("背景色を切り替えても一覧が淡色の配色にならない:\n%q", body)
	}
	if strings.Contains(body, darkCursor) {
		t.Errorf("濃色向けの配色が一覧に残っている:\n%q", body)
	}

	// 3 秒ごとに届く共有状態で、行も選択も失われない。
	if !strings.Contains(body, dockerCacheLabel) {
		t.Errorf("再スタイルで行が失われた:\n%q", body)
	}
	if n := len(m.tbl.Checked()); n != 1 {
		t.Errorf("再スタイルで選択が失われた（%d 件）", n)
	}
}

// 共有状態を受け直しても集計は始まらない。
//
// 共有状態は 3 秒ごとに全タブへ配られる。ここで集計を張ると、走査が周期ごとに
// 積み上がって runner のディスクを読み続けることになる。集計の起点はタブの寿命
// （page.ActivateMsg）と r だけである。
func TestStateMsgDoesNotStartScan(t *testing.T) {
	st, fake := dockerState()
	m := newModel(t, st)
	m, _ = send(t, m, st)

	if m.scan != nil {
		t.Error("共有状態で集計が始まっている")
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Errorf("共有状態で外部コマンドが発行されている（%v）", calls)
	}
}

// r は自分の再集計を起こしたうえで、親へも差し戻す。
//
// 差し戻さないと Disk タブに居る間だけ runner の再検出が止まる。二重解釈ではなく
// 「両方が自分の再読み込みを行う」のが正しい（screens.md の Disk タブのキーマップ）。
func TestRefreshRescansAndBubbles(t *testing.T) {
	st, _ := baseState()
	m := activate(t, newModel(t, st))

	next, cmd := m.Update(press("r"))
	if as(t, next).scan == nil {
		t.Error("r で再集計が始まっていない")
	}
	if _, _, bubbled := pagetest.ScanKey(cmd); !bubbled {
		t.Error("r が親へ差し戻されていない（Disk タブに居る間だけ再検出が止まる）")
	}
}

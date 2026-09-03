package setupmodal

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// testTab は登録に使うタブ番号。0 以外にするのは、包み忘れ（Do を通さない）を
// 「たまたま 0 と一致する」で見逃さないためである。
const testTab = 3

// state は共有状態のスナップショット。色は既定で使わない。
func state() page.StateMsg { return pagetest.State(80, 24) }

// resultOf は Cmd を辿って、page が受けるはずの決定を 1 つ返す。
//
// **タブ番号ごと確かめる。** モーダルの決定は page.Do で包まれて発行元のタブへ
// 戻る（page.AttachMsg の doc）。包み忘れると結果は「そのとき選択中のタブ」へ
// 配られて静かに失われるので、包みの有無まで含めてここで見る。
func resultOf(t *testing.T, cmd tea.Cmd) page.ResultMsg {
	t.Helper()

	if cmd == nil {
		t.Fatal("決定の Cmd が発行されていない")
	}
	for _, msg := range cmdtest.MustMsgs(cmd, cmdtest.CmdTimeout) {
		tab, ok := msg.(page.TabMsg)
		if !ok {
			continue
		}
		res, ok := tab.Msg.(page.ResultMsg)
		if !ok {
			continue
		}
		if tab.Tab != testTab {
			t.Errorf("決定の差し戻し先 = タブ %d, want %d", tab.Tab, testTab)
		}
		return res
	}
	t.Fatal("page.TabMsg に包まれた page.ResultMsg が発行されていない")

	return page.ResultMsg{Kind: "", Msg: nil}
}

// decideVia はキーを 1 打鍵ぶん配り、モーダルが自分宛に包んだ Msg を配り直して、
// page が受ける決定を返す。
//
// **2 段で辿るのが要点である。** 確認ダイアログの決定は「打鍵 → dialog が決定の
// Cmd を返す → モーダルが page.WrapModal で自分宛に包む → 配り直されて初めて
// モーダルが決定として受ける」という往復を通る。1 段目を省いて dialog.DecidedMsg を
// 直に流すと、包み（タブ番号と種類）が壊れていても緑になる（page.WrapModal の doc）。
func decideVia(t *testing.T, o *page.Overlay, k string) page.ResultMsg {
	t.Helper()

	_, cmd := o.Update(pagetest.Press(k))
	if cmd == nil {
		t.Fatalf("%q で Cmd が発行されていない", k)
	}

	var back tea.Cmd
	for _, msg := range cmdtest.MustMsgs(cmd, cmdtest.CmdTimeout) {
		tab, ok := msg.(page.TabMsg)
		if !ok {
			continue
		}
		if tab.Tab != testTab {
			t.Errorf("%q の包みの宛先 = タブ %d, want %d", k, tab.Tab, testTab)
		}
		if mm, ok := tab.Msg.(page.ModalMsg); ok {
			_, back = o.Update(mm)
		}
	}
	if back == nil {
		t.Fatalf("%q の決定がモーダル宛（page.ModalMsg）に包まれていない", k)
	}

	return resultOf(t, back)
}

// modelOf は登録済みモーダルの中身を返す。
func modelOf(t *testing.T, o page.Overlay, kind page.ModalKind) tea.Model {
	t.Helper()

	m, ok := o.Modal(kind)
	if !ok {
		t.Fatalf("%q が登録されていない", kind)
	}
	return m.Model
}

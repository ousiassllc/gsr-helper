package runnerdetail

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 詳細画面で決めた操作は、発行元のタブ番号を載せて page へ戻る（Issue #26）。
//
// 修正前は operation の決定（organism.ChosenMsg）が Overlay 経由で詳細画面自身へ
// 戻り、情報部の viewport で捨てられていた。さらにタブ番号が載らないため、
// タブを切り替えている間に返った結果は「そのとき選択中のタブ」へ配られていた。
func TestChosenReturnsToPageWithOriginTab(t *testing.T) {
	o := newOverlayWithDetail(80, 20)
	Open(&o, pagetest.SampleRunner(), pagetest.Caps())

	chosen := organism.ChosenMsg{Key: "s"}
	o, cmd := o.Update(chosen)
	if cmd == nil {
		t.Fatal("決定が Cmd にならず捨てられている")
	}

	tabbed, ok := cmd().(page.TabMsg)
	if !ok {
		t.Fatalf("決定の包み = %T, want page.TabMsg（タブ番号が載っていない）", cmd())
	}
	if tabbed.Tab != testTab {
		t.Errorf("差し戻し先のタブ = %d, want %d", tabbed.Tab, testTab)
	}

	res, ok := tabbed.Msg.(page.ResultMsg)
	if !ok {
		t.Fatalf("包まれた Msg = %T, want page.ResultMsg", tabbed.Msg)
	}
	if res.Kind != Kind {
		t.Errorf("決定の出どころ = %q, want %q", res.Kind, Kind)
	}
	if got, isChosen := res.Msg.(organism.ChosenMsg); !isChosen || got != chosen {
		t.Errorf("決定の中身 = %+v, want %+v", res.Msg, chosen)
	}

	// 決定は page が解釈する。Overlay へ戻すと詳細画面自身へ帰って捨てられる。
	if o.Handles(res) {
		t.Error("決定が Overlay へ配られている（page に届かない）")
	}
}

// 詳細画面を開いた後に届いた宛先付きの Msg は、閉じた後でも詳細画面へ届く。
//
// 開いた瞬間に処理を始めるモーダル（#9 のログ購読）が結果を回収できるようにする。
func TestAddressedMsgReachesDetailAfterClose(t *testing.T) {
	o := newOverlayWithDetail(80, 20)
	Open(&o, pagetest.SampleRunner(), pagetest.Caps())
	o.Close()
	if o.Active() {
		t.Fatal("閉じていない（前提が崩れている）")
	}

	if !o.Handles(page.ModalMsg{Kind: Kind, Msg: nil}) {
		t.Fatal("閉じた後の宛先付き Msg が捨てられている")
	}

	// 閉じた後に届いた OpenMsg でも対象を差し替えられる（宛先を失っていない）。
	other := pagetest.StandaloneRunner()
	other.Dir = "/opt/runners/late"
	o, _ = o.Update(page.ModalMsg{
		Kind: Kind,
		Msg:  OpenMsg{Runner: other, Caps: pagetest.Caps()},
	})
	if got := detailOf(t, o).Title(); got != other.Name()+"  詳細" {
		t.Errorf("閉じた後に届いた Msg 後の見出し = %q, want %q", got, other.Name()+"  詳細")
	}
}

// tea.Model の型検査（詳細画面のモーダルが Overlay の期待する形であること）。
var _ tea.Model = modal{}

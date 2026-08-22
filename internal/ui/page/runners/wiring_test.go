package runners

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// タブとランタイムのつなぎ（登録が返した Cmd の行き先・寿命の通知・決定の宛先）を
// 検証する。内部テスト（package runners）にしてあるのは、New が登録するモーダルの
// ほかに「Cmd を返すモーダル」を足す必要があり、overlay も initCmd も外からは
// 見えないためである。

// withEcho は Cmd を返すモーダルを 1 枚足したタブと、その中身を返す。
//
// New と同じ形（登録が返した Cmd を initCmd に持つ）にそろえる。runnerdetail は
// 登録時のどの Msg にも Cmd を返さないため、足さないと initCmd が nil になり、
// 経路そのものを検証できない。
func withEcho(t *testing.T, st page.StateMsg) (Model, *pagetest.Echo) {
	t.Helper()

	m := New(0, st)
	echo := pagetest.NewEcho()
	cmd := m.overlay.Register(pagetest.EchoKind, pagetest.EchoModal(echo))
	if cmd == nil {
		t.Fatal("登録が Cmd を返していない（前提が崩れている）")
	}
	m.initCmd = tea.Batch(m.initCmd, cmd)
	return m, echo
}

// step は Msg を 1 つ渡し、Model と Cmd を返す。
func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()

	next, cmd := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update が %T を返した, want runners.Model", next)
	}
	return got, cmd
}

// attached は Msg の並びに、モーダルが page.AttachMsg を受けた痕跡があるかを返す。
func attached(msgs []tea.Msg) bool {
	for _, m := range msgs {
		e, ok := m.(pagetest.EchoMsg)
		if !ok {
			continue
		}
		if _, ok := e.Msg.(page.AttachMsg); ok {
			return true
		}
	}
	return false
}

// 登録が返した Cmd は最初の共有状態で親へ流れる（Issue #26 の受入条件 5）。
//
// **親はタブの Init を呼ばない。** bubbletea が Init を呼ぶのはルート Model
// （ui.App）だけで、App.Init は自分の Cmd しか返さない。Init に持たせたままだと、
// 登録した時点で処理を始めるモーダルはその処理を永久に始められない。
func TestRegisterCmdReachesParentOnFirstState(t *testing.T) {
	st := pagetest.State(80, 16, pagetest.SampleRunner())
	m, _ := withEcho(t, st)

	if cmd := m.Init(); cmd != nil {
		t.Errorf("Init が Cmd を返している（%T）。親は Init を呼ばないため届かない", cmd())
	}

	next, cmd := step(t, m, st)
	if !attached(pagetest.Msgs(cmd)) {
		t.Fatal("登録が返した Cmd が最初の共有状態で流れていない")
	}

	// 共有状態は 3 秒ごとに届く。2 周目以降で流し直さない。
	if _, again := step(t, next, st); attached(pagetest.Msgs(again)) {
		t.Error("登録が返した Cmd が周期ごとに再実行されている")
	}
}

// 寿命の通知は、モーダルを開いたままでも page 本体のものである（Issue #41）。
//
// モーダルへ渡すと、開いたままタブを切り替えた／終了したときに page が長寿命の
// 処理を畳む機会を失い、通知は中身（最終的に viewport）に飲まれて消える。
func TestLifecycleMsgsAreNotEatenByModal(t *testing.T) {
	st := pagetest.State(80, 16, pagetest.SampleRunner())
	m, echo := withEcho(t, st)
	m, _ = step(t, m, st)

	m.overlay.Open(pagetest.EchoKind, nil)
	if !m.overlay.Active() {
		t.Fatal("モーダルが開いていない（前提が崩れている）")
	}

	lifecycle := []tea.Msg{page.DeactivateMsg{}, page.ActivateMsg{}, page.ShutdownMsg{}}
	before := len(echo.Got)
	for _, msg := range lifecycle {
		m, _ = step(t, m, msg)
	}
	if got := echo.Got[before:]; len(got) != 0 {
		t.Errorf("寿命の通知がモーダルへ配られている（%v）", got)
	}
}

// モーダルが返した決定は page が受け取り、Overlay へ転送しない（Issue #26）。
//
// 転送すると決定は発行元のモーダル自身へ戻り、そこで捨てられる。page の Update が
// default（Overlay への転送）より前に case page.ResultMsg を置くことと、
// page.Overlay.Handles が ResultMsg に偽を返すことの両方でこれを守る。
func TestResultMsgDoesNotReturnToModal(t *testing.T) {
	st := pagetest.State(80, 16, pagetest.SampleRunner())
	m, echo := withEcho(t, st)
	m, _ = step(t, m, st)

	m.overlay.Open(pagetest.EchoKind, nil)
	before := len(echo.Got)

	res := page.ResultMsg{Kind: pagetest.EchoKind, Msg: "決定"}
	_, cmd := step(t, m, res)

	if n := pagetest.Received[page.ResultMsg](echo); n != 0 {
		t.Errorf("決定が発行元のモーダルへ戻っている（%d 件）", n)
	}
	if got := echo.Got[before:]; len(got) != 0 {
		t.Errorf("決定を受けてモーダルへ配られた Msg = %v, want 無し", got)
	}

	// page が受けたときに親へ返すのは、モーダルの開閉を伝える ChromeMsg だけである。
	msgs := pagetest.Msgs(cmd)
	if len(msgs) != 1 {
		t.Fatalf("決定を受けて発行された Msg = %v, want ChromeMsg 1 件", msgs)
	}
	if _, ok := msgs[0].(page.ChromeMsg); !ok {
		t.Errorf("決定を受けて発行された Msg = %T, want page.ChromeMsg", msgs[0])
	}
}

// 決定の case は default（Overlay への転送）より前に置く（page.ResultMsg の doc）。
//
// **並びそのものを見るのは、今の実装では振る舞いで区別できないためである。**
// page.Overlay.Handles も ResultMsg に偽を返すので、case を消しても決定はモーダルへ
// 戻らない。二重の守りの片方だけが外れても症状は出ず、この case に分岐を足す後続
// Issue（操作の実行）で初めて決定が発行元のモーダルへ帰るようになる。
func TestResultMsgCaseComesBeforeDefault(t *testing.T) {
	src, err := os.ReadFile("runners.go")
	if err != nil {
		t.Fatalf("実装を読めない: %v", err)
	}

	body := string(src)
	res := strings.Index(body, "\tcase page.ResultMsg:")
	def := strings.Index(body, "\tdefault:\n\t\treturn m.forward(msg)")
	if res < 0 {
		t.Fatal("Update に case page.ResultMsg が無い（決定が Overlay へ転送される）")
	}
	if def < 0 {
		t.Fatal("Update の default（Overlay への転送）が見つからない（前提が崩れている）")
	}
	if res > def {
		t.Error("case page.ResultMsg が default より後にある（決定が発行元のモーダルへ戻る）")
	}
}

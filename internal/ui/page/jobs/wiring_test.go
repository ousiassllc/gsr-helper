package jobs

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// タブとランタイムのつなぎ（登録が返した Cmd の行き先・寿命の通知・決定の宛先）を
// 検証する。理由と作りは runners/wiring_test.go と同じで、タブごとに独立した
// 経路であるため 2 つとも固定する。

// withEcho は Cmd を返すモーダルを 1 枚足したタブと、その中身を返す。
func withEcho(t *testing.T, st page.StateMsg) (Model, *pagetest.Echo) {
	t.Helper()

	m := New(1, st)
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
		t.Fatalf("Update が %T を返した, want jobs.Model", next)
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
// **親はタブの Init を呼ばない。** Init に持たせたままだと、登録した時点で処理を
// 始めるモーダルはその処理を永久に始められない。
func TestRegisterCmdReachesParentOnFirstState(t *testing.T) {
	st := pagetest.State(80, 16, pagetest.BusyRunner())
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
func TestLifecycleMsgsAreNotEatenByModal(t *testing.T) {
	st := pagetest.State(80, 16, pagetest.BusyRunner())
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
func TestResultMsgDoesNotReturnToModal(t *testing.T) {
	st := pagetest.State(80, 16, pagetest.BusyRunner())
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
// 並びそのものを見る理由は runners/wiring_test.go と同じである（今は
// page.Overlay.Handles も偽を返すため、振る舞いでは区別できない）。
func TestResultMsgCaseComesBeforeDefault(t *testing.T) {
	src, err := os.ReadFile("jobs.go")
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

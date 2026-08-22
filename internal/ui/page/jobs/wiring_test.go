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

// モーダルが返した決定は page が受け取り、モーダルにも一覧にも吸われない（Issue #26）。
//
// 守りが二重（page.Overlay.Handles が偽を返すことと page 側の case）であることと、
// 並びに意味が無い理由は runners/wiring_test.go と同じである。
func TestResultMsgDoesNotReturnToModal(t *testing.T) {
	st := pagetest.State(80, 16, pagetest.BusyRunner())
	m, echo := withEcho(t, st)
	m, _ = step(t, m, st)

	m.overlay.Open(pagetest.EchoKind, nil)
	before := len(echo.Got)

	res := page.ResultMsg{Kind: pagetest.EchoKind, Msg: "決定"}
	if m.overlay.Handles(res) {
		t.Error("Overlay が決定を引き受けている（転送すると発行元のモーダルへ戻って捨てられる）")
	}

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

// Update は決定を自分の case で受ける（page.ResultMsg の doc）。
//
// case の有無をソースで見る理由は runners/wiring_test.go と同じである（今は
// page.Overlay.Handles も偽を返すため、振る舞いでは区別できない）。並びは見ない。
func TestUpdateHasResultMsgCase(t *testing.T) {
	src, err := os.ReadFile("jobs.go")
	if err != nil {
		t.Fatalf("実装を読めない: %v", err)
	}
	if !strings.Contains(string(src), "\tcase page.ResultMsg:") {
		t.Error("Update に case page.ResultMsg が無い（決定を解釈するのが page でなくなる）")
	}
}

// New は登録が返した Cmd を initCmd に保持する（Issue #26 の受入条件 5）。
//
// ソースで見る理由は runners/wiring_test.go と同じである（本番で登録するモーダルは
// 登録時に Cmd を返さないため、取り落としても今日は症状が出ない）。
func TestNewKeepsRegisterCmd(t *testing.T) {
	src, err := os.ReadFile("jobs.go")
	if err != nil {
		t.Fatalf("実装を読めない: %v", err)
	}
	if !strings.Contains(string(src), "\t\tinitCmd: cmd,\n") {
		t.Error("New が登録の Cmd を initCmd に保持していない（登録時に始まる処理が動かない）")
	}
}

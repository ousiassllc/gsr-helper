package config

import (
	"os"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/configmodal"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 自身の設定（FR-41 / FR-42）の回帰テスト。
//
// runner の設定編集（regress_test.go）と分けているのは、**編集対象が違う**ためで
// ある。こちらが書くのは gsr-helper 自身の設定ファイル 1 つで、差分の基準も
// 「保存済みの値か、ファイルが無いか」で決まる。1 ファイル 300 行の上限
// （docs/ui/atomic-design.md）に収める際の切れ目もここになる（Issue #108）。

// 自身の設定を保存したあと開き直すと、保存した値が初期値になること（FR-42）。
func TestSelfConfigReopensWithSavedValues(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	st := pagetest.State(80, 24)
	st.Config = page.ConfigDeps{Conf: st.Config.Conf, Path: dir + "/config.yaml", FirstRun: false}
	m := New(tabIndex, st)
	next, _ := m.Update(st)
	m = next.(Model)

	m, _ = send(t, m, organism.ChosenMsg{ID: selfID, Key: ""})
	m.vals.Self.Refresh = "42"
	m, _ = send(t, m, page.ResultMsg{Kind: configmodal.FormKind, Msg: dialog.FormDoneMsg{Form: nil}})

	m, cmd := send(t, m, page.ResultMsg{Kind: configmodal.DiffKind, Msg: dialog.DecidedMsg{Confirmed: true}})
	done, ok := doneOf(cmd)
	if !ok || done.err != nil {
		t.Fatalf("設定の書き込みに失敗: %v", done.err)
	}
	m, _ = send(t, m, done)

	m, _ = send(t, m, organism.ChosenMsg{ID: selfID, Key: ""})
	if m.vals.Self.Refresh != "42" {
		t.Errorf("開き直した初期値 = %q, want 42（保存した値）", m.vals.Self.Refresh)
	}
}

// 初回ウィザードを値を変えずに確定しても設定ファイルが作られること（FR-41）。
//
// ウィザードの初期値は既定値そのものなので、素通しの確定は保存済みの内容と
// 一致する。差分なしで書き込みを飛ばすとファイルが作られず、次回起動も
// FirstRun のままでウィザードが毎回出続ける。
func TestFirstRunWizardWritesWithoutEdits(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/config.yaml"
	st := pagetest.State(80, 24)
	m := New(tabIndex, st)

	// 実環境の初回起動と同じ基準にする。appconfig.Load はファイルが無ければ
	// Default() を返し、ウィザードの初期値もそこから作られるため、素通しの
	// 確定は往復して同じ値になる（pagetest の Conf はゼロ値で往復しない）。
	st.Config = page.ConfigDeps{Conf: appconfig.Default(), Path: path, FirstRun: true}
	next, cmd := m.Update(st)
	m = pagetest.Advance(next, cmd, 5).(Model)
	if !m.self {
		t.Fatal("初回設定ウィザードが開いていない")
	}

	// 値を触らずに確定する。
	m, _ = send(t, m, page.ResultMsg{Kind: configmodal.FormKind, Msg: dialog.FormDoneMsg{Form: nil}})

	m, cmd = send(t, m, page.ResultMsg{Kind: configmodal.DiffKind, Msg: dialog.DecidedMsg{Confirmed: true}})
	done, ok := doneOf(cmd)
	if !ok {
		t.Fatalf("書き込みの Cmd が出ていない（差分なしで飛ばされた）: %s", view(m))
	}
	if done.err != nil {
		t.Fatalf("設定の書き込みに失敗: %v", done.err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("設定ファイルが作られていない: %v", err)
	}
}

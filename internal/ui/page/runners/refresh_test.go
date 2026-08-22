package runners_test

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runners"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// StateMsg を受け直したときに一覧が保つもの・作り直すものを確かめる。
//
// 共有状態は 3 秒ごとに配られ、そのたびに配色と行の両方が入れ替わりうる。配色は
// 届いた値へ追随させ（背景色は起動後に届き切り替わることもある）、カーソルは
// 識別子で貼り直す。この 2 つは同じ setState の中で起きるため 1 ファイルにまとめる。

// 背景色が濃色 → 淡色に切り替わると、一覧の中身も新しい配色で描き直される。
//
// 端末の背景色は tea.BackgroundColorMsg で後から届くため、App は token.Styles を
// 作り直して StateMsg で配り直す。一覧が受け取った配色を捨てると、周囲の枠だけが
// 淡色になり、中身は濃色向けの薄い色のまま白背景に残って読めなくなる
// （token/state.go の palette が 2 型を持つ理由そのもの）。
func TestBackgroundColorChangeRestylesList(t *testing.T) {
	darkState := testState(80, 16)
	darkState.Styles = token.NewStyles(true, true)
	darkState.Dark = true

	m, _ := runners.New(0, darkState).Update(darkState)
	// カーソル位置と選択を作ってから配色を切り替える（再スタイルで失われないこと）。
	m, _ = send(t, m, "j", " ")
	darkBody := m.View().Content

	darkCursor := token.NewStyles(true, true).Cursor.Render(token.IconCursor)
	if !strings.Contains(darkBody, darkCursor) {
		t.Fatalf("濃色のカーソルが出ていない（テストの前提が崩れている）:\n%q", darkBody)
	}

	lightState := testState(80, 16)
	lightState.Styles = token.NewStyles(false, true)
	lightState.Dark = false
	m, _ = m.Update(lightState)
	lightBody := m.View().Content

	lightCursor := token.NewStyles(false, true).Cursor.Render(token.IconCursor)
	if !strings.Contains(lightBody, lightCursor) {
		t.Errorf("背景色を切り替えても一覧が淡色の配色にならない:\n%q", lightBody)
	}
	if strings.Contains(lightBody, darkCursor) {
		t.Errorf("濃色向けの配色が一覧に残っている:\n%q", lightBody)
	}

	// スクロール位置・選択・絞り込みは再スタイルで失われない。
	if !strings.Contains(lightBody, "build01-2") || !strings.Contains(lightBody, "build01-1") {
		t.Errorf("再スタイルで行が失われた:\n%q", lightBody)
	}
}

// 自動更新でカーソルより上の runner が消えても、enter が開く詳細は同じ runner のまま。
//
// 3 秒ごとの再検出は runner が 1 台消えるだけで並びを詰める。カーソルを生の添字で
// 当て直すと選択が 1 つ下へずれ、利用者が選んだつもりの runner とは別の runner の
// 詳細が開く。サービス制御を実装した時点で「選んだつもりとは別の runner を停止する」
// に化けるため、一覧の側で識別子ごとに貼り直す。
func TestAutoRefreshKeepsSelectedRunner(t *testing.T) {
	st := testState(80, 16)
	st.Result.Runners = []runner.Runner{
		sampleRunner("build01-1", false),
		sampleRunner("build01-2", true),
		sampleRunner("build01-3", false),
	}
	m, _ := runners.New(0, st).Update(st)

	// カーソルを 2 台目へ置く。
	m, _ = send(t, m, "j")

	// 再検出で 1 台目が消える（並びが 1 つ詰まる）。
	next := st
	next.Result.Runners = []runner.Runner{
		sampleRunner("build01-2", true),
		sampleRunner("build01-3", false),
	}
	m, _ = m.Update(next)

	m, c := send(t, m, "enter")
	if !c.Modal {
		t.Fatal("enter で詳細画面が開かない")
	}
	body := m.View().Content
	if !strings.Contains(body, "build01-2") {
		t.Errorf("選んでいた runner とは別の詳細が開いた:\n%s", body)
	}
	if strings.Contains(body, "build01-3") {
		t.Errorf("1 つ下の runner の詳細が開いている:\n%s", body)
	}
}

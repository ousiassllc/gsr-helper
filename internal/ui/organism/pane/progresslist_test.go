package pane_test

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/pane"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// barFill は進捗バーの塗り。バーが出ているかの判定に使う。
const barFill = "█"

// setupRows は screens.md の Setup タブのモックの 3 行を返す。
func setupRows() []molecule.ProgressView {
	return []molecule.ProgressView{
		{Name: "build01-5", State: molecule.ProgressDone, Detail: "登録・サービス起動 完了"},
		{Name: "build01-6", State: molecule.ProgressRunning, Detail: "config.sh 実行中…"},
		{Name: "build01-7", State: molecule.ProgressWaiting, Detail: "待機"},
	}
}

// newList は大きさと中身を与えた進捗表示を返す。
func newList(in pane.ProgressInput, w, h int) pane.ProgressList {
	p := pane.NewProgressList(testStyles())
	p.SetSize(w, h)
	p.SetInput(in)
	return p
}

// 全体件数が分かっているときは件数とバーを出す。
//
// 進捗バーは全体件数が事前に確定する処理に限る（atomic-design.md の
// 「`bubbles/progress` を使う範囲」）。一括追加がこれに当たる。
func TestProgressListShowsBarWhenTotalKnown(t *testing.T) {
	p := newList(pane.ProgressInput{
		Title: "追加中…", Rows: setupRows(), Done: 2, Total: 3, Report: nil,
	}, 60, 10)

	view := p.View()
	if !strings.Contains(view, "追加中… 2/3") {
		t.Errorf("見出しに件数が無い:\n%s", view)
	}
	if !strings.Contains(view, barFill) {
		t.Errorf("バーが出ていない:\n%s", view)
	}
	for _, r := range setupRows() {
		if !strings.Contains(view, r.Name) {
			t.Errorf("%s の行が出ていない:\n%s", r.Name, view)
		}
	}
}

// 全体件数が分からないときはバーを出さず、スピナだけを出す。
//
// 分母を示すと完了時期を約束する表示になってしまう（atomic-design.md の
// 「`bubbles/progress` を使う範囲」）。
func TestProgressListHidesBarWhenTotalUnknown(t *testing.T) {
	p := newList(pane.ProgressInput{
		Title: "集計中…", Rows: setupRows(), Done: 2, Total: 0, Report: nil,
	}, 60, 10)

	view := p.View()
	if strings.Contains(view, barFill) {
		t.Errorf("分母が不明なのにバーが出ている:\n%s", view)
	}
	if strings.Contains(view, "/") {
		t.Errorf("分母が不明なのに件数が出ている:\n%s", view)
	}
	if !strings.Contains(view, "集計中…") {
		t.Errorf("見出しが出ていない:\n%s", view)
	}
}

// 結果報告は区切り線の下に置く。
//
// 処理中の行と終わった後の報告が地続きに並ぶと、どこまでが進捗でどこからが結果なのか
// 読めない（screens.md の Setup タブのモック）。
func TestProgressListReportBelowDivider(t *testing.T) {
	report := []string{"完了: 1 台（build01-5）", "未実行: 1 台（build01-7）"}
	p := newList(pane.ProgressInput{
		Title: "追加中…", Rows: setupRows(), Done: 1, Total: 3, Report: report,
	}, 60, 20)

	lines := strings.Split(p.View(), "\n")
	divider, first := -1, -1
	for i, l := range lines {
		if divider < 0 && strings.HasPrefix(strings.TrimSpace(l), strings.Repeat(token.IconDivider, 3)) {
			divider = i
		}
		if first < 0 && strings.Contains(l, report[0]) {
			first = i
		}
	}
	if divider < 0 || first < 0 {
		t.Fatalf("区切り線（%d 行目）と結果報告（%d 行目）が揃っていない:\n%s", divider, first, p.View())
	}
	if divider > first {
		t.Errorf("結果報告が区切り線より上にある（区切り %d 行目、報告 %d 行目）", divider, first)
	}
}

// どの行も幅を超えない。折り返さず中略する。
//
// 進捗は 1 対象 1 行で読む表示であり、折り返すと行数が対象の数と合わなくなって
// どの行がどの対象なのか追えなくなる。
func TestProgressListClipsWidth(t *testing.T) {
	const width = 40

	p := newList(pane.ProgressInput{
		Title: "追加中… とても長い見出しで幅をはみ出させる",
		Rows: []molecule.ProgressView{{
			Name:   "build01-5",
			State:  molecule.ProgressFailed,
			Detail: "失敗（config.sh 終了コード 1。Http response code: Forbidden）",
		}},
		Done:   0,
		Total:  1,
		Report: []string{"build01-5 はそのまま残っています。長い行を置いて折り返しを試す。"},
	}, width, 12)

	for i, line := range strings.Split(p.View(), "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Errorf("%d 行目の幅 = %d, want <= %d（%q）", i+1, w, width, line)
		}
	}
}

// 高さに収まらない行はスクロールで辿れる。見出しとバーは動かない。
//
// 何件中何件が終わったのかは、行が 1 つも見えていなくても読めるようにする。
func TestProgressListScrolls(t *testing.T) {
	rows := make([]molecule.ProgressView, 0, 20)
	for i := range 20 {
		rows = append(rows, molecule.ProgressView{
			Name: "build01-" + string(rune('a'+i)), State: molecule.ProgressWaiting, Detail: "待機",
		})
	}
	p := newList(pane.ProgressInput{
		Title: "追加中…", Rows: rows, Done: 0, Total: 20, Report: nil,
	}, 60, 8)

	before := p.View()
	if p.Offset() != 0 {
		t.Fatalf("初期のスクロール位置 = %d, want 0", p.Offset())
	}

	p, _ = p.Update(press("down"))
	if p.Offset() == 0 {
		t.Error("j / ↓ でスクロールしない")
	}
	if after := p.View(); after == before {
		t.Error("スクロールしても表示が変わらない")
	}
	if !strings.Contains(p.View(), "追加中… 0/20") {
		t.Error("スクロールで見出しが流れた")
	}
}

// 止めた後はスピナの Tick を配らない。
//
// スピナは自分の Tick を Update で繋いで回り続けるため、止めるには配るのをやめる
// しかない。止め忘れると処理を終えた後も Msg が流れ続け、他の画面の再描画を無駄に起こす。
func TestProgressListStopEndsSpinner(t *testing.T) {
	p := newList(pane.ProgressInput{
		Title: "集計中…", Rows: nil, Done: 0, Total: 0, Report: nil,
	}, 60, 6)

	if cmd := p.Start(); cmd == nil {
		t.Fatal("Start がスピナの Cmd を返さない")
	}
	if _, cmd := p.Update(spinner.TickMsg{}); cmd == nil {
		t.Error("動いている間は Tick が繋がらない")
	}

	p.Stop()
	if _, cmd := p.Update(spinner.TickMsg{}); cmd != nil {
		t.Error("止めた後もスピナの Tick が繋がっている")
	}
}

// 配色を差し替えても進捗は残る。
//
// 背景の明暗は起動後に届き、共有状態は 3 秒ごとに配られる。作り直すと実行中の
// 進捗が消える（pane.Log.Restyle と同じ理由）。
func TestProgressListRestyleKeepsInput(t *testing.T) {
	p := newList(pane.ProgressInput{
		Title: "追加中…", Rows: setupRows(), Done: 2, Total: 3, Report: nil,
	}, 60, 10)

	p.Restyle(token.NewStyles(false, false))
	if view := p.View(); !strings.Contains(view, "追加中… 2/3") || !strings.Contains(view, "build01-5") {
		t.Errorf("配色の差し替えで中身が消えた:\n%s", view)
	}
}

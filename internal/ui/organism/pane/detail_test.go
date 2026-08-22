package pane_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/organism/pane"
)

// detailLines は n 行の内容を返す。どの行も領域の幅より長くする（幅に収まる内容だと
// 横スクロールが動く余地が無く、h / l を無効にしたことを検証できない）。
func detailLines(n int) []string {
	out := make([]string, 0, n)
	for i := range n {
		out = append(out, "行 "+strconv.Itoa(i)+" "+strings.Repeat("横に長い", 30))
	}
	return out
}

// newDetail は領域と内容を設定した詳細を返す。
func newDetail(w, h, lines int) pane.Detail {
	d := pane.NewDetail()
	d.SetSize(w, h)
	d.SetContent(detailLines(lines))
	return d
}

// sendDetail はキーを順に送る。
func sendDetail(d pane.Detail, keys ...string) pane.Detail {
	for _, k := range keys {
		d, _ = d.Update(press(k))
	}
	return d
}

// SetSize が viewport に伝わる。渡し忘れると描画が崩れたまま気付きにくい。
func TestDetailSetSize(t *testing.T) {
	// 幅・高さ・行数の組。内容が領域より多い場合と少ない場合の両方を通す。
	for _, tt := range [][3]int{{60, 8, 40}, {60, 8, 2}, {60, 8, 0}, {100, 4, 40}} {
		got := newDetail(tt[0], tt[1], tt[2]).View()
		if w := lipgloss.Width(got); w != tt[0] {
			t.Errorf("%v: 幅 = %d, want %d", tt, w, tt[0])
		}
		if h := lipgloss.Height(got); h != tt[1] {
			t.Errorf("%v: 高さ = %d, want %d", tt, h, tt[1])
		}
	}
}

// スクロールのキーで表示位置が動き、bubbles/viewport の既定キー（u / d / f / b /
// space / h / l）では動かない（理由は viewportKeyMap を参照）。
func TestDetailScrolls(t *testing.T) {
	for _, keys := range [][]string{{"j"}, {"down"}, {"ctrl+f"}, {"j", "j", "j"}} {
		d := newDetail(60, 5, 40)
		if got := sendDetail(d, keys...).View(); got == d.View() {
			t.Errorf("%v で表示位置が動いていない", keys)
		}
	}
	// 既定キーは「動く余地がある位置」で試す。先頭のままだと半ページ戻し（u）や前ページ（b）
	// が動かず、既定のキーマップに戻しても落ちないテストになる。1 行下げるのに j を使うのは、
	// 既定のキーマップでも同じキーが下方向に割り当てられているためである。
	for _, k := range []string{
		"u", "d", "f", "b", "space", "h", "l", "left", "right", "pgup", "pgdown", "home", "end",
	} {
		d := sendDetail(newDetail(60, 5, 40), "j")
		if got := sendDetail(d, k).View(); got != d.View() {
			t.Errorf("既定キー %q でスクロールした", k)
		}
	}

	// 下へ動いた後は上へ戻せる。
	d := newDetail(60, 5, 40)
	if got := sendDetail(d, "ctrl+f", "ctrl+b").View(); got != d.View() {
		t.Error("ctrl+f の後に ctrl+b で元の位置へ戻らない")
	}

	// 内容が領域に収まっているときは動かない。
	small := newDetail(60, 10, 3)
	if got := sendDetail(small, "j", "j", "ctrl+f").View(); got != small.View() {
		t.Error("内容が収まっているのに表示位置が動いた")
	}
}

// SetContent に渡したスライスは Detail の内部と共有しない。
//
// bubbles/viewport の SetContentLines は受け取ったスライスをそのまま持ち、改行を含む行を
// 分割する際に中身を書き戻す（要素への代入と slices.Insert）。写しを取らないと呼び出し側の
// スライスが書き換わり、page が手元の行を使い回した時点で表示が崩れる
// （organism/table.Model.SetItems がこの危険のために写しを取っているのと同じ）。
func TestDetailDoesNotShareContentSlice(t *testing.T) {
	// 改行を含む行を先頭に置き、容量に余裕を持たせる。slices.Insert は余裕があると
	// その場で要素を後ろへ詰めるため、呼び出し側の配列が書き換わる経路がここで開く
	// （page が append で組み立てた行はこの形になりうる）。
	lines := make([]string, 0, 8)
	lines = append(lines, "1 行目\n2 行目", "3 行目", "4 行目")
	want := slices.Clone(lines)

	d := pane.NewDetail()
	d.SetSize(40, 10)
	d.SetContent(lines)

	if !slices.Equal(lines, want) {
		t.Errorf("渡したスライスが書き換わった: %q, want %q", lines, want)
	}

	// 渡した後に呼び出し側が書き換えても表示は変わらない。
	before := d.View()
	lines[0] = "書き換え"
	if got := d.View(); got != before {
		t.Errorf("渡したスライスの書き換えが表示に及んだ:\n%q\n→\n%q", before, got)
	}

	// 改行を含む行は分割して表示する（写しを取っても分割の挙動は保つ）。
	if !strings.Contains(before, "1 行目") || !strings.Contains(before, "2 行目") {
		t.Errorf("改行を含む行が分割されていない:\n%q", before)
	}
}

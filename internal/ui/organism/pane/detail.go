package pane

import (
	"slices"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
)

// Detail は詳細画面の情報部分。スクロールは bubbles/viewport に委ねる。
//
// 詳細画面は Detail と organism.ChoiceList（操作リスト）の組み合わせで構成し、詳細画面
// 専用の部品を作らない（atomic-design.md の詳細画面の操作リスト）。
type Detail struct {
	vp viewport.Model
}

// NewDetail は詳細の表示を組み立てる。
//
// キー定義を引数に取らず keymap から読むのは、スクロールのキーが画面ごとに変わらない
// ためである（既定のキーを使わない理由は viewportKeyMap）。token.Styles を受け取らないのは、
// 表示する行を装飾済みで page から受け取り、Detail 自身は色を決めないためである
// （Help は自分でキーと説明を描くので受け取る）。
func NewDetail() Detail {
	vp := viewport.New()
	vp.KeyMap = viewportKeyMap(keymap.NewList())
	return Detail{vp: vp}
}

// SetContent は表示する行を差し替える。
//
// 渡されたスライスは写しを取って渡す。bubbles/viewport の SetContentLines は受け取った
// スライスをそのまま持ち、改行を含む行を分割する際に中身を書き戻す（要素への代入と、
// 容量に余裕があればその場で詰める slices.Insert）。写しを取らないと呼び出し側の
// スライスが書き換わり、page が手元の行を使い回した時点で表示が崩れる。行数は高々
// 数十なので、確保の費用より状態が壊れる事故の重さを採る
// （organism/table.Model.SetItems と同じ理由）。
func (d *Detail) SetContent(lines []string) {
	d.vp.SetContentLines(slices.Clone(lines))
}

// GotoTop はスクロール位置を先頭へ戻す。
//
// 対象そのものを差し替える側（別の runner の詳細を開く）が呼ぶ。位置を持ち越すと、
// 前に読んでいた場所から始まる詳細が出て、**別の runner の情報を今の runner のものと
// して読んでしまう**。同じ対象の内容を差し替えるだけのときは呼ばないこと
// （3 秒ごとの再検出で読んでいた場所を失う）。
func (d *Detail) GotoTop() {
	d.vp.GotoTop()
}

// Offset はスクロール位置（先頭から隠している行数）を返す。
func (d Detail) Offset() int { return d.vp.YOffset() }

// SetSize は詳細に割り当てられた領域を設定する。
func (d *Detail) SetSize(w, h int) {
	d.vp.SetWidth(w)
	d.vp.SetHeight(h)
}

// Update はスクロールのキーを処理する。
func (d Detail) Update(msg tea.Msg) (Detail, tea.Cmd) {
	var cmd tea.Cmd
	d.vp, cmd = d.vp.Update(msg)
	return d, cmd
}

// View は表示中の範囲を返す。
func (d Detail) View() string {
	return d.vp.View()
}

package pane

import (
	"slices"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
)

// Log はログ本文の表示領域。スクロールは bubbles/viewport に委ね、末尾への追従を足す。
//
// Detail と分けているのは、持つ状態が 1 つ多い（追従の ON / OFF）ためである。詳細画面は
// 内容が差し替わっても位置を保つのが正しく、ログは末尾へ寄せるのが既定であり、同じ型に
// 両方を入れると呼び出し側が毎回どちらのふるまいかを指定することになる。
//
// **色は決めない。** 受け取るのは装飾済みの行であり、`ERROR` / `WARN` の強調を決めるのは
// page である（Detail と同じ理由。organism は token.Styles を受け取らない）。
type Log struct {
	vp     viewport.Model
	follow bool
}

// NewLog はログ本文の表示領域を組み立てる。追従は ON で始める。
//
// 開いた直後に見たいのは末尾（今起きていること）だからである（screens.md の Logs タブ）。
func NewLog() Log {
	vp := viewport.New()
	vp.KeyMap = viewportKeyMap(keymap.NewList())
	return Log{vp: vp, follow: true}
}

// SetContent は表示する行を差し替える。追従が ON なら末尾へ寄せる。
//
// 渡されたスライスは写しを取って渡す（Detail.SetContent と同じ理由。bubbles/viewport は
// 受け取ったスライスをそのまま持ち、改行を含む行を分割する際に中身を書き戻す）。
func (l *Log) SetContent(lines []string) {
	l.vp.SetContentLines(slices.Clone(lines))
	if l.follow {
		l.vp.GotoBottom()
	}
}

// SetSize はログ本文に割り当てられた領域を設定する。
//
// 追従が ON なら末尾へ寄せ直す。高さが変わると末尾の位置も変わるため、寄せ直さないと
// 追従中のはずの画面が末尾の手前で止まる。
func (l *Log) SetSize(w, h int) {
	l.vp.SetWidth(w)
	l.vp.SetHeight(h)
	if l.follow {
		l.vp.GotoBottom()
	}
}

// Following は末尾へ追従しているかを返す。page は状態行の `追従: ON` に使う。
func (l Log) Following() bool { return l.follow }

// SetFollow は追従の ON / OFF を設定する。ON にしたときは末尾へ移る。
//
// `f`（追従の切替）と `G`（末尾へ移動して追従を再開）の両方がここを通る。ON にした
// 時点で末尾へ移らないと、`G` を押しても画面が動かないまま追従だけが立つ。
func (l *Log) SetFollow(on bool) {
	l.follow = on
	if on {
		l.vp.GotoBottom()
	}
}

// Update はスクロールのキーを処理する。
//
// **末尾から離れた時点で追従を切る**（screens.md の「手動でスクロールすると追従が
// OFF になり、G で再開する」）。キーの種類で判断しないのは、末尾で `j` を押しても
// 位置が変わらない（追従を切る理由が無い）ためである。位置の結果で判断すれば、
// スクロールのキーが増えても判定を足さずに済む。
func (l Log) Update(msg tea.Msg) (Log, tea.Cmd) {
	var cmd tea.Cmd
	l.vp, cmd = l.vp.Update(msg)
	if l.follow && !l.vp.AtBottom() {
		l.follow = false
	}
	return l, cmd
}

// View は表示中の範囲を返す。
func (l Log) View() string {
	return l.vp.View()
}

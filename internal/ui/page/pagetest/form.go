package pagetest

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// huh のフォームを載せた画面を打鍵で進める道具を集める。

// QuickTimeout は Quick が Cmd 1 本を待つ上限。
//
// 入力欄のカーソルの点滅（0.53 秒）には届かず、ドメイン層の 1 往復には十分な
// 長さである（AdvanceQuick の doc）。
const QuickTimeout = 100 * time.Millisecond

// Paste は入力欄へ文字列をまとめて流し込む Msg を作る。
//
// 1 文字ずつの打鍵と同じ経路（huh の Input → bubbles/textinput）を通るが、
// 打鍵の回数だけカーソルの点滅を辿らずに済む。
func Paste(s string) tea.Msg { return tea.PasteMsg{Content: s} }

// Quick は Msg を順に配り、そのつど AdvanceQuick で落ち着くまで進める。
//
// フォームの打鍵と実行中の描画はこちらを使う。Advance は点滅を辿って 1 打鍵
// ごとに数秒待ち、進捗待ちの Cmd に至っては戻ってこない。
func Quick(m tea.Model, msgs ...tea.Msg) tea.Model {
	for _, msg := range msgs {
		next, cmd := m.Update(msg)
		m = AdvanceQuick(next, cmd, AdvanceRounds, QuickTimeout)
	}
	return m
}

// SubmitHuh は項目送りを繰り返して入力を確定させ、done が真になるまで進める。
//
// huh は項目の送り・区画の送り・送信を Msg で自分へ返す作りなので、1 回では
// 完了に届かない。steps を使い切っても done が偽なら、偽を返して呼び出し側に
// 判定を委ねる（このパッケージは testing を import しない）。
func SubmitHuh(m tea.Model, steps int, done func(tea.Model) bool) (tea.Model, bool) {
	for range steps {
		if done(m) {
			return m, true
		}
		m = Quick(m, huh.NextField())
	}
	return m, done(m)
}

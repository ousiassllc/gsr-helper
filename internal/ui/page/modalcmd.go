package page

import (
	"reflect"

	tea "charm.land/bubbletea/v2"
)

// モーダルの中身が発行した Cmd を、その中身へ戻すための包みを置く。

// WrapModal は中身が発行した Cmd の結果を、その中身へ戻るように包む。
//
// 包まないと結果は「そのとき選択中のタブの最上位のモーダル」へ配られる
// （page.AttachMsg / ModalMsg の doc）。確認ダイアログの決定は上に別の
// モーダルが重なった後やタブを切り替えた後に届きうるし、待機画面の計時と
// スピナの Tick は自分自身へ戻らなければ動きが止まる。
//
// **包む相手は中身が自分で発行した Cmd に限る。** bubbletea / bubbles が解釈する
// Msg（終了・順次実行）を包むとランタイムへ届かなくなる（Do の doc）。
// dialog.Confirm は決定の Cmd だけ、dialog.DrainWaiter は bubbles の stopwatch と
// spinner の Tick だけを返し、いずれもランタイム宛ではなく自分宛である。
//
// **束（tea.Batch / tea.Sequence の結果）は展開してから 1 本ずつ包む。** 束はランタイムが
// 中身を取り出して実行するものであり、そのまま包むと ModalMsg の中に取り出されない
// []tea.Cmd が入るだけで**誰も実行しない**。待機画面の Start は計時とスピナを束で返し、
// その計時（stopwatch.Start）自身も開始の指示と最初の Tick を **tea.Sequence** で返すため、
// どちらの束も展開しないと計時が動かず、経過時間が 0 のまま止まる。
func WrapModal(tab int, kind ModalKind, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		if bundle := expand(tab, kind, msg); bundle != nil {
			return bundle()
		}
		return Do(tab, func() tea.Msg {
			return ModalMsg{Kind: kind, Msg: msg}
		})()
	}
}

// expand は束の中身を 1 本ずつ包み直した**同じ種類の**束を返す。束でなければ nil を返す。
//
// 束を返す Cmd はランタイムが再び中身を取り出すので、入れ子の束もこの経路で
// 展開される（WrapModal を再帰的に通す）。
//
// tea.Batch と tea.Sequence を包み直す関数で区別するのは、順次実行の束を Batch に
// 均すと順序の保証が消えるためである。stopwatch.Start は「開始の指示 → 最初の Tick」の
// 順を前提にしており（先に Tick が届くと止まっている間の Tick として捨てられる）、
// 並行に流すと計時が動かないことがある。
func expand(tab int, kind ModalKind, msg tea.Msg) tea.Cmd {
	if batch, ok := msg.(tea.BatchMsg); ok {
		return tea.Batch(wrapAll(tab, kind, batch)...)
	}
	seq, ok := cmdSlice(msg)
	if !ok {
		return nil
	}
	return tea.Sequence(wrapAll(tab, kind, seq)...)
}

// wrapAll は束の中身を 1 本ずつ包んだ並びを返す。
func wrapAll(tab int, kind ModalKind, cmds []tea.Cmd) []tea.Cmd {
	out := make([]tea.Cmd, 0, len(cmds))
	for _, c := range cmds {
		out = append(out, WrapModal(tab, kind, c))
	}
	return out
}

// cmdSlice は Msg が「要素型が tea.Cmd の slice」ならその中身を返す。
//
// **reflect で見分けるのは、tea.Sequence の実体（sequenceMsg）が非公開型だからである。**
// tea.BatchMsg は公開型なので型アサーションで捕まえられるが、順次実行の束には
// 名指しできる型が無く、`msg.(tea.SequenceMsg)` に相当する書き方が存在しない。
// 束はどちらも []tea.Cmd を素の型に持つので、要素型で判別する
// （テスト側の pagetest.Cmds も同じ手を使っている）。
func cmdSlice(msg tea.Msg) ([]tea.Cmd, bool) {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeFor[tea.Cmd]() {
		return nil, false
	}

	out := make([]tea.Cmd, 0, v.Len())
	for i := range v.Len() {
		c, ok := v.Index(i).Interface().(tea.Cmd)
		if !ok {
			return nil, false
		}
		out = append(out, c)
	}
	return out, true
}

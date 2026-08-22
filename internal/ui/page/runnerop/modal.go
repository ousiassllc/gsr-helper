package runnerop

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// モーダル 2 枚（確認ダイアログ・待機画面）が共有する包みの都合を置く。

// wrap は中身が発行した Cmd の結果を、その中身へ戻るように包む。
//
// 包まないと結果は「そのとき選択中のタブの最上位のモーダル」へ配られる
// （page.AttachMsg / page.ModalMsg の doc）。確認ダイアログの決定は上に別の
// モーダルが重なった後やタブを切り替えた後に届きうるし、待機画面の計時と
// スピナの Tick は自分自身へ戻らなければ動きが止まる。
//
// **包む相手は中身が自分で発行した Cmd に限る。** bubbletea / bubbles が解釈する
// Msg（終了・順次実行）を包むとランタイムへ届かなくなる（page.Do の doc）。
// dialog.Confirm は決定の Cmd だけ、dialog.DrainWaiter は bubbles の stopwatch と
// spinner の Tick だけを返し、いずれもランタイム宛ではなく自分宛である。
//
// **束（tea.Batch の結果）は展開してから 1 本ずつ包む。** 束はランタイムが中身を
// 取り出して実行するものであり、そのまま包むと ModalMsg の中に取り出されない
// []tea.Cmd が入るだけで**誰も実行しない**。待機画面の Start は計時とスピナの
// 2 本を束で返すため、展開しないとどちらも動かず、経過時間が 0 のまま止まる。
func wrap(tab int, kind page.ModalKind, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			return expand(tab, kind, batch)()
		}
		return page.Do(tab, func() tea.Msg {
			return page.ModalMsg{Kind: kind, Msg: msg}
		})()
	}
}

// expand は束の中身を 1 本ずつ包み直した束を返す。
//
// 束を返す Cmd はランタイムが再び中身を取り出すので、入れ子の束もこの経路で
// 展開される（wrap を再帰的に通す）。
func expand(tab int, kind page.ModalKind, batch tea.BatchMsg) tea.Cmd {
	out := make([]tea.Cmd, 0, len(batch))
	for _, c := range batch {
		out = append(out, wrap(tab, kind, c))
	}
	return tea.Batch(out...)
}

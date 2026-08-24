// Package cmdtest は tea.Cmd を走らせ、束（tea.Batch / tea.Sequence）を辿るための
// 道具を集める。
//
// **page/pagetest から分けてある。** 分けた直接の理由は 1 ディレクトリ 2000 行の上限で
// （pagetest が Issue #140 で警告帯に入った）、境界はこの責務の違いに沿わせた
// ——pagetest 本体が持つのは「タブと親 Model を組み立てて動かす」道具（共有状態・
// フィクスチャ・Spy）であり、こちらは「発行された Cmd をどう走らせ、どう辿るか」
// だけを持つ（Issue #147）。依存は cmdtest → page の一方向で、pagetest 側からは
// import するが逆は無い。
//
// **`testing` を import しない。** pagetest と同じく通常のパッケージなので、import すると
// テスト用のフラグが本番のバイナリ側の依存に現れる。合否の判定は呼び出し側の _test.go に
// 残し、ここは結果と成否だけを返す。**足したら pagetest/import_test.go の fixtures へ
// 登録すること**（登録しないと本番混入の検査の網から外れる）。
package cmdtest

import (
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/stopwatch"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// 非同期の往復（page が Cmd を返し、親が結果を発行元のタブへ戻す）を回す道具を置く。

// AdvanceRounds は Advance の既定の往復数。
//
// サービス制御の最長経路（キー → 確認ダイアログ → y → 実行 → 結果の報告、
// ドレインなら 待機 → esc → キャンセル → 結果）が 4 往復で収まる。余裕を持たせて
// あるのは、往復が足りないと**結果が届く前に静かに打ち切られる**ためである。
const AdvanceRounds = 8

// Advance はタブが発行した Cmd の結果を、親 Model と同じ規則でタブへ配り直す。
//
// 親は page.TabMsg を外して発行元のタブへ渡す（ui/app.go の forwardTo）。この往復を
// テストごとに手で書くと、1 段を書き忘れたテストだけが「何も起きない」を正常として
// 緑になる。**サービス制御の検証はこの往復の上に載っている**（確認の決定も実行結果も
// 一度 Cmd を経由して戻ってくる）ので、道具として 1 つに集める。
//
// **計時とスピナの Tick は配らない。** bubbles の stopwatch / spinner は自分の Tick を
// Update で繋いで回り、その Cmd（tea.Tick）は実時間を待つ。配ると 1 往復ごとに
// テストが 1 秒止まり、しかも Tick が Tick を生むので rounds を使い切るまで終わらない。
// 動きそのものの検証は organism/dialog のテストが持つ。
//
// ChromeMsg と GlobalKeyMsg も配らない。どちらも親が解釈するもので、タブへは戻らない。
//
// **戻らない Cmd は CmdTimeout で諦める**（Msgs）。素で走らせていたころはそこで
// 止まり、壊れ方が失敗ではなくハングになった（Issue #150）。諦めた本数は見ない
// ——rounds で打ち切る時点でこの道具は「配れるだけ配る」ものであり、ここへ渡す
// Cmd はいずれも速やかに戻る前提だからである（戻らない Cmd を含む往復は
// AdvanceQuick を使うこと）。
func Advance(m tea.Model, cmd tea.Cmd, rounds int) tea.Model {
	for range rounds {
		if cmd == nil {
			return m
		}

		msgs, _ := Msgs(cmd, CmdTimeout)
		next := make([]tea.Cmd, 0, 4)
		for _, msg := range msgs {
			inner, ok := deliverable(msg)
			if !ok {
				continue
			}
			var c tea.Cmd
			m, c = m.Update(inner)
			next = append(next, c)
		}
		cmd = tea.Batch(next...)
	}
	return m
}

// AdvanceQuick は Advance と同じ配り直しを、1 本あたり timeout だけ待って行う。
//
// **入力欄のカーソルの点滅を辿らないためにある。** bubbles/cursor の点滅は
// 0.53 秒待ってから Msg を返す Cmd で、その Msg を配るとまた点滅の Cmd が返る。
// 打鍵でフォームを進める検証は 1 打鍵ごとにこれを何度も引き受けることになり、
// テスト 1 本で数十秒かかる。点滅は検証の対象ではないので、戻らない Cmd と同じ
// 扱いで諦める（RunCmd）。
//
// timeout は「配りたい Cmd が確実に間に合い、点滅は間に合わない」長さにすること。
// ドメイン層の呼び出しを含む往復では、その処理時間を見込んだ値を渡す。
//
// **Msgs の待ち時間切れは捨てる。** ここでは諦めることそのものが目的なので、
// 諦めた本数は異常の合図にならない。
func AdvanceQuick(m tea.Model, cmd tea.Cmd, rounds int, timeout time.Duration) tea.Model {
	for range rounds {
		if cmd == nil {
			return m
		}

		msgs, _ := Msgs(cmd, timeout)
		next := make([]tea.Cmd, 0, 4)
		for _, msg := range msgs {
			inner, ok := deliverable(msg)
			if !ok {
				continue
			}
			var c tea.Cmd
			m, c = m.Update(inner)
			next = append(next, c)
		}
		cmd = tea.Batch(next...)
	}
	return m
}

// deliverable は Msg をタブへ配り直すかを返す。配る場合は包みを外した中身を返す。
func deliverable(msg tea.Msg) (tea.Msg, bool) {
	tm, ok := msg.(page.TabMsg)
	if !ok {
		return nil, false
	}
	if mm, ok := tm.Msg.(page.ModalMsg); ok && isTick(mm.Msg) {
		return nil, false
	}
	return tm.Msg, true
}

// isTick は bubbles の計時・スピナが自分を回すために使う Msg かを返す。
//
// 型で名指しするのは、これらを配ったときに返る Cmd が実時間を待つ唯一の経路だから
// である。新しく計時を持つ部品を足したときは、その Tick もここへ足すこと。
func isTick(msg tea.Msg) bool {
	switch msg.(type) {
	case stopwatch.TickMsg, stopwatch.StartStopMsg, spinner.TickMsg:
		return true
	default:
		return false
	}
}

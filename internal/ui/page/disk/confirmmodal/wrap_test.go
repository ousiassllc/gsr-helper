package confirmmodal

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// 包み（`page.WrapModal`）の回帰テスト（Issue #190）。
//
// **このパッケージは自前の包み（`modal.wrap`）を持っていた。** 兄弟のモーダルが使う
// `page.WrapModal` の写しだが、束（`tea.Batch` / `tea.Sequence`）を展開する経路が欠けて
// おり、`cmd()` の結果をそのまま `page.ModalMsg` に入れていた。写しを落として
// `page.WrapModal` へ寄せたので、寄せた先の性質をここで縛る。
//
// **`confirmmodal_test.go` と分けたのは 1 ファイル 300 行の上限である**（ディレクトリ側の
// 残りには余裕があるが、ファイル側が警告帯に入った）。境界は責務に沿わせた——あちらは
// 打鍵から決定までの往復、こちらは包みそのものの性質を見る。

// 包みは束（tea.Batch / tea.Sequence）を展開してから 1 本ずつ包み直す。
//
// 束をそのまま包むと、**取り出されない `[]tea.Cmd` が `page.ModalMsg` の中身のまま
// 届いて誰も実行しない**（`page.WrapModal` の doc）。`dialog.Confirm` が返す Cmd は決定
// 1 本だけなので写しでも壊れていなかったが、ダイアログが束を返すようになった瞬間に
// 「y を押しても何も起きない」へ変わり、**コンパイルエラーも実行時エラーも出ない。**
//
// **縛るのは `modal.Update` の default が呼ぶ包みそのものである。** その呼び出しが
// タブ番号と種類を正しく渡すことは `confirmmodal_test.go` の decideVia を通す 3 本が縛る。
// 束の平坦化に `cmdtest.MustMsgs`（Batch と Sequence を区別せず再帰的に辿る＝ランタイムと
// 同じ規則）を使うのは、包み自身の判定に頼らないためである。
func TestWrapExpandsBundles(t *testing.T) {
	t.Parallel()

	cases := map[string]tea.Cmd{
		"tea.Batch":    func() tea.Msg { return tea.Batch(mark(1), mark(2))() },
		"tea.Sequence": func() tea.Msg { return tea.Sequence(mark(1), mark(2))() },
		// 束の中の束も展開される（束を返す Cmd はランタイムが再び中身を取り出す）。
		"入れ子": func() tea.Msg { return tea.Batch(mark(1), tea.Sequence(mark(2)))() },
	}

	for name, cmd := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := make(map[int]bool, 2)
			for _, msg := range cmdtest.MustMsgs(page.WrapModal(testTab, Kind, cmd), cmdtest.CmdTimeout) {
				inner, ok := modalMsg(t, msg)
				if !ok {
					continue
				}
				m, isMark := inner.(markMsg)
				if !isMark {
					t.Errorf("包みの中身 = %T, want markMsg（束が展開されていない）", inner)
					continue
				}
				got[m.n] = true
			}

			for n := 1; n <= 2; n++ {
				if !got[n] {
					t.Errorf("束の %d 本目がモーダルへ戻ってこない（展開されずに封じられている）", n)
				}
			}
		})
	}
}

// 包みは Cmd が無いときに何も発行しない。
//
// ダイアログが受け取らない Msg（大半がこれである）に対しては nil が返る。nil を包んで
// Cmd を作ると、打鍵のたびに中身の無い page.ModalMsg がタブへ流れる。
func TestWrapKeepsNilAsNil(t *testing.T) {
	t.Parallel()

	if got := page.WrapModal(testTab, Kind, nil); got != nil {
		t.Error("Cmd が無いのに包みが Cmd を返した")
	}
}

// markMsg は束の中身が 1 本ずつ包み直されたことを見るための目印。
type markMsg struct{ n int }

// mark は目印を返す Cmd を組み立てる。
func mark(n int) tea.Cmd {
	msg := markMsg{n: n}
	return func() tea.Msg { return msg }
}

// modalMsg は Msg がこのモーダル宛の包みなら中身を返す。宛先と種類はここで見る。
func modalMsg(t *testing.T, msg tea.Msg) (tea.Msg, bool) {
	t.Helper()

	tab, ok := msg.(page.TabMsg)
	if !ok {
		return nil, false
	}
	if tab.Tab != testTab {
		t.Errorf("包みの宛先 = タブ %d, want %d", tab.Tab, testTab)
		return nil, false
	}
	to, ok := tab.Msg.(page.ModalMsg)
	if !ok {
		return nil, false
	}
	if to.Kind != Kind {
		t.Errorf("包みの種類 = %q, want %q", to.Kind, Kind)
		return nil, false
	}
	return to.Msg, true
}

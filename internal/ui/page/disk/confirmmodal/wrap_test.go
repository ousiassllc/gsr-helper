package confirmmodal

import (
	"go/ast"
	"go/parser"
	"go/token"
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
// 残りには余裕があるが、まとめていれば 330 行を超えて ERROR 帯に当たっていた）。境界は
// 責務に沿わせた——あちらは打鍵から決定までの往復、こちらは包みそのものの性質を見る。

// 包みは束（tea.Batch / tea.Sequence）を展開してから 1 本ずつ包み直す。
//
// 束をそのまま包むと、**取り出されない `[]tea.Cmd` が `page.ModalMsg` の中身のまま
// 届いて誰も実行しない**（`page.WrapModal` の doc）。`dialog.Confirm` が返す Cmd は決定
// 1 本だけなので写しでも壊れていなかったが、ダイアログが束を返すようになった瞬間に
// 「y を押しても何も起きない」へ変わり、**コンパイルエラーも実行時エラーも出ない。**
//
// **この 2 本が縛るのは `page.WrapModal` 単体の性質である**（`modal.Update` を通らない）。
// その包みを `modal.Update` の default が呼ぶこと——タブ番号と種類を正しく渡すこと——は
// `confirmmodal_test.go` の decideVia を通す 3 本が縛り、**自前の写しへ戻る退行**は
// 下の TestUpdateDoesNotBuildModalMsgByHand が構文で縛る。
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

// このパッケージは包みを `page.WrapModal` に任せ、`page.ModalMsg` を自分で組み立てない。
//
// **打鍵の経路では写しと `page.WrapModal` を挙動で区別できない。** `dialog.Confirm` が返す
// Cmd は決定 1 本だけで束を返さないため、束を展開しない写しを default へ戻しても、上の
// 2 本も `confirmmodal_test.go` の 3 本も緑のままである（Issue #190 のコミットに両方向の
// 変異注入の記録がある）。区別を挙動で付けるには `modal.dlg` を差し替え可能にする
// （インタフェース化する）ほか無く、テストのために本番の構造を変える判断は #190 で
// 採らなかった——**その代わりに写しの形そのものをここで構文として縛る。**
//
// 写しがしていたのは `page.ModalMsg` を手で組み立てて `page.Do` へ渡すことであり、
// モーダル宛の包みを作る正規の入口は `page.WrapModal` だけである。「`page.WrapModal` を
// 呼ぶこと」と「`page.ModalMsg` を手で組み立てないこと」の 2 つを見れば、写しが戻った
// 時点で必ず赤くなる。構文木で見るのは、同じ名前が doc コメントにも現れるためである。
func TestUpdateDoesNotBuildModalMsgByHand(t *testing.T) {
	t.Parallel()

	const src = "confirmmodal.go"
	file, err := parser.ParseFile(token.NewFileSet(), src, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("%s を解析できない: %v", src, err)
	}

	wraps, byHand := 0, 0
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if isPageRef(node.Fun, "WrapModal") {
				wraps++
			}
		case *ast.CompositeLit:
			if isPageRef(node.Type, "ModalMsg") {
				byHand++
			}
		}
		return true
	})

	if wraps == 0 {
		t.Errorf("%s が page.WrapModal を呼んでいない（包みを自前で持っている）", src)
	}
	if byHand != 0 {
		t.Errorf("%s が page.ModalMsg を手で %d か所組み立てている"+
			"（束を展開する経路が抜け、中身が誰にも実行されない）", src, byHand)
	}
}

// isPageRef は式が page パッケージの name を指しているかを返す。
func isPageRef(expr ast.Expr, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "page"
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

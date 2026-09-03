package chrome

import (
	"errors"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 状態行を幅に収める挙動を検証する（Issue #136）。
//
// 初めから chrome_test.go とは別ファイルにした——この検証を足すと同ファイルは
// 320 行となり 1 ファイル 300 行の警告帯（301〜330 行）に入る。境界は行数だけ
// ではない——あちらは「状態行に何が出るか」を、ここは「幅に収まらないときに何を削るか」を固定する。

// 状態行は幅に収まる。右側の文言が長くても枠を溢れさせない（Issue #136）。
//
// atom.Justify は幅が足りなくても切り詰めないため、Status が中略しないと
// 「左 + 空白 1 + 右」がそのまま幅を超える。溢れた分は template.Frame が
// 中略記号なしで断ち切るので、文が黙って途中で終わる。
func TestStatusFitsWidthWhenPageStatusIsLong(t *testing.T) {
	// results.go の再登録の案内（68 セル）。右側の最長ではない——runnerop の
	// 操作結果報告は runner 名とエラー 1 行目を連ねるため長さに上限が無い。
	// ここで使うのは、幅 80 を実際に超える実在の文言のうち長さが固定で、
	// 期待値を書ける唯一の種類だからである。
	const long = "変更には再登録が必要です（Setup タブで削除して追加し直してください）"

	v := testView()
	v.OrphanUnits = 2
	v.Warnings = 1
	v.Status = long
	v.Width = 80

	got := Status(v)
	if w := lipgloss.Width(got); w > v.Width {
		t.Errorf("状態行の幅 = %d, want %d 以下（%q）", w, v.Width, got)
	}
	// 削るのは右側の末尾であり、左側の件数は残る。
	for _, want := range []string{"孤児ユニット 2 件", "警告 1 件"} {
		if !strings.Contains(got, want) {
			t.Errorf("状態行から左側の %q が消えている: %q", want, got)
		}
	}
	// 右側も丸ごとは消さない。頭が残り、中略記号が末尾に付く。
	if !strings.Contains(got, "変更には再登録が必要です") {
		t.Errorf("状態行に右側の頭が無い: %q", got)
	}
	if !strings.HasSuffix(got, token.IconEllipsis) {
		t.Errorf("中略記号で終わっていない: %q", got)
	}
}

// 左側だけで幅を超える場合も収める。直近のエラーの文言は長さを選べない。
//
// このとき**右側は 1 文字も残らない**（screens.md の状態行）。左側に出るのは
// 検出結果そのもので、右側の報告は打鍵のたびに出し直せるためである。Status を
// 空でない値にしてあるのは、atom.Justify に右側を連結させてその状況を作るため
// であり、残ることを期待しているわけではない。
func TestStatusFitsWidthWhenErrorAloneIsLong(t *testing.T) {
	v := testView()
	v.Err = errors.New(strings.Repeat("長いエラー", 20))
	v.Status = "2 件選択中"
	v.Width = 80

	got := Status(v)
	if w := lipgloss.Width(got); w > v.Width {
		t.Errorf("状態行の幅 = %d, want %d 以下（%q）", w, v.Width, got)
	}
}

// 幅がまだ届いていない（0）状態でも状態行を空にしない。
//
// 固定するのは画面の挙動ではなく Status の契約である。幅 0 のフレームで実際に
// 描かれるのは template.Frame の縮退表示（幅 60 未満は tooNarrow）であって
// 状態行ではない。それでも、幅を知らないことを理由に内容を捨てる純粋関数には
// しない——atom.Truncate は幅 0 に空文字を返すため、無条件に通すと消える。
func TestStatusKeepsContentBeforeFirstResize(t *testing.T) {
	v := testView()
	v.OrphanUnits = 1
	v.Width = 0

	if got := Status(v); !strings.Contains(got, "孤児ユニット 1 件") {
		t.Errorf("幅 0 の状態行 = %q, want 件数が残る", got)
	}
}

// 色ありでも中略が幅を壊さない。状態行は装飾済みの文字列を切る唯一の経路である。
//
// counts は Warn と Fail を使い分けるため、行には ANSI 列が複数並ぶ。atom 側の
// 検証（TestTruncateKeepsANSISequenceIntact）は装飾 1 つの文字列なので、
// 複数の装飾が並ぶこの形はここでしか踏まない。
func TestStatusFitsWidthWithColorAndMultipleStyles(t *testing.T) {
	v := testView()
	v.Styles = token.NewStyles(true, true)
	v.OrphanUnits = 2
	v.Warnings = 1
	v.Err = errors.New("検出に失敗しました")
	// 無効なタブの理由は Muted で装飾されて右側に入る。
	v.Notice = "[3]Disk この版では未対応です。幅を広げてから開き直してください"
	v.Width = 80

	got := Status(v)
	if w := lipgloss.Width(got); w > v.Width {
		t.Errorf("状態行の幅 = %d, want %d 以下（%q）", w, v.Width, got)
	}
	// 中略記号は装飾の外に付く（atom.Cell の doc）。ANSI 列を割っていれば
	// 幅の計算が狂うので、上の幅の検査と合わせて健全性が見える。
	if !strings.HasSuffix(got, token.IconEllipsis) {
		t.Errorf("中略記号で終わっていない: %q", got)
	}
}

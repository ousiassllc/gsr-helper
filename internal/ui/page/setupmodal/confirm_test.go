package setupmodal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 確認ダイアログの包み方の回帰テスト（Issue #187）。
//
// **判断は 1 つだけある——決定をどの種類で差し戻すかである。** 中身
// （organism/dialog）は共通で、実行前プレビューと入力の破棄は同じ confirmModal を
// 種類だけ変えて 2 枚登録する。差し戻しの Kind を取り違えると「破棄するつもりで
// 押した y で実行が始まる」形の事故になる（ConfirmKind の doc）。ここは値の比較
// ではなく**2 枚を同時に開いた状態で決定を流す**形で縛る。

// newConfirmOverlay は 2 種類の確認ダイアログを登録した Overlay を返す。
//
// **2 枚を同時に登録するのが要点である。** 1 枚だけだと、Kind を m.kind ではなく
// 定数リテラルへ書き換える退行が片方では緑になる。二重登録の panic
// （page.Overlay.Register）が種類の綴りの重複も同時に見る。
func newConfirmOverlay(t *testing.T) page.Overlay {
	t.Helper()

	st := state()
	o, _ := page.NewOverlay(testTab, st)
	o.Register(ConfirmKind, NewConfirm(st, ConfirmKind))
	o.Register(DiscardKind, NewConfirm(st, DiscardKind))

	return o
}

// sampleInput は実行前プレビューの中身。
func sampleInput() dialog.ConfirmInput {
	return dialog.ConfirmInput{
		Title:   "runner の追加",
		Targets: []string{"build01-1"},
		Impact:  []string{"systemd ユニットを作成します"},
		Command: []string{"./config.sh", "--unattended"},
		Note:    nil,
	}
}

// 決定は開いた側の種類で差し戻される。
func TestConfirmResultCarriesOwnKind(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		open func(o *page.Overlay) tea.Cmd
		want page.ModalKind
	}{
		"実行前プレビュー": {
			func(o *page.Overlay) tea.Cmd { return OpenConfirm(o, sampleInput()) },
			ConfirmKind,
		},
		"入力の破棄": {OpenDiscard, DiscardKind},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			o := newConfirmOverlay(t)
			tt.open(&o)

			res := decideVia(t, &o, "y")

			if res.Kind != tt.want {
				t.Errorf("決定の種類 = %q, want %q（宛先を取り違えている）", res.Kind, tt.want)
			}
			decided, ok := res.Msg.(dialog.DecidedMsg)
			if !ok {
				t.Fatalf("決定の中身 = %T, want dialog.DecidedMsg", res.Msg)
			}
			if !decided.Confirmed {
				t.Error("y が否認として差し戻された")
			}
		})
	}
}

// esc は Overlay ではなく確認ダイアログが先に受ける。
//
// Overlay に閉じさせると dialog.DecidedMsg{false} が出ないまま画面が消え、
// **承認待ちの計画が残ったまま次の確認で実行されうる**（NewConfirm の doc）。
func TestConfirmHandlesBackItself(t *testing.T) {
	t.Parallel()

	o := newConfirmOverlay(t)
	OpenConfirm(&o, sampleInput())

	res := decideVia(t, &o, "esc")

	if !o.Active() {
		t.Error("esc で Overlay がモーダルを閉じた（確認ダイアログへ届いていない）")
	}
	if res.Kind != ConfirmKind {
		t.Errorf("esc の決定の種類 = %q, want %q", res.Kind, ConfirmKind)
	}
	decided, ok := res.Msg.(dialog.DecidedMsg)
	if !ok {
		t.Fatalf("esc の決定の中身 = %T, want dialog.DecidedMsg", res.Msg)
	}
	if decided.Confirmed {
		t.Error("esc が承認として差し戻された")
	}
}

// 開く指示に載せた中身がそのまま表示される（実行前プレビューは呼び出し側が組む）。
func TestOpenConfirmShowsGivenInput(t *testing.T) {
	t.Parallel()

	o := newConfirmOverlay(t)
	in := sampleInput()
	OpenConfirm(&o, in)

	if got := confirmTitle(modelOf(t, o, ConfirmKind)); got != in.Title {
		t.Errorf("見出し = %q, want %q", got, in.Title)
	}
	view := o.View()
	for _, want := range []string{in.Targets[0], in.Impact[0], in.Command[0]} {
		if !strings.Contains(view, want) {
			t.Errorf("表示に %q が無い", want)
		}
	}
}

// 入力の破棄の定型文はモーダル側が持つ（呼び出し側は毎回書かない）。
func TestOpenDiscardShowsFixedText(t *testing.T) {
	t.Parallel()

	o := newConfirmOverlay(t)
	OpenDiscard(&o)

	in := discardInput()
	if got := confirmTitle(modelOf(t, o, DiscardKind)); got != in.Title {
		t.Errorf("見出し = %q, want %q", got, in.Title)
	}
	if !strings.Contains(o.View(), in.Impact[0]) {
		t.Errorf("表示に %q が無い", in.Impact[0])
	}
	// 破棄の確認は計画に依らないので、実行するコマンドも対象も載せない。
	if in.Command != nil || in.Targets != nil {
		t.Error("入力の破棄の確認に対象または実行コマンドが載っている")
	}
}

// 見出しとフッタは他のモーダルの Model を渡されても落ちない。
//
// Overlay は登録した関数を Model 付きで呼ぶだけなので、型が合わない場合は
// 空を返すほかに手が無い（panic すると画面ごと落ちる）。
func TestConfirmTitleAndHintsIgnoreForeignModel(t *testing.T) {
	t.Parallel()

	foreign := pagetest.NewEcho()
	if got := confirmTitle(foreign); got != "" {
		t.Errorf("別のモーダルの見出し = %q, want 空", got)
	}
	if got := confirmHints(foreign); got != nil {
		t.Errorf("別のモーダルのフッタ = %v, want nil", got)
	}
}

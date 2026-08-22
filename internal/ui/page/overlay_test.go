package page

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/template"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// newOverlay はモーダルを 1 枚も開いていない状態で返す。
func newOverlay() Overlay {
	o := NewOverlay(testKeys(), testStyles(), true)
	o.SetSize(80, 20)
	return o
}

// sendOverlay はキーを順に送り、最後の Cmd を返す。
func sendOverlay(o Overlay, keys ...string) (Overlay, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		o, cmd = o.Update(press(k))
	}
	return o, cmd
}

// 何も開いていない間はキーを受けても状態が変わらず、描画も空になる。
func TestOverlayInactive(t *testing.T) {
	o := newOverlay()
	if o.Active() {
		t.Fatal("開いていないのに Active が真である")
	}

	o, cmd := sendOverlay(o, "j", "enter", "esc")
	if cmd != nil {
		t.Error("開いていないのに Cmd が発行されている")
	}
	if o.Active() {
		t.Error("キー入力でモーダルが開いている")
	}
	if got := o.View(); got != "" {
		t.Errorf("表示 = %q, want 空", got)
	}
	if o.Hints() != nil {
		t.Error("開いていないのにフッタのヒントがある")
	}
}

// モーダルが 1 枚のとき、キーは最上位にのみ届く。
func TestOverlayDeliversToTopOnly(t *testing.T) {
	o := newOverlay()
	o.OpenDetail(sampleRunner(), fullCaps())
	if !o.Active() {
		t.Fatal("詳細を開いたのに Active が偽である")
	}

	o, _ = sendOverlay(o, "j", "j")
	if got := o.detail.Cursor(); got != 2 {
		t.Fatalf("詳細のカーソル = %d, want 2", got)
	}

	// ヘルプを重ねると、同じキーは最上位（ヘルプ）にのみ渡り、背後の詳細は動かない。
	o.OpenHelp()
	o, _ = sendOverlay(o, "j", "j")
	if got := o.detail.Cursor(); got != 2 {
		t.Errorf("背後の詳細のカーソル = %d, want 2（キーが背後に流れている）", got)
	}
}

// esc は 1 枚だけ閉じる。
func TestOverlayCloseOneByOne(t *testing.T) {
	o := newOverlay()
	o.OpenDetail(sampleRunner(), fullCaps())
	o.OpenHelp()

	o, _ = sendOverlay(o, "esc")
	if !o.Active() {
		t.Fatal("esc で 2 枚とも閉じている")
	}
	if !strings.Contains(o.View(), "build01-1") {
		t.Error("ヘルプを閉じた後に詳細が最上位になっていない")
	}

	o, _ = sendOverlay(o, "esc")
	if o.Active() {
		t.Error("2 回目の esc でモーダルが閉じていない")
	}
}

// 同じ種類のモーダルは重ねず、最上位へ動かす（閉じられないモーダルを作らない）。
func TestOverlayDoesNotStackSameKind(t *testing.T) {
	o := newOverlay()
	o.OpenHelp()
	o.OpenHelp()

	o, _ = sendOverlay(o, "esc")
	if o.Active() {
		t.Error("ヘルプを 2 回開くと esc 1 回で閉じない")
	}
}

// Active は ChromeMsg.Modal に載せる値であり、開閉と一致する。
func TestOverlayActiveMatchesStack(t *testing.T) {
	o := newOverlay()
	steps := []struct {
		do   func(o *Overlay)
		want bool
		name string
	}{
		{func(o *Overlay) { o.OpenDetail(sampleRunner(), fullCaps()) }, true, "詳細を開く"},
		{func(o *Overlay) { o.OpenHelp() }, true, "ヘルプを重ねる"},
		{func(o *Overlay) { o.Close() }, true, "1 枚閉じる"},
		{func(o *Overlay) { o.Close() }, false, "全部閉じる"},
		{func(o *Overlay) { o.Close() }, false, "空でも閉じられる"},
	}
	for _, s := range steps {
		s.do(&o)
		if got := o.Active(); got != s.want {
			t.Errorf("%s の後の Active = %v, want %v", s.name, got, s.want)
		}
	}
}

// フッタのヒントは最上位のモーダルのものになる。
func TestOverlayHints(t *testing.T) {
	o := newOverlay()
	o.OpenDetail(sampleRunner(), fullCaps())
	if got := len(o.Hints()); got != len(o.detail.Hints()) {
		t.Errorf("詳細のヒント件数 = %d, want %d", got, len(o.detail.Hints()))
	}

	o.OpenHelp()
	hints := o.Hints()
	if len(hints) != 1 || hints[0].Key != "esc" {
		t.Errorf("ヘルプのヒント = %+v, want esc のみ", hints)
	}
}

// 背景の明暗が変わったら配色を持つ部品を作り直し、開いている詳細は開き直す。
func TestOverlaySetStateRestyles(t *testing.T) {
	o := newOverlay()
	o.OpenDetail(sampleRunner(), fullCaps())
	o, _ = sendOverlay(o, "j")
	if got := o.detail.Cursor(); got != 1 {
		t.Fatalf("詳細のカーソル = %d, want 1", got)
	}

	st := StateMsg{
		Caps: fullCaps(), Styles: token.NewStyles(false, false), Keys: testKeys(), Dark: false,
		BodyW: 80, BodyH: 20,
	}
	o.SetState(st)
	if !o.Active() {
		t.Error("配色の切り替えでモーダルが閉じている")
	}
	if got := o.detail.Cursor(); got != 0 {
		t.Errorf("開き直した後のカーソル = %d, want 0（安全側へ戻す）", got)
	}
	if !strings.Contains(o.View(), "build01-1") {
		t.Error("開き直した詳細の対象が引き継がれていない")
	}

	// 明暗が変わらない StateMsg では作り直さない（カーソルを保つ）。
	o, _ = sendOverlay(o, "j")
	o.SetState(st)
	if got := o.detail.Cursor(); got != 1 {
		t.Errorf("同じ明暗での Cursor = %d, want 1", got)
	}
}

// 詳細画面の各行はモーダルの中で折り返さない。
//
// SetSize が中身へ配る幅（template.ModalPadding）と template.Modal が枠へ渡す幅が
// 食い違うと、操作リストの行が 2 行に割れて「押せない理由」が読めなくなる（FR-46）。
// 幅を複数点で固定し、真実が 2 箇所に分かれたら落ちるようにする。
//
// 能力と runner の状態の組み合わせも合わせて走査する。理由（screens.md「無効な操作の
// 表示」）は文言ごとに長さが違い、最長の組み合わせだけが幅 80 で溢れていた。
func TestOverlayDetailDoesNotWrapInModal(t *testing.T) {
	const height = 40
	padW, _ := template.ModalPadding()
	for _, width := range []int{60, 72, 80, 100} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			for _, c := range detailCases() {
				o := NewOverlay(testKeys(), testStyles(), true)
				o.SetSize(width, height)
				o.OpenDetail(c.target, c.caps)

				view := o.View()
				for i, line := range strings.Split(view, "\n") {
					if w := lipgloss.Width(line); w > width {
						t.Errorf("%s: %d 行目の表示幅 = %d, want <= %d", c.name, i+1, w, width)
					}
				}
				// 折り返された行は改行が入るため、1 行としては現れない。
				//
				// 内側幅を超える行の除外は不要になった。無効な操作は影響を併記せず、
				// 理由を幅に収める（molecule.ActionRow）ため、全行が内側幅に収まる。
				inner := width - padW
				for _, line := range strings.Split(o.detail.View(), "\n") {
					line = strings.TrimRight(line, " ")
					if line == "" {
						continue
					}
					if w := lipgloss.Width(line); w > inner {
						t.Errorf("%s: 詳細の行 %q の表示幅 = %d, want <= %d", c.name, line, w, inner)
						continue
					}
					if !strings.Contains(view, line) {
						t.Errorf("%s: 詳細の行 %q が折り返している:\n%s", c.name, line, view)
					}
				}
			}
		})
	}
}

// detailCase は詳細画面を開く条件の 1 通り。
type detailCase struct {
	name   string
	target runner.Runner
	caps   appconfig.Caps
}

// detailCases は操作の可否が変わる条件を網羅して返す。
//
// 能力（root / systemd / gh 認証）の有無と runner の状態（systemd 管理・run.sh 直起動・
// ジョブ実行中）を掛け合わせると、screens.md「無効な操作の表示」の理由が一通り現れる。
// docker / journal と sudo ユーザは操作の可否に効かないため振らない（page.Allow）。
func detailCases() []detailCase {
	targets := []struct {
		name string
		r    runner.Runner
	}{
		{"systemd", sampleRunner()},
		{"ジョブ実行中", busyRunner()},
		{"直起動", standaloneRunner()},
	}

	out := make([]detailCase, 0, len(targets)*8)
	for _, tg := range targets {
		for mask := range 8 {
			caps := appconfig.Caps{
				Root:        mask&1 != 0,
				Systemd:     mask&2 != 0,
				GitHubToken: mask&4 != 0,
				Docker:      true,
				Journal:     true,
				SudoUser:    "ousiass",
			}
			out = append(out, detailCase{
				name:   tg.name + "/caps" + strconv.Itoa(mask),
				target: tg.r,
				caps:   caps,
			})
		}
	}
	return out
}

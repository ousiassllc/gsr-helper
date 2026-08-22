package pane_test

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/pane"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// View に全グループのキーと説明が並ぶ。無効なキーも消さない。
//
// 全キー一覧は操作の可否を反映しない。可否は状況で変わるため、一覧の役割は
// キーと動作の対応を示すことに限る（screens.md の無効な操作の表示）。
func TestHelpShowsEveryBinding(t *testing.T) {
	stop := key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "停止"))
	stop.SetEnabled(false)
	groups := append(keymap.New().FullHelp(), []key.Binding{stop})

	h := pane.NewHelp(testStyles(), groups)
	h.SetSize(400, 0)

	got := h.View()
	for _, group := range groups {
		for _, b := range group {
			if !strings.Contains(got, b.Help().Key) || !strings.Contains(got, b.Help().Desc) {
				t.Errorf("キー %q（%s）が一覧に無い", b.Help().Key, b.Help().Desc)
			}
		}
	}
	// 呼び出し側の Binding は書き換えない。
	if stop.Enabled() {
		t.Error("渡された Binding の可否が書き換えられている")
	}
}

// 色を使わない設定では出力に ANSI 列を含まない。bubbles/help の既定スタイルに任せると、
// 色を無効にしても lipgloss のカラープロファイル判定で装飾が入ってしまう。
func TestHelpFollowsColorSetting(t *testing.T) {
	groups := keymap.New().FullHelp()

	plain := pane.NewHelp(testStyles(), groups)
	plain.SetSize(400, 0)
	if got := plain.View(); strings.Contains(got, "\x1b[") {
		t.Errorf("色が無効なのに ANSI 列がある: %q", got)
	}

	colored := pane.NewHelp(token.NewStyles(true, true), groups)
	colored.SetSize(400, 0)
	if got := colored.View(); !strings.Contains(got, "\x1b[") {
		t.Error("色が有効なのに装飾が付いていない")
	}
}

// SetSize が反映される。幅は bubbles/help の列組み、高さは行数の切り詰めに効く。
func TestHelpSetSize(t *testing.T) {
	h := pane.NewHelp(testStyles(), keymap.New().FullHelp())

	h.SetSize(400, 0)
	full := lipgloss.Height(h.View())
	wide := lipgloss.Width(h.View())
	if full < 2 {
		t.Fatalf("全キー一覧の行数 = %d, want 2 以上", full)
	}

	h.SetSize(400, 2)
	if got := lipgloss.Height(h.View()); got != 2 {
		t.Errorf("高さ 2 のときの行数 = %d, want 2", got)
	}

	h.SetSize(400, full+10)
	if got := lipgloss.Height(h.View()); got != full {
		t.Errorf("高さが余るときの行数 = %d, want %d", got, full)
	}

	// 幅を絞ると bubbles/help が収まらない列を落とす。
	h.SetSize(30, 0)
	if got := lipgloss.Width(h.View()); got >= wide {
		t.Errorf("幅 30 のときの表示幅 = %d, want %d 未満", got, wide)
	}
}

package organism_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
)

// detailChoices は詳細画面の操作リストに相当する項目を返す。
//
// 区切り線の下に破壊的な操作を置く並びは screens.md の詳細画面に合わせる。
func detailChoices() []organism.Choice {
	return []organism.Choice{
		{Key: "l", Desc: "ログを開く", Enabled: true},
		{Key: "s", Desc: "開始", Enabled: false, Reason: "稼働中のため不要"},
		{Key: "d", Desc: "ドレイン停止", Enabled: true},
		{Key: "X", Desc: "強制停止", Enabled: true, DividerBefore: true,
			Impact: "⚠ 実行中のジョブは中断されます"},
		{Key: "D", Desc: "削除", Enabled: false, Reason: "ジョブ実行中です"},
	}
}

// newChoices は操作リストを組み立てる。
func newChoices(items []organism.Choice) organism.ChoiceList {
	c := organism.NewChoiceList(keymap.NewList(), testStyles())
	c.SetWidth(72)
	c.SetItems(items)
	return c
}

// sendChoice はキーを順に送り、最後の Cmd を返す。
func sendChoice(c organism.ChoiceList, keys ...string) (organism.ChoiceList, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		c, cmd = c.Update(press(k))
	}
	return c, cmd
}

// chosenKey は Cmd が返す ChosenMsg のキーを取り出す。
func chosenKey(t *testing.T, cmd tea.Cmd) (string, bool) {
	t.Helper()

	if cmd == nil {
		return "", false
	}
	msg, ok := cmd().(organism.ChosenMsg)
	if !ok {
		t.Fatalf("ChosenMsg 以外の Msg が返った（%T）", cmd())
	}
	return msg.Key, true
}

// SetItems はカーソルを先頭（安全側）へ戻す。詳細を開き直すたびにリセットする
// 規則（FR-46）を担保する。一覧の enter → 詳細の enter で破壊的操作に到達しない。
func TestChoiceListResetsCursorOnSetItems(t *testing.T) {
	c, _ := sendChoice(newChoices(detailChoices()), "j", "j", "j")
	if c.Cursor() == 0 {
		t.Fatal("カーソルが動いていない")
	}

	c.SetItems(detailChoices())
	if got := c.Cursor(); got != 0 {
		t.Errorf("開き直した後のカーソル = %d, want 0", got)
	}
}

// j / k で移動し、無効な項目にも止まれる（理由を読めるようにするため）。
func TestChoiceListMovesOntoDisabledItems(t *testing.T) {
	tests := map[string]struct {
		keys []string
		want int
	}{
		"初期位置は先頭":    {nil, 0},
		"無効な項目にも止まる": {[]string{"j"}, 1},
		"区切り線をまたげる":  {[]string{"j", "j", "j"}, 3},
		"末尾を越えない":    {[]string{"j", "j", "j", "j", "j", "j"}, 4},
		"先頭を越えない":    {[]string{"k"}, 0},
		"k で戻る":      {[]string{"j", "j", "k"}, 1},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			c, _ := sendChoice(newChoices(detailChoices()), tt.keys...)
			if got := c.Cursor(); got != tt.want {
				t.Errorf("カーソル = %d, want %d", got, tt.want)
			}
		})
	}
}

// enter と直接キーは、有効な項目でだけ ChosenMsg を発行する。
func TestChoiceListChosen(t *testing.T) {
	tests := map[string]struct {
		keys    []string
		wantKey string
	}{
		"有効な項目で enter":     {[]string{"enter"}, "l"},
		"無効な項目で enter":     {[]string{"j", "enter"}, ""},
		"区切り線の下の項目で enter": {[]string{"j", "j", "j", "enter"}, "X"},
		"有効な項目のキーを直接打つ":    {[]string{"d"}, "d"},
		"無効な項目のキーを直接打つ":    {[]string{"D"}, ""},
		"どの項目にも無いキー":       {[]string{"z"}, ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, cmd := sendChoice(newChoices(detailChoices()), tt.keys...)
			key, ok := chosenKey(t, cmd)
			if ok != (tt.wantKey != "") {
				t.Fatalf("ChosenMsg の発行 = %v, want %v", ok, tt.wantKey != "")
			}
			if key != tt.wantKey {
				t.Errorf("選ばれたキー = %q, want %q", key, tt.wantKey)
			}
		})
	}
}

// 無効な項目も消さずに残し、理由を出す（screens.md の無効な操作の表示）。
// 項目が無い場合は何も描かず、キーにも反応しない。
func TestChoiceListView(t *testing.T) {
	got := newChoices(detailChoices()).View()
	for _, want := range []string{"開始", "稼働中のため不要", "削除", "ジョブ実行中です"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q が描かれていない", want)
		}
	}

	c, cmd := sendChoice(newChoices(nil), "j", "k", "enter", "D")
	if cmd != nil {
		t.Error("項目が無いのに ChosenMsg を発行している")
	}
	if got := c.Cursor(); got != 0 {
		t.Errorf("カーソル = %d, want 0", got)
	}
	if got := c.View(); got != "" {
		t.Errorf("表示 = %q, want 空", got)
	}
}

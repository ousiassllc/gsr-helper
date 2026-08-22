package organism_test

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
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
	c.SetItems(items, organism.ResetCursor)
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

	c.SetItems(detailChoices(), organism.ResetCursor)
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

// 区切り線は各行と同じだけ字下げする（screens.md の詳細画面のモックの `  ────…`）。
//
// 左端から引くと区切り線だけが行より外に出て、操作リストの区切りに見えない。
func TestChoiceListDividerAlignsWithRows(t *testing.T) {
	const width = 72
	c := organism.NewChoiceList(keymap.NewList(), testStyles())
	c.SetWidth(width)
	c.SetItems(detailChoices(), organism.ResetCursor)

	// 添字 3 は最初の破壊的な操作（X）の前に入る区切り線の行。
	divider := strings.Split(c.View(), "\n")[3]
	if want := "  " + strings.Repeat(token.IconDivider, width-2); divider != want {
		t.Errorf("区切り線 = %q, want %q", divider, want)
	}
}

// どの行もカーソル記号を含めて幅に収まる。理由を右端へ寄せた行（添字 1 と 5。区切り線の
// 1 行を挟む）はちょうど幅に収まる。理由は幅に収まらなければ末尾を中略するが、
// 丸ごと消すことはしない（molecule.ActionRow / atom.Justify の契約）。
func TestChoiceListRowsFitWidth(t *testing.T) {
	for _, width := range []int{72, 60} {
		c := organism.NewChoiceList(keymap.NewList(), testStyles())
		c.SetWidth(width)
		c.SetItems(detailChoices(), organism.ResetCursor)

		lines := strings.Split(c.View(), "\n")
		for i, line := range lines {
			w := lipgloss.Width(line)
			if w > width || (w != width && (i == 1 || i == 5)) {
				t.Errorf("幅 %d: %d 行目の幅 = %d（%q）", width, i, w, line)
			}
		}
	}
}

// KeepCursor は内容を差し替えてもカーソル位置を保つ。
//
// 同じ対象の状態が変わっただけ（3 秒ごとの再検出でジョブが始まった等）でカーソルが
// 先頭へ戻ると、操作を選んでいる途中で選択がずれる。本番の呼び出し元は
// page/runnerdetail/detail.go の SetState である（Issue #30）。
func TestChoiceListUpdateItemsKeepsCursor(t *testing.T) {
	c := newChoices(detailChoices())
	c, _ = sendChoice(c, "j", "j")
	if got := c.Cursor(); got != 2 {
		t.Fatalf("カーソル = %d, want 2", got)
	}

	// 同じ件数で内容だけが変わる。
	next := detailChoices()
	next[2].Enabled = false
	next[2].Reason = "ジョブ実行中です"
	c.SetItems(next, organism.KeepCursor)
	if got := c.Cursor(); got != 2 {
		t.Errorf("内容の差し替え後のカーソル = %d, want 2", got)
	}
	if !strings.Contains(c.View(), "ジョブ実行中です") {
		t.Error("差し替えた内容が描かれていない")
	}

	// 件数が減ったら末尾へ丸める（範囲外を指したままにしない）。
	c.SetItems(detailChoices()[:2], organism.KeepCursor)
	if got := c.Cursor(); got != 1 {
		t.Errorf("件数が減った後のカーソル = %d, want 1", got)
	}

	// 空になっても 0 に収まる。
	c.SetItems(nil, organism.KeepCursor)
	if got := c.Cursor(); got != 0 {
		t.Errorf("空にした後のカーソル = %d, want 0", got)
	}

	// ResetCursor（ゼロ値）は逆に先頭（安全側）へ戻す（FR-46）。
	c.SetItems(detailChoices(), organism.ResetCursor)
	c, _ = sendChoice(c, "j", "j")
	c.SetItems(detailChoices(), organism.ResetCursor)
	if got := c.Cursor(); got != 0 {
		t.Errorf("SetItems の後のカーソル = %d, want 0（安全側へ戻していない）", got)
	}
}

// Restyle は配色とキー定義を差し替え、項目とカーソル位置は保つ。
//
// 共有状態は 3 秒ごとに配られるため、作り直すとカーソルが先頭へ戻って操作を
// 選べない。本番の呼び出し元は page/runnerdetail/detail.go の SetState である。
func TestChoiceListRestyleKeepsItemsAndCursor(t *testing.T) {
	c := newChoices(detailChoices())
	c, _ = sendChoice(c, "j", "j")
	before := c.View()

	c.Restyle(keymap.NewList(), token.NewStyles(false, false))
	if got := c.Cursor(); got != 2 {
		t.Errorf("配色の差し替え後のカーソル = %d, want 2", got)
	}
	if lipgloss.Width(c.View()) != lipgloss.Width(before) {
		t.Error("配色の差し替えで項目が失われている")
	}

	// 差し替えたキー定義で動く（Up / Down が別のキーになっても追随する）。
	alt := keymap.NewList()
	alt.Up = key.NewBinding(key.WithKeys("p"))
	alt.Down = key.NewBinding(key.WithKeys("n"))
	c.Restyle(alt, testStyles())
	c, _ = sendChoice(c, "n")
	if got := c.Cursor(); got != 3 {
		t.Errorf("差し替えたキーでの移動後のカーソル = %d, want 3", got)
	}
}

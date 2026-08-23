package dialog_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// cleanupInput は screens.md のクリーンアップの確認に相当する内容を返す。
func cleanupInput() dialog.ConfirmInput {
	return dialog.ConfirmInput{
		Title: "クリーンアップの確認",
		Targets: []string{
			"/opt/runners/build01-1/_work/_temp            1.2G   12,004 ファイル",
			"docker build cache                           12.4G",
		},
		Impact:  []string{"解放見込み: 13.6G"},
		Command: []string{"docker builder prune -f"},
		Note:    []string{"削除したファイルは復元できません。"},
	}
}

// newConfirm は確認ダイアログを組み立てる。
func newConfirm() dialog.Confirm {
	c := dialog.NewConfirm(keymap.New(), testStyles())
	c.SetSize(72, 20)
	c.SetInput(cleanupInput())
	return c
}

// decided は Cmd が返す DecidedMsg を取り出す。発行されていなければ偽を返す。
func decided(t *testing.T, cmd tea.Cmd) (dialog.DecidedMsg, bool) {
	t.Helper()

	if cmd == nil {
		return dialog.DecidedMsg{}, false
	}
	msg, ok := cmd().(dialog.DecidedMsg)
	if !ok {
		t.Fatalf("DecidedMsg 以外の Msg が返った（%T）", cmd())
	}
	return msg, true
}

// mustUpdate はキーを 1 つ送り、返った Cmd を取り出す。
func mustUpdate(c dialog.Confirm, k string) tea.Cmd {
	_, cmd := c.Update(press(k))
	return cmd
}

// y だけが実行、n / esc / enter はキャンセルである（screens.md の確認ダイアログ）。
//
// **enter がキャンセルであることがこのテストの要点である。** 一覧や詳細画面から
// 続けて enter を打っている流れのまま破壊的操作へ到達しない（設計原則 5）。
func TestConfirmDecides(t *testing.T) {
	tests := map[string]struct {
		key  string
		want bool
	}{
		"y で実行":      {"y", true},
		"n でキャンセル":   {"n", false},
		"esc でキャンセル": {"esc", false},
		"enter でキャンセル（連打で進まない）": {"enter", false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, cmd := newConfirm().Update(press(tt.key))
			msg, ok := decided(t, cmd)
			if !ok {
				t.Fatalf("キー %q で決定が発行されなかった", tt.key)
			}
			if msg.Confirmed != tt.want {
				t.Errorf("キー %q の決定 = %v, want %v", tt.key, msg.Confirmed, tt.want)
			}
		})
	}
}

// 確認に関係のないキーでは何も発行しない。
//
// ダイアログを開いている間はキーが背後へ流れない（page/overlay.go）。ここで
// 別の Msg を発行すると、確認を経ずに操作が進む経路ができる。
func TestConfirmIgnoresOtherKeys(t *testing.T) {
	for _, k := range []string{"j", "k", "q", "r", "space", "c", "Y", "N", "tab", "ctrl+c"} {
		if _, cmd := newConfirm().Update(press(k)); cmd != nil {
			t.Errorf("キー %q で Msg が発行された", k)
		}
	}
}

// キー以外の Msg では何も発行しない（大きさの変更などが決定に化けない）。
func TestConfirmIgnoresNonKeyMessages(t *testing.T) {
	if _, cmd := newConfirm().Update(tea.WindowSizeMsg{Width: 80, Height: 24}); cmd != nil {
		t.Error("キー以外の Msg で決定が発行された")
	}
}

// 配色とキー定義の配り直しで内容と決定は変わらない。
//
// 3 秒ごとに配られる共有状態と、起動後に届く背景色（tea.BackgroundColorMsg）で
// Restyle が繰り返し呼ばれる。作り直しに置き換えると確認の途中で内容が消える。
func TestConfirmRestyleKeepsInput(t *testing.T) {
	c := newConfirm()
	c.Restyle(keymap.New(), token.NewStyles(false, true))

	if got := c.Title(); got != cleanupInput().Title {
		t.Errorf("Restyle 後の見出し = %q, want %q", got, cleanupInput().Title)
	}
	msg, ok := decided(t, mustUpdate(c, "y"))
	if !ok || !msg.Confirmed {
		t.Error("Restyle 後に y で実行されない")
	}
}

// 差し替えたキー定義で決定する（キー文字列を埋め込まない）。
func TestConfirmFollowsKeymap(t *testing.T) {
	keys := keymap.New()
	keys.Confirm.Yes.SetKeys("Y")

	c := dialog.NewConfirm(keys, testStyles())
	c.SetInput(cleanupInput())

	if _, cmd := c.Update(press("y")); cmd != nil {
		t.Error("差し替え前のキー（y）で決定が発行された")
	}
	msg, ok := decided(t, mustUpdate(c, "Y"))
	if !ok || !msg.Confirmed {
		t.Error("差し替えたキー（Y）で実行されない")
	}
}

// フッタには実行とキャンセルのキーが出る。説明文は keymap の定義から引く。
func TestConfirmHints(t *testing.T) {
	keys := keymap.New()
	hints := newConfirm().Hints()

	if len(hints) != 2 {
		t.Fatalf("キーヒントの数 = %d, want 2", len(hints))
	}
	want := []struct{ key, desc string }{
		{"y", keys.Confirm.Yes.Help().Desc},
		{"n", keys.Confirm.No.Help().Desc},
	}
	for i, w := range want {
		if hints[i].Key != w.key || hints[i].Desc != w.desc {
			t.Errorf("%d 番目のヒント = %q/%q, want %q/%q", i, hints[i].Key, hints[i].Desc, w.key, w.desc)
		}
		if !hints[i].Enabled {
			t.Errorf("%d 番目のヒントが無効になっている", i)
		}
	}
}

// 高さに収まらない内容は落として中略記号を出す（ダイアログはスクロールしない）。
//
// 描画結果そのものは検証しない。行数の契約だけを見る。
func TestConfirmViewFitsHeight(t *testing.T) {
	c := dialog.NewConfirm(keymap.New(), testStyles())
	c.SetInput(cleanupInput())

	for _, h := range []int{1, 3, 8, 40} {
		c.SetSize(72, h)
		if got := len(strings.Split(c.View(), "\n")); got > h {
			t.Errorf("高さ %d の行数 = %d, want %d 以下", h, got, h)
		}
	}
}

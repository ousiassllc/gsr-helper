package dialog_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
)

// confirmWidth / confirmHeight はモーダルの中身に配られる領域の目安。
// 幅 80 の端末で枠と余白を引いた値に近い大きさを使う。
const (
	confirmWidth  = 72
	confirmHeight = 16
)

// stopInput は runner の停止を確認する中身。4 ブロックすべてを埋める。
func stopInput() dialog.ConfirmInput {
	return dialog.ConfirmInput{
		Title:   "停止の確認",
		Targets: []string{"build01-1", "build01-2"},
		Impact:  []string{"⚠ 実行中のジョブは中断されます"},
		Command: []string{"./svc.sh stop"},
		Note:    []string{"再開するには s を押してください"},
	}
}

// newConfirm は確認ダイアログを組み立てる。
func newConfirm(in dialog.ConfirmInput) dialog.Confirm {
	c := dialog.NewConfirm(keymap.NewConfirm(), testStyles())
	c.SetSize(confirmWidth, confirmHeight)
	c.SetInput(in)

	return c
}

// confirmed は Cmd が返す ConfirmedMsg を取り出す。発行されていなければ偽を返す。
func confirmed(t *testing.T, cmd tea.Cmd) (dialog.ConfirmedMsg, bool) {
	t.Helper()

	if cmd == nil {
		return dialog.ConfirmedMsg{OK: false}, false
	}
	msg, ok := cmd().(dialog.ConfirmedMsg)
	if !ok {
		t.Fatalf("ConfirmedMsg 以外の Msg が返った（%T）", cmd())
	}
	return msg, true
}

// 実行は y だけで、n / esc / enter はいずれもキャンセルになる。
//
// **enter がキャンセル側であることをここで固定する。** 一覧の enter で詳細を開き、
// 詳細の enter で操作を選ぶ流れの勢いのまま確認の enter を押しても破壊的操作が
// 走らないことが、この割り当ての目的である（screens.md の確認ダイアログ）。
// キャンセルも Msg で返す（page が閉じる判断をするため）。
func TestConfirmDecision(t *testing.T) {
	tests := map[string]struct {
		key  string
		want bool
	}{
		"y は実行":        {"y", true},
		"n はキャンセル":     {"n", false},
		"esc はキャンセル":   {"esc", false},
		"enter はキャンセル": {"enter", false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, cmd := newConfirm(stopInput()).Update(press(tt.key))

			got, ok := confirmed(t, cmd)
			if !ok {
				t.Fatal("ConfirmedMsg が発行されていない")
			}
			if got.OK != tt.want {
				t.Errorf("ConfirmedMsg.OK = %v, want %v", got.OK, tt.want)
			}
		})
	}
}

// 決定に関わらないキーでは何も返さない。返すと page が勝手に閉じる。
func TestConfirmIgnoresOtherKeys(t *testing.T) {
	for _, k := range []string{"j", "Y", "N", "space"} {
		t.Run(k, func(t *testing.T) {
			_, cmd := newConfirm(stopInput()).Update(press(k))

			if _, ok := confirmed(t, cmd); ok {
				t.Errorf("%q で ConfirmedMsg が発行された", k)
			}
		})
	}
}

// 空のブロックは見出しごと落ちる。中身の無い見出しだけの行を残さない。
func TestConfirmDropsEmptyBlocks(t *testing.T) {
	in := stopInput()
	in.Targets, in.Command = nil, nil

	view := newConfirm(in).View()

	for _, gone := range []string{"対象:", "実行するコマンド:"} {
		if strings.Contains(view, gone) {
			t.Errorf("空のブロックの見出し %q が残っている:\n%s", gone, view)
		}
	}
	for _, want := range []string{"⚠ 実行中のジョブは中断されます", "再開するには s を押してください"} {
		if !strings.Contains(view, want) {
			t.Errorf("残るはずの行 %q が無い:\n%s", want, view)
		}
	}
	if strings.Contains(view, "\n\n\n") {
		t.Errorf("落としたブロックの空行が残っている:\n%s", view)
	}
}

// 4 ブロックがそろっているときは、見出し・字下げした項目・y/N の行が並ぶ。
func TestConfirmRendersEveryBlock(t *testing.T) {
	view := newConfirm(stopInput()).View()

	want := []string{
		"対象:",
		"  build01-1",
		"  build01-2",
		"⚠ 実行中のジョブは中断されます",
		"実行するコマンド:",
		"  ./svc.sh stop",
		"再開するには s を押してください",
		"実行しますか? [y/N]",
	}
	for _, line := range want {
		if !strings.Contains(view, line) {
			t.Errorf("%q が描かれていない:\n%s", line, view)
		}
	}
	if last := lastLine(view); last != "実行しますか? [y/N]" {
		t.Errorf("最終行 = %q, want %q", last, "実行しますか? [y/N]")
	}
}

// 幅を超える行を出さない。長い対象・コマンドは中略する。
func TestConfirmFitsWidth(t *testing.T) {
	long := strings.Repeat("/opt/runners/build01-very-long-name", 5)
	in := stopInput()
	in.Targets = []string{long}
	in.Command = []string{long}
	in.Impact = []string{strings.Repeat("⚠ 実行中のジョブは中断されます", 5)}
	in.Note = []string{strings.Repeat("復元できません", 20)}

	wantNoWideLine(t, newConfirm(in).View(), confirmWidth)
}

// 高さが足りなくても y/N の行は残る。枠に任せると末尾から落ちて消える行である。
func TestConfirmKeepsPromptWhenHeightIsShort(t *testing.T) {
	c := newConfirm(stopInput())
	c.SetSize(confirmWidth, 3)

	view := c.View()

	if got := len(strings.Split(view, "\n")); got != 3 {
		t.Errorf("行数 = %d, want 3:\n%s", got, view)
	}
	if last := lastLine(view); last != "実行しますか? [y/N]" {
		t.Errorf("最終行 = %q, want %q:\n%s", last, "実行しますか? [y/N]", view)
	}
}

// 見出しは枠（template.Modal）へ渡すために取り出せる。
func TestConfirmTitle(t *testing.T) {
	if got := newConfirm(stopInput()).Title(); got != "停止の確認" {
		t.Errorf("Title() = %q, want %q", got, "停止の確認")
	}
}

// フッタのキーヒントは実行とキャンセルの 2 つ。どちらも押せる状態で返す。
func TestConfirmHints(t *testing.T) {
	hints := newConfirm(stopInput()).Hints()

	if len(hints) != 2 {
		t.Fatalf("キーヒントの数 = %d, want 2（%v）", len(hints), hints)
	}
	for i, want := range []struct{ key, desc string }{{"y", "実行"}, {"n", "キャンセル"}} {
		if hints[i].Key != want.key || hints[i].Desc != want.desc {
			t.Errorf("hints[%d] = %q:%q, want %q:%q", i, hints[i].Key, hints[i].Desc, want.key, want.desc)
		}
		if !hints[i].Enabled {
			t.Errorf("hints[%d] が無効になっている", i)
		}
	}
}

// 配色とキー定義を差し替えても中身は保つ。3 秒ごとの共有状態の配り直しで消えない。
func TestConfirmRestyleKeepsInput(t *testing.T) {
	c := newConfirm(stopInput())

	c.Restyle(keymap.NewConfirm(), testStyles())

	if !strings.Contains(c.View(), "build01-1") {
		t.Errorf("Restyle で中身が消えた:\n%s", c.View())
	}
	if _, ok := confirmed(t, mustUpdate(c, press("y"))); !ok {
		t.Error("Restyle 後に y が効かない")
	}
}

// mustUpdate は Update の Cmd だけを取り出す。
func mustUpdate(c dialog.Confirm, msg tea.Msg) tea.Cmd {
	_, cmd := c.Update(msg)
	return cmd
}

// lastLine は最終行を返す。
func lastLine(view string) string {
	lines := strings.Split(view, "\n")
	return lines[len(lines)-1]
}

package dialog_test

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// diffInput は screens.md の Config タブ「変更内容の確認」に相当する内容を返す。
func diffInput() dialog.DiffApprovalInput {
	return dialog.DiffApprovalInput{
		Path: "/opt/runners/build01-1/.env",
		Diff: []string{
			"  PATH=/usr/local/bin:/usr/bin:/bin",
			"- https_proxy=http://old-proxy:3128",
			"+ https_proxy=http://new-proxy:3128",
			"+ ACTIONS_RUNNER_HOOK_JOB_COMPLETED=/opt/hooks/cleanup.sh",
			"  LANG=ja_JP.UTF-8",
		},
		Backup: "/opt/runners/build01-1/.env.bak",
	}
}

// newDiffApproval は差分承認ダイアログを組み立てる。
func newDiffApproval() dialog.DiffApproval {
	d := dialog.NewDiffApproval(keymap.New(), testStyles())
	d.SetSize(72, 20)
	d.SetInput(diffInput())
	return d
}

// y だけが書き込み、n / esc / enter はキャンセルである（Confirm と同じ意味）。
//
// **enter がキャンセルであることがこのテストの要点である。** フォームの確定から
// 続けて enter を打っている流れのまま書き込みへ到達しない（設計原則 5）。
func TestDiffApprovalDecides(t *testing.T) {
	tests := map[string]struct {
		key  string
		want bool
	}{
		"y で書き込み":               {"y", true},
		"n でキャンセル":              {"n", false},
		"esc でキャンセル":            {"esc", false},
		"enter でキャンセル（連打で進まない）": {"enter", false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			d := newDiffApproval()
			_, cmd := d.Update(press(tt.key))
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

// 承認に関係のないキーとキー以外の Msg では何も発行しない。
//
// 別の Msg を発行すると、承認を経ずに設定が書き込まれる経路ができる。
func TestDiffApprovalIgnoresOtherMessages(t *testing.T) {
	for _, k := range []string{"j", "k", "q", "space", "Y", "N", "tab", "ctrl+c"} {
		if _, cmd := newDiffApproval().Update(press(k)); cmd != nil {
			t.Errorf("キー %q で Msg が発行された", k)
		}
	}
	if _, cmd := newDiffApproval().Update(tea.WindowSizeMsg{Width: 80, Height: 24}); cmd != nil {
		t.Error("キー以外の Msg で決定が発行された")
	}
}

// 対象パスは見出しに、差分・退避先・問いは本文に出る。
//
// 対象パスを見出しへ出すのは、本文が高さ不足で末尾から落ちても「どのファイルへ
// 書くのか」を必ず残すためである（DiffApproval.Title）。
func TestDiffApprovalShowsDiffAndBackup(t *testing.T) {
	d := newDiffApproval()
	in := diffInput()

	if title := d.Title(); !strings.Contains(title, in.Path) {
		t.Errorf("見出し = %q, want 対象パス %q を含む", title, in.Path)
	}
	view := d.View()
	want := append([]string{in.Backup, "[y/N]"}, in.Diff...)
	for _, w := range want {
		if !strings.Contains(view, w) {
			t.Errorf("本文に %q が無い:\n%s", w, view)
		}
	}
	wantNoWideLine(t, view, 72)
}

// 配色とキー定義の配り直しで内容と決定は変わらない（Restyle は作り直しではない）。
func TestDiffApprovalRestyleKeepsInput(t *testing.T) {
	d := newDiffApproval()
	d.Restyle(keymap.New(), token.NewStyles(false, true))

	if !strings.Contains(d.Title(), diffInput().Path) {
		t.Errorf("Restyle 後の見出し = %q, want 対象パスを含む", d.Title())
	}
	_, cmd := d.Update(press("y"))
	if msg, ok := decided(t, cmd); !ok || !msg.Confirmed {
		t.Error("Restyle 後に y で書き込みにならない")
	}
}

// 高さに収まらない差分は落として中略記号を出す（ダイアログはスクロールしない）。
//
// **落とすのは差分の末尾だけで、退避先と問いは残す。** 逆に落ちると、書き込むか
// を尋ねる行の無い画面で y を待つことになる（doc.go の fitHeight）。
func TestDiffApprovalClipsLongDiff(t *testing.T) {
	long := make([]string, 0, 200)
	for i := range 200 {
		long = append(long, "+ KEY"+strconv.Itoa(i)+"=value")
	}
	in := diffInput()
	in.Diff = long

	d := dialog.NewDiffApproval(keymap.New(), testStyles())
	d.SetInput(in)
	for _, h := range []int{1, 2, 5, 12, 40} {
		d.SetSize(72, h)
		view := d.View()
		lines := strings.Split(view, "\n")
		if len(lines) > h {
			t.Errorf("高さ %d の行数 = %d, want %d 以下", h, len(lines), h)
		}
		if !strings.Contains(view, "[y/N]") {
			t.Errorf("高さ %d で問いが落ちた:\n%s", h, view)
		}
		if h >= 5 && !strings.Contains(view, token.IconEllipsis) {
			t.Errorf("高さ %d で中略記号が出ていない:\n%s", h, view)
		}
	}
}

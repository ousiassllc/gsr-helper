package config

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 束を辿る道具の回帰テスト（Issue #140）。
//
// doneOf / savedOf が「見つからない」を返したとき、それが画面の判断（差分が
// 無くて書き込みを飛ばした）なのか、機械が混んで締め切りに間に合わなかった
// だけなのかで、次に見る場所が正反対になる。道具の側で確定させる。

// blocked は戻らない Cmd を返す。
func blocked() tea.Cmd { return func() tea.Msg { select {} } }

// isDone は doneMsg（TabMsg に包まれていても）かを返す。
func isDone(m tea.Msg) bool {
	if tab, wrapped := m.(page.TabMsg); wrapped {
		m = tab.Msg
	}
	_, is := m.(doneMsg)

	return is
}

func TestFindMsgTellsTimeoutApartFromMissingMsg(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		cmd  tea.Cmd
		want error
	}{
		{
			name: "Msg が本当に無ければ見つからない",
			cmd:  func() tea.Msg { return page.ChromeMsg{} },
			want: errNotFound,
		},
		{name: "戻らない Cmd だけなら待ち時間切れ", cmd: blocked(), want: pagetest.ErrCmdTimeout},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := findMsg(tt.cmd, 10*time.Millisecond, isDone); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

// 1 本諦めても束の残りを辿ること（Issue #140）。
//
// 待ち時間切れで打ち切ると、戻らない Cmd が先に並んだ束で本体の結果を取り
// こぼす。取りこぼしは「書き込みが走っていない」と同じ見た目になる。
func TestFindMsgKeepsScanningAfterATimeout(t *testing.T) {
	t.Parallel()

	done := func() tea.Msg { return doneMsg{text: "書き込みました", err: nil} }

	got, err := findMsg(tea.Batch(blocked(), done), 10*time.Millisecond, isDone)
	if err != nil {
		t.Fatalf("戻らない Cmd の後ろにある結果を取れない: %v", err)
	}
	if msg, is := got.(doneMsg); !is || msg.text != "書き込みました" {
		t.Errorf("見つけた Msg = %#v, want doneMsg{text: 書き込みました}", got)
	}
}

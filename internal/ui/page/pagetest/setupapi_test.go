package pagetest_test

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// ChromeAfter（領域を配り直して状態行とフッタを読む道具）の検証を集める。
//
// **委譲先が固めてあることは、こちらの腕が正しいことを意味しない。** ChromeAfter は
// cmdtest.ChromeOf への 2 行の委譲だが、返しを取り違えて包み直す・締め切りを渡し
// 忘れるといった誤りはこの 2 行の側にしか現れない。どちらの腕も呼び出し側が
// 「状態行が出ていない」と「辿り切れなかった」を読み分ける材料になるので、
// 取り違えると失敗メッセージが原因と違う場所を指す（Issue #157）。

// chromeModel は Update のたびに決まった Cmd を返すだけの tea.Model。
//
// タブの具体型を持ち出さずに ChromeAfter の腕を突けるようにするために置く。
type chromeModel struct{ cmd tea.Cmd }

func (m chromeModel) Init() tea.Cmd                       { return nil }
func (m chromeModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, m.cmd }
func (m chromeModel) View() tea.View                      { return tea.NewView("") }

// 状態行が発行されていなければ ErrNotFound、辿り切れなければ ErrCmdTimeout を返す。
func TestChromeAfterTellsMissingApartFromTimeout(t *testing.T) {
	t.Parallel()

	status := "見つかった"
	for _, tt := range []struct {
		name string
		cmd  tea.Cmd
		want error
	}{
		{
			name: "束の中にあれば取り出す",
			cmd:  func() tea.Msg { return page.ChromeMsg{Status: status} },
			want: nil,
		},
		{
			name: "発行されていなければ見つからない",
			cmd:  func() tea.Msg { return struct{}{} },
			want: cmdtest.ErrNotFound,
		},
		{
			name: "Cmd が無ければ見つからない",
			cmd:  nil,
			want: cmdtest.ErrNotFound,
		},
		// 諦めるまでの時間がそのまま所要時間になるので、締め切りはミリ秒で渡す
		// （Issue #156 と同じ理由）。
		{
			name: "戻らない Cmd は待ち時間切れ",
			cmd:  func() tea.Msg { select {} },
			want: cmdtest.ErrCmdTimeout,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := pagetest.ChromeAfter(chromeModel{cmd: tt.cmd}, 10*time.Millisecond)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if tt.want == nil && got.Status != status {
				t.Errorf("ChromeMsg.Status = %q, want %q", got.Status, status)
			}
		})
	}
}

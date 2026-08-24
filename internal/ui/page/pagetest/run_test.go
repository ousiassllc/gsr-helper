package pagetest_test

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 待ち時間切れと「Cmd が無い」を区別して返すこと（Issue #140）。
//
// 以前はどちらも同じ偽で返していたため、`make check` の並列負荷で締め切りに
// 間に合わなかっただけの Cmd を呼び出し側が「そもそも Cmd が出ていない」と読み、
// 失敗メッセージが原因と違う場所（差分の判定）を指していた。
func TestRunCmdTellsTimeoutApartFromMissingCmd(t *testing.T) {
	t.Parallel()

	type marker struct{}

	for _, tt := range []struct {
		name string
		cmd  tea.Cmd
		want error
	}{
		{name: "Cmd が無い", cmd: nil, want: pagetest.ErrNoCmd},
		{name: "戻らない Cmd は待ち時間切れ", cmd: func() tea.Msg { select {} }, want: pagetest.ErrCmdTimeout},
		{name: "戻る Cmd は Msg を返す", cmd: func() tea.Msg { return marker{} }, want: nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			msg, err := pagetest.RunCmd(tt.cmd, 10*time.Millisecond)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if _, is := msg.(marker); tt.want == nil && !is {
				t.Errorf("Msg = %T, want %T", msg, marker{})
			}
		})
	}
}

// blocked は戻らない Cmd を返す。
func blocked() tea.Cmd { return func() tea.Msg { select {} } }

// marker は束の中から探す目印の Msg。
type marker struct{ n int }

// isMarker は marker かを返す。
func isMarker(m tea.Msg) bool { _, is := m.(marker); return is }

// 待ち時間切れと「目当ての Msg が無い」を区別して返すこと（Issue #140）。
//
// どちらを返すかで次に見る場所が正反対になる。ErrNotFound は Msg を出す側の
// 判断（差分が無いなど）を、ErrCmdTimeout は機械の混み具合を疑う合図である。
func TestFindMsgTellsTimeoutApartFromMissingMsg(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		cmd  tea.Cmd
		want error
	}{
		{
			name: "Msg が本当に無ければ見つからない",
			cmd:  func() tea.Msg { return struct{}{} },
			want: pagetest.ErrNotFound,
		},
		{name: "戻らない Cmd だけなら待ち時間切れ", cmd: blocked(), want: pagetest.ErrCmdTimeout},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := pagetest.FindMsg(tt.cmd, 10*time.Millisecond, isMarker); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

// 1 本諦めても束の残りを辿ること（Issue #140）。
//
// 待ち時間切れで打ち切ると、戻らない Cmd が先に並んだ束で目当てを取りこぼす。
// 取りこぼしは「そもそも Msg が出ていない」と同じ見た目になる。
func TestFindMsgKeepsScanningAfterATimeout(t *testing.T) {
	t.Parallel()

	found := func() tea.Msg { return marker{n: 7} }

	got, err := pagetest.FindMsg(tea.Batch(blocked(), found), 10*time.Millisecond, isMarker)
	if err != nil {
		t.Fatalf("戻らない Cmd の後ろにある Msg を取れない: %v", err)
	}
	if m, is := got.(marker); !is || m.n != 7 {
		t.Errorf("見つけた Msg = %#v, want marker{n: 7}", got)
	}
}

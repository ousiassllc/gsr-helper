package cmdtest_test

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
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
		{name: "Cmd が無い", cmd: nil, want: cmdtest.ErrNoCmd},
		{name: "戻らない Cmd は待ち時間切れ", cmd: func() tea.Msg { select {} }, want: cmdtest.ErrCmdTimeout},
		{name: "戻る Cmd は Msg を返す", cmd: func() tea.Msg { return marker{} }, want: nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			msg, err := cmdtest.RunCmd(tt.cmd, 10*time.Millisecond)
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
			want: cmdtest.ErrNotFound,
		},
		{name: "戻らない Cmd だけなら待ち時間切れ", cmd: blocked(), want: cmdtest.ErrCmdTimeout},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := cmdtest.FindMsg(tt.cmd, 10*time.Millisecond, isMarker); !errors.Is(err, tt.want) {
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

	got, err := cmdtest.FindMsg(tea.Batch(blocked(), found), 10*time.Millisecond, isMarker)
	if err != nil {
		t.Fatalf("戻らない Cmd の後ろにある Msg を取れない: %v", err)
	}
	if m, is := got.(marker); !is || m.n != 7 {
		t.Errorf("見つけた Msg = %#v, want marker{n: 7}", got)
	}
}

// 束になっていない「戻らない Cmd」を渡しても止まらず、待ち時間切れとして返すこと
// （Issue #145）。
//
// Expand が先頭の Cmd を締め切り無しで走らせていたころ、この形の Cmd を渡した
// パッケージは `go test` の既定のタイムアウト（10 分）まで戻らなかった。**壊れ方が
// 失敗ではなくハングになる**ため、CI では原因の分からない停止に見える。
func TestExpandGivesUpOnCmdThatNeverReturns(t *testing.T) {
	t.Parallel()

	cmds, err := cmdtest.Expand(blocked(), 10*time.Millisecond)
	if !errors.Is(err, cmdtest.ErrCmdTimeout) {
		t.Fatalf("err = %v, want %v", err, cmdtest.ErrCmdTimeout)
	}
	if cmds != nil {
		t.Errorf("待ち時間切れで Cmd を %d 本返している, want 0", len(cmds))
	}
}

// 締め切りを足しても、束の展開そのものは変わらないこと（Issue #145）。
//
// 中の Cmd は実行しない（実行すると Tick が自動更新の間隔だけ待つ）ので、本数だけを見る。
func TestExpandUnwrapsBundles(t *testing.T) {
	t.Parallel()

	one := func() tea.Msg { return marker{n: 1} }

	for _, tt := range []struct {
		name string
		cmd  tea.Cmd
		want int
	}{
		{name: "Cmd が無ければ空", cmd: nil, want: 0},
		{name: "束は中身を返す", cmd: tea.Batch(one, one, one), want: 3},
		{name: "束でなければ Cmd 自身を返す", cmd: one, want: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmds, err := cmdtest.Expand(tt.cmd, cmdtest.CmdTimeout)
			if err != nil {
				t.Fatalf("束を展開できない: %v", err)
			}
			if len(cmds) != tt.want {
				t.Fatalf("展開した Cmd の本数 = %d, want %d", len(cmds), tt.want)
			}
			// 束でない Cmd は「実行済みの Msg を包み直したもの」ではなく cmd 自身が
			// 返る（呼び出し側はもう 1 度走らせる。Expand の doc）。
			if tt.want == 1 {
				if got, is := cmds[0]().(marker); !is || got.n != 1 {
					t.Errorf("返った Cmd の Msg = %#v, want marker{n: 1}", cmds[0]())
				}
			}
		})
	}
}

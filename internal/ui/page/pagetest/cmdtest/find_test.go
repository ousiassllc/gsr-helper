package cmdtest_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// find.go（走らせた束から目当ての 1 件を拾う道具）の検証を集める。
//
// **run_test.go から分けてあるのは 1 ファイル 300 行の上限のためである**
// （find.go 自身を分けたのと同じ理由）。共有の道具（blocked / marker / assertNested）は
// 同じパッケージの run_test.go に置いたままで、こちらからも使う。

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

// 戻らない Cmd を含む束でも ChromeOf が戻り、「無い」と「戻らない」を分けて
// 返すこと（Issue #150）。
//
// 素で走らせていたころは束の途中で止まり、壊れ方が失敗ではなくハングだった。
// 待ち時間切れを ErrNotFound に丸めると、呼び出し側は「page が状態行を発行して
// いない」という実際には無い欠落を疑うことになる（Issue #140 と同じ取り違え）。
func TestChromeOfTellsTimeoutApartFromMissingChrome(t *testing.T) {
	t.Parallel()

	chrome := func() tea.Msg { return page.ChromeMsg{Status: "見つかった"} }
	other := func() tea.Msg { return marker{} }

	for _, tt := range []struct {
		name string
		cmd  tea.Cmd
		want error
	}{
		{
			name: "ChromeMsg が本当に無ければ見つからない",
			cmd:  other,
			want: cmdtest.ErrNotFound,
		},
		{
			// 束のまま辿り切る経路を通す（Cmd 1 本の束は tea.Batch が畳む）。
			name: "束を最後まで辿っても無ければ見つからない",
			cmd:  tea.Batch(other, other),
			want: cmdtest.ErrNotFound,
		},
		{name: "戻らない Cmd だけなら待ち時間切れ", cmd: blocked(), want: cmdtest.ErrCmdTimeout},
		{
			name: "戻らない Cmd の後ろにあっても見つける",
			cmd:  tea.Batch(blocked(), chrome),
			want: nil,
		},
		{
			name: "入れ子の束の中にあっても見つける",
			cmd:  assertNested(t, tea.Batch(blocked(), tea.Batch(blocked(), chrome))),
			want: nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := cmdtest.ChromeOf(tt.cmd, 10*time.Millisecond)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if tt.want == nil && got.Status != "見つかった" {
				t.Errorf("ChromeMsg.Status = %q, want 見つかった", got.Status)
			}
		})
	}
}

// 束の中に戻らない Cmd が混じっても HostReqOf が戻り、諦めた本数を伝えること
// （Issue #145）。
//
// 先頭だけに締め切りを掛けていたころは 2 本目以降で止まり、壊れ方が失敗ではなく
// ハングだった。諦めたことを ErrNotFound に丸めると、呼び出し側は「件数が親へ
// 届いていない」という実際には無い配送の欠落を疑う。
//
// **締め切りはミリ秒で渡す。** 諦めるまでの時間がそのまま所要時間になる検証なので、
// HostReqOf が cmdtest.CmdTimeout（30 秒）を内側で固定していたころは、この 1 本だけで
// パッケージのテストが 30 秒かかっていた（Issue #156）。ここで待つのは戻らない Cmd を
// 諦めるまでの分だけであり、速さを測っているわけではないので短くしてよい。
func TestHostReqOfGivesUpOnBlockedCmd(t *testing.T) {
	t.Parallel()

	cmd := tea.Batch(blocked(), func() tea.Msg { return marker{} })
	_, err := cmdtest.HostReqOf(cmd, 10*time.Millisecond)
	if !errors.Is(err, cmdtest.ErrCmdTimeout) {
		t.Fatalf("err = %v, want %v", err, cmdtest.ErrCmdTimeout)
	}
	if !strings.Contains(err.Error(), "1 本") {
		t.Errorf("err = %v, 諦めた本数を伝えていない", err)
	}
	// 渡した締め切りが実際に使われたことを、報告された長さで確かめる。内側で
	// CmdTimeout を固定していると 30 秒と報告され、ここで落ちる（Issue #156）。
	if !strings.Contains(err.Error(), "10ms") {
		t.Errorf("err = %v, 渡した締め切り（10ms）が使われていない", err)
	}
}

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

// guardTimeout は下の見張りが束を評価する上限。
//
// **速さの検証ではないので負荷の側へ倒す**（cmdtest の CmdTimeout と同じ考え方）。
// ここで待つ Cmd は即座に戻るものだけで、締め切りが効くのは平坦化された cmd が
// 戻らない Cmd だったときだけである。詰めると `make check`（-race で全パッケージを
// 同時に実行）の負荷でだけ落ちる不安定なテストになる（Issue #140 の前例）。
const guardTimeout = 2 * time.Second

// assertBundle は cmd が束（tea.Batch / tea.Sequence）のままであることを確かめて cmd を返す。
//
// **どちらも非 nil の Cmd が 1 本だけなら束を作らずその Cmd を返す**（bubbletea の
// compactCmds）。1 本しか渡さない束は IsQuit の `!isBundle` の早戻りへ落ち、名前が
// 主張する「束を辿る経路」を 1 度も通らないまま緑になる（Issue #150）。
//
// 評価を cmdtest.RunCmd に通すのは、平坦化された cmd が戻らない Cmd に化けたとき、
// 素で呼ぶと失敗ではなくハングになるためである。
func assertBundle(t *testing.T, cmd tea.Cmd) tea.Cmd {
	t.Helper()

	if cmd == nil {
		t.Fatal("見張りに nil の Cmd を渡している")
	}
	msg, err := cmdtest.RunCmd(cmd, guardTimeout)
	if errors.Is(err, cmdtest.ErrCmdTimeout) {
		t.Fatalf("束を評価できなかった（%s 以内に戻らない）", guardTimeout)
	}
	if _, isBundle := cmdtest.Cmds(msg); !isBundle {
		t.Fatal("束が平坦化されている（非 nil の Cmd が 1 本）：束を辿る経路を通らない")
	}

	return cmd
}

// 終了は後始末を流し切ってから行うため tea.Sequence に包まれる（ui/keys.go の quit）。
// その後始末に戻らない Cmd が混じっても IsQuit が戻ること（Issue #150）。
//
// 素で走らせていたころは包みの中で止まり、壊れ方が失敗ではなくハングだった。
// **待ち時間切れを偽に丸めない**のも要点である——丸めると呼び出し側は「q が終了に
// 繋がっていない」という実際には無い退行を疑う（Issue #140 と同じ取り違え）。
func TestIsQuitGivesUpOnBlockedCleanup(t *testing.T) {
	t.Parallel()

	blocked := func() tea.Msg { select {} }

	for _, tt := range []struct {
		name     string
		cmd      tea.Cmd
		wantQuit bool
		wantErr  error
	}{
		{name: "Cmd が無ければ終了ではない", cmd: nil, wantQuit: false, wantErr: nil},
		{name: "包まれていない終了も見つける", cmd: tea.Quit, wantQuit: true, wantErr: nil},
		{
			name: "戻らない後始末の後ろにある終了も見つける",
			cmd:  tea.Sequence(blocked, tea.Quit),
			// 見つかったので諦めた本数は伏せる（呼び出し側は err だけ見れば足りる）。
			wantQuit: true, wantErr: nil,
		},
		{
			name:     "戻らない後始末だけなら待ち時間切れ",
			cmd:      tea.Sequence(blocked, blocked),
			wantQuit: false, wantErr: cmdtest.ErrCmdTimeout,
		},
		{
			name: "終了を含まない束は終了ではない",
			cmd: assertBundle(t, tea.Sequence(
				func() tea.Msg { return struct{}{} },
				func() tea.Msg { return struct{}{} },
			)),
			wantQuit: false, wantErr: nil,
		},
		{
			name:     "先頭の Cmd が戻らなければ待ち時間切れ",
			cmd:      blocked,
			wantQuit: false, wantErr: cmdtest.ErrCmdTimeout,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			quit, err := pagetest.IsQuit(tt.cmd, 10*time.Millisecond)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if quit != tt.wantQuit {
				t.Errorf("終了したか = %v, want %v", quit, tt.wantQuit)
			}
		})
	}
}

// OpenTabOf が「移動の指示が無い」と「辿り切れない」を分けて返すこと（Issue #150）。
//
// 戻りが bool から error になったが、通しの検証（ui/route_test.go）が触るのは見つかる
// 経路だけである。1 つの偽に丸めると、待ち時間切れを呼び出し側が「タブを開く指示が
// 出ていない」と読み、実際には無い配線の欠落を疑うことになる（Issue #140 と同じ取り違え）。
func TestOpenTabOfTellsMissingApartFromTimeout(t *testing.T) {
	t.Parallel()

	open := func() tea.Msg { return page.OpenTabMsg{Title: page.TabLogs, Msg: nil} }

	for _, tt := range []struct {
		name string
		cmd  tea.Cmd
		want error
	}{
		{
			name: "束の中にあれば取り出す",
			cmd:  tea.Batch(func() tea.Msg { return struct{}{} }, open),
			want: nil,
		},
		{
			name: "指示が無ければ見つからない",
			cmd:  func() tea.Msg { return struct{}{} },
			want: cmdtest.ErrNotFound,
		},
		// OpenTabOf に待ち時間の引数は無く、cmdtest.CmdTimeout（30 秒）を使う。
		// 待つのはこの 1 件だけであり、他のケースと並行に走らせて所要時間を重ねない。
		{
			name: "戻らない Cmd は待ち時間切れ",
			cmd:  func() tea.Msg { select {} },
			want: cmdtest.ErrCmdTimeout,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := pagetest.OpenTabOf(tt.cmd)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if tt.want == nil && got.Title != page.TabLogs {
				t.Errorf("移動先 = %q, want %q", got.Title, page.TabLogs)
			}
		})
	}
}

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

// nestTimeout は assertNested が束を評価する上限。
//
// **速さの検証ではないので負荷の側へ倒す**（run.go の CmdTimeout と同じ考え方）。
// ここで待つ Cmd は即座に戻るものだけであり、締め切りが効くのは平坦化された束の
// 最後の要素が戻らない Cmd だったときだけである。詰めると `make check`（-race で
// 全パッケージを同時に実行）の負荷でだけ落ちる不安定なテストになる（Issue #140 で
// 3 秒の締め切りが使い切られた前例がある）。CmdTimeout の 30 秒より十分短ければ
// 「ハングではなく失敗で止める」という目的は果たせる。
const nestTimeout = 2 * time.Second

// assertNested は cmd が入れ子の束（束の最後の要素がまた束）であることを確かめて cmd を返す。
//
// **tea.Batch は非 nil の Cmd が 1 本だけなら束を作らずその Cmd を返す**
// （bubbletea の compactCmds）。内側の Batch へ 1 本しか渡さない tea.Batch(a, tea.Batch(b)) は
// 平坦な 2 要素の束に化けるため、束を再帰的に辿る経路（chromeOf / collectMsgs）を
// 1 段も通らないまま緑になる。入れ子であることをここで固定する。
//
// **最後の要素しか実行しない。** 束の Cmd を呼んでも中の Cmd は走らず並びが返るだけ
// なので、戻らない Cmd を前に置いた束でも安全に確かめられる——内側の束は末尾に置くこと。
//
// **評価は RunCmd を通す**（run.go の「諦める判断は RunCmd に集める」）。平坦化されて
// 最後の要素が呼び出し側の Cmd に化けるのがここで捕まえたい壊れ方そのものであり、
// 素で呼ぶとその Cmd が戻らないときに失敗ではなくハングになる（Issue #150）。
func assertNested(t *testing.T, cmd tea.Cmd) tea.Cmd {
	t.Helper()

	if cmd == nil {
		t.Fatal("見張りに nil の Cmd を渡している（tea.Batch へ非 nil の Cmd が 1 本も無い）")
	}
	msg, err := cmdtest.RunCmd(cmd, nestTimeout)
	if errors.Is(err, cmdtest.ErrCmdTimeout) {
		t.Fatalf("外側の束を評価できなかった（%s 以内に戻らない）", nestTimeout)
	}
	outer, isBundle := cmdtest.Cmds(msg)
	if !isBundle {
		t.Fatal("外側が束になっていない（tea.Batch が Cmd 1 本の束を畳んだ）")
	}
	if len(outer) == 0 {
		t.Fatal("外側の束が空である：入れ子を確かめられない")
	}

	last, err := cmdtest.RunCmd(outer[len(outer)-1], nestTimeout)
	if errors.Is(err, cmdtest.ErrCmdTimeout) {
		t.Fatalf("内側の束を評価できなかった（%s 以内に戻らない）", nestTimeout)
	}
	if _, nested := cmdtest.Cmds(last); !nested {
		t.Fatal("内側の束が平坦化されている：束を再帰的に辿る経路を通らない")
	}

	return cmd
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

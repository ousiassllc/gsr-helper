package cmdtest_test

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// 戻らない Cmd を含む束でも RunAll が戻ること（Issue #150）。
//
// 素で走らせていたころは束の途中で止まり、壊れ方が失敗ではなくハングだった。
// CI では `go test` の既定のタイムアウト（10 分）ぶんの原因の分からない停止に
// 見えるため、諦めたことを error で返させる。
func TestRunAllGivesUpOnBlockedCmd(t *testing.T) {
	t.Parallel()

	ran := 0
	err := cmdtest.RunAll(tea.Batch(
		blocked(),
		func() tea.Msg { ran++; return marker{n: 1} },
	), 10*time.Millisecond)

	if !errors.Is(err, cmdtest.ErrCmdTimeout) {
		t.Fatalf("err = %v, want %v", err, cmdtest.ErrCmdTimeout)
	}
	// 1 本諦めても残りは走らせる（走らせること自体が RunAll の目的である）。
	if ran != 1 {
		t.Errorf("諦めた後に走った Cmd = %d 本, want 1", ran)
	}
}

// 戻らない Cmd を含む束でも Msgs が戻り、残りの Msg は落とさないこと（Issue #150）。
//
// **諦めたことを error で返すのが要点である。** 黙って欠かすと、届かなかった Msg を
// 呼び出し側が「発行されていない」と読み、実際には無い配送の欠落を疑う失敗
// メッセージが出る（Issue #140 / #145 と同じ取り違え）。
func TestMsgsGivesUpOnBlockedCmdAndKeepsTheRest(t *testing.T) {
	t.Parallel()

	got, err := cmdtest.Msgs(tea.Batch(
		func() tea.Msg { return marker{n: 1} },
		blocked(),
		func() tea.Msg { return marker{n: 2} },
	), 10*time.Millisecond)

	if !errors.Is(err, cmdtest.ErrCmdTimeout) {
		t.Fatalf("err = %v, want %v", err, cmdtest.ErrCmdTimeout)
	}
	want := []marker{{n: 1}, {n: 2}}
	if len(got) != len(want) {
		t.Fatalf("辿れた Msg = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != tea.Msg(w) {
			t.Errorf("Msg[%d] = %v, want %v", i, got[i], w)
		}
	}
}

// 戻る Cmd だけの束では Msgs が error を返さないこと。
//
// 待ち時間切れを常に返すようになると、呼び出し側の t.Fatalf がすべて発火して
// 「諦めた」と「そもそも Msg が無い」の区別が付かなくなる。
func TestMsgsReturnsNoErrorWhenEveryCmdReturns(t *testing.T) {
	t.Parallel()

	got, err := cmdtest.Msgs(tea.Batch(
		func() tea.Msg { return marker{n: 1} },
		tea.Batch(func() tea.Msg { return marker{n: 2} }),
	), 10*time.Millisecond)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(got) != 2 {
		t.Errorf("辿れた Msg = %v, want 2 件（入れ子の束も辿る）", got)
	}
}

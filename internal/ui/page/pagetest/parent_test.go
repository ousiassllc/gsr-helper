package pagetest_test

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

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
			name:     "終了を含まない束は終了ではない",
			cmd:      tea.Sequence(func() tea.Msg { return struct{}{} }),
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

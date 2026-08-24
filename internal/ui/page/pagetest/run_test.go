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

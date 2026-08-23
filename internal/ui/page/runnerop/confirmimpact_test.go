package runnerop

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 確認ダイアログに影響行が出ることを、組んだ中身（ConfirmInput）ではなく実際に
// 描かれた文字列で固定する。
//
// dialog.Confirm は空のブロックを見出しごと落とすため、action.Def.Impact が空だと
// 影響のブロックは組んだ時点では「空の並び」で、描画で跡形も無く消える。以前は
// 停止と再起動がこれに当たり、対象がジョブ実行中でない限り影響行が 1 行も出て
// いなかった（Issue #5 の受け入れ条件「x / X / R は確認ダイアログ（影響表示 +
// y/N、既定 N）を経る」に反する）。

// confirmView は操作と対象から確認ダイアログのモーダルを描いた文字列を返す。
func confirmView(t *testing.T, id action.ID, targets []runner.Runner) string {
	t.Helper()

	st := pagetest.State(80, 24)
	m := NewConfirmModal(st).Model
	m, _ = m.Update(page.SizeMsg{W: 72, H: 16})
	m, _ = m.Update(confirmInput(defOf(t, id), targets))

	return m.View().Content
}

// defOf は操作の定義を引く。
func defOf(t *testing.T, id action.ID) action.Def {
	t.Helper()

	for _, d := range action.NewSet(keymap.NewRunnerKeys()).List() {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("操作 %s の定義が無い", id)
	return action.Def{}
}

// x / X / R の確認ダイアログには、対象がジョブ実行中でなくても影響行が出る。
//
// 文言は screens.md の「Runners タブの操作」の表に合わせる（`x` / `R` は
// 「実行中ジョブに影響する可能性」、`X` は「⚠ 実行中のジョブは中断されます」）。
func TestConfirmViewShowsImpact(t *testing.T) {
	// ジョブ実行中でない対象。実行中なら影響行は警告（ジョブ実行中: …）だけでも
	// 出てしまい、操作そのものの影響が抜けていることを検出できない。
	idle := []runner.Runner{testRunner("build01-1")}

	tests := map[string]struct {
		id    action.ID
		title string
		want  string
	}{
		"停止":   {action.Stop, "停止の確認", "実行中ジョブに影響する可能性"},
		"再起動":  {action.Restart, "再起動の確認", "実行中ジョブに影響する可能性"},
		"強制停止": {action.Kill, "強制停止の確認", "実行中のジョブは中断されます"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			body := confirmView(t, tt.id, idle)
			if !strings.Contains(body, tt.want) {
				t.Errorf("%s の確認に影響行 %q が無い:\n%s", tt.title, tt.want, body)
			}
			// 影響のブロックだけでなく、確認の必須段（対象・コマンド・y/N）も残る。
			if !strings.Contains(body, "build01-1") {
				t.Errorf("%s の確認に対象が無い:\n%s", tt.title, body)
			}
		})
	}
}

// ジョブ実行中の対象では、操作の影響に加えて警告とドレインの案内が並ぶ。
//
// 操作の影響を足したことで警告が押し出されていないことを確かめる。
func TestConfirmViewKeepsBusyWarning(t *testing.T) {
	body := confirmView(t, action.Stop, []runner.Runner{busy("build01-2")})

	for _, want := range []string{"実行中ジョブに影響する可能性", "build01-2", "ドレイン停止"} {
		if !strings.Contains(body, want) {
			t.Errorf("停止の確認に %q が無い:\n%s", want, body)
		}
	}
}

// 安全な操作（開始・ドレイン停止・enable の切替）は影響の文言を持たない。
//
// 毎回何かしらの影響が出ると読み飛ばされる。確認ダイアログを経るのは x / X / R
// だけであり（needsConfirm）、それ以外に影響を足す必要は無い。
func TestSafeActionsHaveNoImpact(t *testing.T) {
	for _, id := range []action.ID{action.Start, action.Drain, action.Enable} {
		in := confirmInput(defOf(t, id), []runner.Runner{testRunner("build01-1")})
		if len(in.Impact) != 0 {
			t.Errorf("%s の影響 = %v, want 無し", id, in.Impact)
		}
	}
}

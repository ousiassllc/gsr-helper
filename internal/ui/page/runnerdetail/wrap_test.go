package runnerdetail

import (
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/template"
)

// 詳細画面の各行はモーダルの中で折り返さない。
//
// SetSize が中身へ配る幅（template.ModalPadding）と template.Modal が枠へ渡す幅が
// 食い違うと、操作リストの行が 2 行に割れて「押せない理由」が読めなくなる（FR-46）。
// 幅を複数点で固定し、真実が 2 箇所に分かれたら落ちるようにする。
//
// 能力と runner の状態の組み合わせも合わせて走査する。理由（screens.md「無効な操作の
// 表示」）は文言ごとに長さが違い、最長の組み合わせだけが幅 80 で溢れていた。
func TestOverlayDetailDoesNotWrapInModal(t *testing.T) {
	const height = 40
	padW, _ := template.ModalPadding()
	for _, width := range []int{60, 72, 80, 100} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			for _, c := range detailCases() {
				o := newOverlayWithDetail(width, height)
				Open(&o, c.target, c.caps)

				view := o.View()
				for i, line := range strings.Split(view, "\n") {
					if w := lipgloss.Width(line); w > width {
						t.Errorf("%s: %d 行目の表示幅 = %d, want <= %d", c.name, i+1, w, width)
					}
				}
				// 折り返された行は改行が入るため、1 行としては現れない。
				//
				// 内側幅を超える行の除外は不要になった。無効な操作は影響を併記せず、
				// 理由を幅に収める（molecule.ActionRow）ため、全行が内側幅に収まる。
				inner := width - padW
				for _, line := range strings.Split(detailOf(t, o).View(), "\n") {
					line = strings.TrimRight(line, " ")
					if line == "" {
						continue
					}
					if w := lipgloss.Width(line); w > inner {
						t.Errorf("%s: 詳細の行 %q の表示幅 = %d, want <= %d", c.name, line, w, inner)
						continue
					}
					if !strings.Contains(view, line) {
						t.Errorf("%s: 詳細の行 %q が折り返している:\n%s", c.name, line, view)
					}
				}
			}
		})
	}
}

// detailCase は詳細画面を開く条件の 1 通り。
type detailCase struct {
	name   string
	target runner.Runner
	caps   appconfig.Caps
}

// detailCases は操作の可否が変わる条件を網羅して返す。
//
// 能力（root / systemd / gh 認証）の有無と runner の状態（systemd 管理・run.sh 直起動・
// ジョブ実行中）を掛け合わせると、screens.md「無効な操作の表示」の理由が一通り現れる。
// docker / journal と sudo ユーザは操作の可否に効かないため振らない（page.Allow）。
func detailCases() []detailCase {
	targets := []struct {
		name string
		r    runner.Runner
	}{
		{"systemd", pagetest.SampleRunner()},
		{"ジョブ実行中", pagetest.BusyRunner()},
		{"直起動", pagetest.StandaloneRunner()},
	}

	out := make([]detailCase, 0, len(targets)*8)
	for _, tg := range targets {
		for mask := range 8 {
			caps := appconfig.Caps{
				Root:        mask&1 != 0,
				Systemd:     mask&2 != 0,
				GitHubToken: mask&4 != 0,
				Docker:      true,
				Journal:     true,
				SudoUser:    "ousiass",
			}
			out = append(out, detailCase{
				name:   tg.name + "/caps" + strconv.Itoa(mask),
				target: tg.r,
				caps:   caps,
			})
		}
	}
	return out
}

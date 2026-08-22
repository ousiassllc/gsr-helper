package molecule

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

func sampleCaps() CapsView {
	return CapsView{
		Host:       "build01",
		Root:       true,
		Systemd:    true,
		GitHubUser: "ousiass",
		HasToken:   true,
	}
}

// 依存する操作が使えないことをヘッダで示す（root が無ければ read-only など）。
//
// ディスク使用率は出さない。集計は internal/disk の担当であり別 Issue である。
func TestCapsBar(t *testing.T) {
	full := sampleCaps()
	noRoot, noToken, noUser, noSystemd := full, full, full, full
	noRoot.Root, noToken.HasToken, noUser.GitHubUser, noSystemd.Systemd = false, false, "", false

	cases := []struct {
		name    string
		v       CapsView
		want    []string
		notWant []string
	}{
		{"全ての能力あり", full,
			[]string{"gsr-helper", "host: build01", "root", "gh: ousiass"},
			[]string{"read-only", "systemd なし", "disk", "%"}},
		{"root なし", noRoot, []string{"read-only"}, []string{" root"}},
		{"gh 未認証", noToken, []string{"gh: 未認証"}, nil},
		{"ユーザー名が未取得", noUser, []string{"gh: 認証済み"}, nil},
		{"systemd なし", noSystemd, []string{"systemd なし"}, nil},
	}
	for _, c := range cases {
		got := CapsBar(c.v, 120, plainStyles())
		for _, want := range c.want {
			if !strings.Contains(got, want) {
				t.Errorf("%s: ヘッダに %q が無い: %q", c.name, want, got)
			}
		}
		for _, ng := range c.notWant {
			if strings.Contains(got, ng) {
				t.Errorf("%s: ヘッダに %q が含まれている: %q", c.name, ng, got)
			}
		}
	}
}

// 長いホスト名でも割り当てられた幅を超えない。
func TestCapsBarFitsWidth(t *testing.T) {
	v := sampleCaps()
	v.Host = "very-long-hostname-for-a-build-machine"
	v.Root, v.Systemd = false, false

	for _, width := range []int{120, token.WidthTarget, token.WidthMin, 20, 1, 0} {
		if w := lipgloss.Width(CapsBar(v, width, plainStyles())); w > width {
			t.Errorf("幅 %d のヘッダの幅 = %d", width, w)
		}
	}
}

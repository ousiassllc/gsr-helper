package ui

import (
	"slices"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// 設定を保存したら走査ルートが再起動を待たずに切り替わること（Issue #132）。
//
// 起動時に合成した結果を Options.Roots へ畳んでいた頃は、設定ファイルには
// 書けているのに走査の入力が古い scan_roots のままだった（Issue #128 で親の
// cfg を差し替えるようにしても、走査は cfg を見ていないので直らなかった）。
//
// **--root は保存後も効き続ける。** 明示的に渡したルートを設定の保存で消すと、
// 起動コマンドを変えていないのに走査対象が減る。
//
// **見ているのは scanRoots() までである。** discover() が走査へ渡す runner.Options は
// discovery.Start がクロージャへ畳み込むので、ここからは覗けない。したがって
// (1) discover() が scanRoots() を呼んでいること、(2) cmd が ui.Options.Roots へ
// 渡すのが --root の値だけであること、の 2 つは本テストの守備範囲の外にある。
// とくに (2) が巻き戻ると、MergeScanRoots が冪等なせいで合成結果は正しく見えたまま、
// 設定から消した scan_roots だけが再起動まで残るという形で #132 が再発する。
func TestScanRootsFollowSavedConfig(t *testing.T) {
	a := newApp(exec.NewFake())
	a.cfg.ScanRoots = []string{"/srv/old"}
	a.opts.Roots = []string{"/srv/cli"}

	if got, want := a.scanRoots(), []string{"/srv/old", "/srv/cli"}; !slices.Equal(got, want) {
		t.Fatalf("保存前の走査ルート = %v, want %v", got, want)
	}

	next := appconfig.Default()
	next.ScanRoots = []string{"/srv/new"}
	a, _ = update(a, page.ConfigSavedMsg{Conf: next})

	want := []string{"/srv/new", "/srv/cli"}
	if got := a.scanRoots(); !slices.Equal(got, want) {
		t.Errorf("保存後の走査ルート = %v, want %v", got, want)
	}
}

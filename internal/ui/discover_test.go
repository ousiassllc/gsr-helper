package ui

import (
	"slices"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/discovery"
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
// **見ているのは scanRoots() までである。** input() が走査へ渡す runner.Options は
// discovery.Start がクロージャへ畳み込むので、ここからは覗けない。したがって
// (1) input() が scanRoots() を呼んでいること、(2) cmd が ui.Options.Roots へ
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

// 追い抜かれた周期では共有状態を配り直さない。**配り直すと、消えたはずの古い一覧が
// page へ届く。** 周期を捨てる判定そのものは discovery.State.Apply が持つ
// （discovery/state_test.go の TestApplyDropsStaleCycle）。
func TestStaleDiscoverResultDoesNotRedistribute(t *testing.T) {
	a := newApp(exec.NewFake())
	a, _ = update(a, discovery.Msg{Seq: 2, Result: runner.Result{}, Err: nil})

	if _, cmd := update(a, discovery.Msg{Seq: 1, Result: runner.Result{}, Err: nil}); cmd != nil {
		t.Error("古い周期の結果で共有状態を配り直している")
	}
}

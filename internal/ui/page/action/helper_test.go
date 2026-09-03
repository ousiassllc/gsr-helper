package action

import (
	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 検証の材料は page/pagetest から取る。同じ runner の形を各パッケージで組み直すと、
// 判定の前提がパッケージごとに食い違う（pagetest の doc）。

// testKeys はキー定義の集約を返す。
func testKeys() keymap.Set { return pagetest.Keys() }

// fullCaps はすべての能力がある状態を返す。
func fullCaps() appconfig.Caps { return pagetest.Caps() }

// sampleRunner は systemd 管理で稼働中の runner を返す。
func sampleRunner() runner.Runner { return pagetest.SampleRunner() }

// busyRunner はジョブを実行中の runner を返す。
func busyRunner() runner.Runner { return pagetest.BusyRunner() }

// standaloneRunner は run.sh を直起動している runner を返す。
func standaloneRunner() runner.Runner { return pagetest.StandaloneRunner() }

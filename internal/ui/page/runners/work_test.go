package runners_test

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runners"
)

// 共有状態が届いたら `_WORK` 列が埋まる（Issue #73）。
//
// 行の組み立てそのものは rowview のテストが見る。ここで固定するのは
// 親 Model → page → 行 の経路が繋がっていることである。
func TestWorkColumnAppearsAfterSharedStateArrives(t *testing.T) {
	st := pagetest.State(120, 16)
	st.Result.Runners = []runner.Runner{{Dir: "/opt/runners/a"}}
	st.Disk.Work = map[string]page.WorkUsage{"/opt/runners/a": {Bytes: 3 << 20}}

	m, _ := runners.New(0, st).Update(st)
	if body := m.View().Content; !strings.Contains(body, "3.0M") {
		t.Errorf("_WORK 列に使用量が出ていない:\n%s", body)
	}
}

package molecule

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// sampleRunner は表示に必要な値が全て埋まった runner。
func sampleRunner() RunnerView {
	return RunnerView{
		Name:          "build01-1",
		Scope:         "org:foo",
		Managed:       "systemd",
		SvcActive:     "active",
		SvcSub:        "running",
		Busy:          true,
		Elapsed:       4*time.Minute + 12*time.Second,
		Version:       "2.311.0",
		LatestVersion: "2.311.0",
		Work:          "31.2G",
		Warn:          false,
	}
}

// runner は sampleRunner に変更を加えた runner を返す。
func runner(f func(v *RunnerView)) RunnerView {
	v := sampleRunner()
	f(&v)
	return v
}

// セル数が列数と一致し、各セル幅が列幅と一致する。
//
// 遷移中の状態（deactivating）と長いバージョン文字列を含めるのは、装飾済みの
// 文字列を Pad するだけでは列幅を超えるからである。x（停止）や d（ドレイン）の
// 直後には必ず deactivating を通る。
func TestRunnerRowCells(t *testing.T) {
	views := map[string]RunnerView{
		"標準":      sampleRunner(),
		"空の値":     {},
		"全角の名前":   runner(func(v *RunnerView) { v.Name = "日本語ランナー名01" }),
		"長い名前":    runner(func(v *RunnerView) { v.Name = "build01-very-long-runner-name-1" }),
		"未集計":     runner(func(v *RunnerView) { v.Work = "-" }),
		"注意あり":    runner(func(v *RunnerView) { v.Warn = true }),
		"バージョン差異": runner(func(v *RunnerView) { v.Version, v.LatestVersion = "2.309.0", "2.311.0" }),
		"長いバージョン": runner(func(v *RunnerView) { v.Version, v.LatestVersion = "2.311.0-beta.1", "2.311.0" }),
		"サービス異常":  runner(func(v *RunnerView) { v.SvcActive, v.SvcSub = "failed", "failed" }),
		"停止中への遷移": runner(func(v *RunnerView) { v.SvcActive, v.SvcSub = "deactivating", "stop-sigterm" }),
		"起動中":     runner(func(v *RunnerView) { v.SvcActive, v.SvcSub = "activating", "start-pre" }),
		"ユニット無し":  runner(func(v *RunnerView) { v.SvcActive, v.SvcSub = "", "" }),
		"ジョブ無し":   runner(func(v *RunnerView) { v.Busy = false }),
		"長いジョブ":   runner(func(v *RunnerView) { v.Elapsed = 120 * time.Hour }),
		"長いスコープ":  runner(func(v *RunnerView) { v.Scope = "very-long-owner/very-long-repository" }),
	}
	assertRows(t, token.RunnerColumns(), []int{120, 100, 80, 78, 71, 61, 53}, views, RunnerRow)

	// 知らない列でもセルを欠かさない（列数とセル数を必ず一致させる）。
	unknown := []token.Column{{ID: "UNKNOWN", Title: "UNKNOWN", Width: 8, Right: false}}
	assertRowCells(t, "知らない列", unknown, RunnerRow(sampleRunner(), unknown, plainStyles()))
}

func TestRunnerRowContents(t *testing.T) {
	cols := Columns(token.RunnerColumns(), 100)
	cells := RunnerRow(sampleRunner(), cols, plainStyles())
	row := strings.Join(cells, " ")

	for _, want := range []string{
		"build01-1", "org:foo", "systemd",
		token.IconActive + " active", token.IconJob + " 4m12s", "2.311.0", "31.2G",
	} {
		if !strings.Contains(row, want) {
			t.Errorf("行に %q が無い: %q", want, row)
		}
	}
	if strings.Contains(row, token.IconWarn) {
		t.Errorf("注意事項が無いのに注意記号が出ている: %q", row)
	}

	// 未集計の _work は "-" のまま表示する（internal/disk の集計は別 Issue）。
	uncounted := RunnerRow(runner(func(v *RunnerView) { v.Work = "-" }), cols, plainStyles())
	if last := uncounted[len(uncounted)-1]; strings.TrimSpace(last) != token.IconNoUnit {
		t.Errorf("未集計の _WORK セル = %q, want %q", last, token.IconNoUnit)
	}
}

// 注意記号は名前の直後に置く（列の端に離すと、どの行の注意なのか読み取りにくい）。
// 記号の有無で列幅は変わらない。
func TestRunnerRowWarnMark(t *testing.T) {
	warned := runner(func(v *RunnerView) { v.Warn = true })
	cols := Columns(token.RunnerColumns(), 100)
	with := RunnerRow(warned, cols, plainStyles())
	without := RunnerRow(sampleRunner(), cols, plainStyles())

	if want := "build01-1 " + token.IconWarn; !strings.HasPrefix(with[0], want) {
		t.Errorf("NAME セル = %q, want %q で始まる", with[0], want)
	}
	if lipgloss.Width(with[0]) != lipgloss.Width(without[0]) {
		t.Errorf("注意記号の有無で NAME セルの幅が変わった: %d / %d",
			lipgloss.Width(with[0]), lipgloss.Width(without[0]))
	}

	// 幅が注意記号の分すら無い場合も panic せず、幅を守る。
	narrow := []token.Column{{ID: token.ColName, Title: "NAME", Width: 2, Right: false}}
	if w := lipgloss.Width(RunnerRow(warned, narrow, plainStyles())[0]); w != 2 {
		t.Errorf("狭い NAME 列のセル幅 = %d, want 2", w)
	}
}

package listrow

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
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
	cols := molecule.Columns(token.RunnerColumns(), 100, token.RunnerColumnRules())
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
	cols := molecule.Columns(token.RunnerColumns(), 100, token.RunnerColumnRules())
	with := RunnerRow(warned, cols, plainStyles())
	without := RunnerRow(sampleRunner(), cols, plainStyles())

	if want := "build01-1 " + token.IconWarn; !strings.HasPrefix(with[0], want) {
		t.Errorf("NAME セル = %q, want %q で始まる", with[0], want)
	}
	if lipgloss.Width(with[0]) != lipgloss.Width(without[0]) {
		t.Errorf("注意記号の有無で NAME セルの幅が変わった: %d / %d",
			lipgloss.Width(with[0]), lipgloss.Width(without[0]))
	}

	// 幅が注意記号の分すら無い場合も panic せず、幅を守る。名前を諦めても注意記号は
	// 残す。行に注意があることを示すのはこの記号だけで、落とすと警告が画面から消える。
	narrow := []token.Column{{ID: token.ColName, Title: "NAME", Width: 2, Right: false}}
	cell := RunnerRow(warned, narrow, plainStyles())[0]
	if w := lipgloss.Width(cell); w != 2 {
		t.Errorf("狭い NAME 列のセル幅 = %d, want 2", w)
	}
	if !strings.Contains(cell, token.IconWarn) {
		t.Errorf("狭い NAME 列のセル = %q, want %q を含む", cell, token.IconWarn)
	}
}

// 名前が空のときも他の列と同じ「値なし」の記号を出す。
//
// 空白のままだと値が無いのか描画に失敗したのかを読み分けられない。注意記号は
// 名前が空でも消さない。
func TestRunnerRowEmptyName(t *testing.T) {
	cols := molecule.Columns(token.RunnerColumns(), 100, token.RunnerColumnRules())
	nameless := RunnerRow(runner(func(v *RunnerView) { v.Name = "" }), cols, plainStyles())
	if got := strings.TrimSpace(nameless[0]); got != token.IconNoUnit {
		t.Errorf("名前が空のときの NAME セル = %q, want %q", nameless[0], token.IconNoUnit)
	}

	warned := RunnerRow(runner(func(v *RunnerView) { v.Name, v.Warn = "", true }), cols, plainStyles())
	if want := token.IconNoUnit + " " + token.IconWarn; !strings.HasPrefix(warned[0], want) {
		t.Errorf("名前が空で注意ありの NAME セル = %q, want %q で始まる", warned[0], want)
	}
}

// 状態を取得できなかったユニットは「ユニットなし」と別の記号で描く。
//
// systemctl show が失敗したユニットは値の無い状態として渡ってくるが、それは
// ユニットが存在しないことを意味しない（atom.StatusUnknown）。同じ "-" で描くと
// 仕様が書き分けている 2 つの状態を SVC 列で読み分けられない。
func TestRunnerRowUnknownServiceState(t *testing.T) {
	cols := molecule.Columns(token.RunnerColumns(), token.WidthTarget, token.RunnerColumnRules())

	noUnit := RunnerRow(RunnerView{
		Name: "build01-1", Managed: "run.sh", Version: "2.309.0",
	}, cols, plainStyles())
	unknown := RunnerRow(RunnerView{
		Name: "build01-1", Managed: "systemd", Version: "2.309.0", SvcUnknown: true,
	}, cols, plainStyles())

	svc := columnIndex(t, cols, token.ColSvc)
	if noUnit[svc] == unknown[svc] {
		t.Errorf("ユニットなしと状態不明が同じ表示になっている（%q）", unknown[svc])
	}
	if !strings.Contains(unknown[svc], token.IconUnknown) {
		t.Errorf("状態不明の SVC セル = %q, want %q を含む", unknown[svc], token.IconUnknown)
	}
	if !strings.Contains(noUnit[svc], token.IconNoUnit) {
		t.Errorf("ユニットなしの SVC セル = %q, want %q を含む", noUnit[svc], token.IconNoUnit)
	}
}

// columnIndex は列の識別子から添字を返す。
func columnIndex(t *testing.T, cols []token.Column, id string) int {
	t.Helper()

	for i, c := range cols {
		if c.ID == id {
			return i
		}
	}
	t.Fatalf("列 %q が幅 %d に無い", id, token.WidthTarget)
	return -1
}

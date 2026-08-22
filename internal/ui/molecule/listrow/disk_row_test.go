package listrow

import (
	"strconv"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

func sampleTarget() DiskTargetView {
	return DiskTargetView{
		Target:   "build01-1 / _work/bar",
		Bytes:    25877477785,
		Files:    412003,
		Path:     "/opt/runners/build01-1/_work/bar",
		Scanning: false,
		Reason:   "",
		Failed:   false,
	}
}

// diskViews は screens.md の Disk タブのモックに出る行をひととおり並べる。
func diskViews() map[string]DiskTargetView {
	return map[string]DiskTargetView{
		"標準": sampleTarget(),
		"docker（パスもファイル数も無い）": {
			Target: "docker / build cache", Bytes: 13314398618, Files: -1,
			Path: "", Scanning: false, Reason: "", Failed: false,
		},
		"集計中": {
			Target: "build01-2 / _work/bar", Bytes: -1, Files: -1,
			Path: "/opt/runners/build01-2/_work/bar", Scanning: true, Reason: "", Failed: false,
		},
		"選択できない": {
			Target: "build01-2 / _work/bar", Bytes: -1, Files: -1,
			Path: "/opt/runners/build01-2/_work/bar", Scanning: true,
			Reason: "ジョブ実行中のため削除不可", Failed: false,
		},
		"集計に失敗": {
			Target: "build01-1 / _diag", Bytes: -1, Files: -1,
			Path: "/opt/runners/build01-1/_diag", Scanning: false, Reason: "", Failed: true,
		},
		"空の値": {},
		"長い対象名": {
			Target: strings.Repeat("very-long-runner-name", 3), Bytes: 1, Files: 1,
			Path: strings.Repeat("/very-long-name", 8), Scanning: false, Reason: "", Failed: false,
		},
	}
}

// 幅を変えても、セル数は列数と一致し、各セルの幅は列幅と一致する。
//
// assertRows を使えないのは、あちらが runner の一覧の落とし方（RunnerColumnRules）を
// 前提にしているためである。Disk は落とす順が違う（token.DiskColumnRules）。
func TestDiskTargetRowCells(t *testing.T) {
	views := diskViews()
	for _, width := range []int{200, 120, 80, 70, 60, 40} {
		cols := molecule.Columns(token.DiskColumns(), width, token.DiskColumnRules())
		for name, v := range views {
			cells := DiskTargetRow(v, cols, plainStyles())
			assertRowCells(t, "幅 "+strconv.Itoa(width)+" "+name, cols, cells)
		}
	}

	// 列が無い場合も panic せずセルを返さない。
	if got := DiskTargetRow(DiskTargetView{}, nil, plainStyles()); len(got) != 0 {
		t.Errorf("列が無いときのセル数 = %d, want 0", len(got))
	}
}

// 幅が足りないときも対象名と容量は残る（DiskColumnRules の Keep）。
func TestDiskTargetRowKeepsTargetAndSize(t *testing.T) {
	cols := molecule.Columns(token.DiskColumns(), 60, token.DiskColumnRules())
	ids := make([]string, 0, len(cols))
	for _, c := range cols {
		ids = append(ids, c.ID)
	}
	for _, want := range []string{token.ColTarget, token.ColSize} {
		if !strings.Contains(strings.Join(ids, ","), want) {
			t.Errorf("幅 60 の列 = %v, want %s を含む", ids, want)
		}
	}
}

// 各列に載る値は screens.md の Disk タブのモックのとおりである。
func TestDiskTargetRowContents(t *testing.T) {
	cols := molecule.Columns(token.DiskColumns(), 120, token.DiskColumnRules())
	cells := DiskTargetRow(sampleTarget(), cols, plainStyles())
	for i, want := range []string{"build01-1 / _work/bar", "24.1G", "412,003", "_work/bar"} {
		if !strings.Contains(cells[i], want) {
			t.Errorf("列 %s のセル = %q, want %q を含む", cols[i].ID, cells[i], want)
		}
	}
	// 桁を縦に比較する列なので右寄せにする。
	for _, i := range []int{1, 2} {
		if strings.HasSuffix(cells[i], " ") {
			t.Errorf("列 %s のセル = %q, 右寄せでなければならない", cols[i].ID, cells[i])
		}
	}
}

// 値を持たない対象（docker の行）は「値なし」の記号にする。0 件と読ませない。
func TestDiskTargetRowMissingValues(t *testing.T) {
	cols := molecule.Columns(token.DiskColumns(), 120, token.DiskColumnRules())
	cells := DiskTargetRow(DiskTargetView{
		Target: "docker / build cache", Bytes: 13314398618, Files: -1,
		Path: "", Scanning: false, Reason: "", Failed: false,
	}, cols, plainStyles())

	for i, name := range map[int]string{2: "FILES", 3: "PATH"} {
		if got := strings.TrimSpace(cells[i]); got != token.IconNoUnit {
			t.Errorf("%s セル = %q, want %q", name, got, token.IconNoUnit)
		}
	}
	if got := strings.TrimSpace(cells[1]); got != "12.4G" {
		t.Errorf("SIZE セル = %q, want %q", got, "12.4G")
	}
}

// 集計中・集計失敗・選択不可はそれぞれ別の表示になる。
//
// 集計中と失敗を同じ表示にすると、待てば埋まるのか埋まらないのかを読み分けられない。
func TestDiskTargetRowStates(t *testing.T) {
	cols := molecule.Columns(token.DiskColumns(), 120, token.DiskColumnRules())

	tests := map[string]struct {
		view DiskTargetView
		cell int
		want string
	}{
		"集計中は SIZE 列に出す": {
			view: DiskTargetView{Target: "build01-2 / _work/bar", Bytes: -1, Files: -1,
				Path: "", Scanning: true, Reason: "", Failed: false},
			cell: 1, want: labelScanning,
		},
		"集計失敗は記号を添える": {
			view: DiskTargetView{Target: "build01-2 / _work/bar", Bytes: -1, Files: -1,
				Path: "", Scanning: true, Reason: "", Failed: true},
			cell: 1, want: token.Icon(token.StateFail) + " " + labelScanFailed,
		},
		"選択できない理由は PATH 列に出す": {
			view: DiskTargetView{Target: "build01-2 / _work/bar", Bytes: -1, Files: -1,
				Path: "/opt/runners/build01-2/_work/bar", Scanning: true,
				Reason: "ジョブ実行中のため削除不可", Failed: false},
			cell: 3, want: "ジョブ実行中のため削除不可",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cells := DiskTargetRow(tt.view, cols, plainStyles())
			if got := strings.TrimSpace(cells[tt.cell]); got != tt.want {
				t.Errorf("列 %s のセル = %q, want %q", cols[tt.cell].ID, got, tt.want)
			}
		})
	}
}

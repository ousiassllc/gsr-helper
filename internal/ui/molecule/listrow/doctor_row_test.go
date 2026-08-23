package listrow

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 行は列と同じ順・同じ数のセルを返し、各セルの幅が列幅と一致する。
func TestDoctorRowCells(t *testing.T) {
	t.Parallel()

	views := map[string]DoctorView{
		"FAIL と対象あり": {
			Status: token.StateFail, Category: "ジョブ実行の前提",
			Check: "docker グループが未反映", Target: "build01-2",
		},
		"OK でホスト全体": {
			Status: token.StateOK, Category: "ネットワーク",
			Check: "api.github.com:443 到達", Target: "",
		},
		"SKIP": {
			Status: token.StateSkip, Category: "docker",
			Check: "使用量取得（docker が無い）", Target: "",
		},
		"空の値": {},
	}

	for _, width := range []int{80, 60, 40, 20} {
		cols := molecule.Columns(token.DoctorColumns(), width, token.DoctorColumnRules())
		for name, v := range views {
			assertRowCells(t, name, cols, DoctorRow(v, cols, plainStyles()))
		}
	}

	if got := DoctorRow(DoctorView{}, nil, plainStyles()); len(got) != 0 {
		t.Errorf("列が無いときのセル数 = %d, want 0", len(got))
	}
}

// 対象が空のときは「値なし」の記号にする（ホスト全体のチェック）。
func TestDoctorRowEmptyTargetIsDash(t *testing.T) {
	t.Parallel()

	cols := token.DoctorColumns()
	cells := DoctorRow(DoctorView{Status: token.StateOK, Check: "x"}, cols, plainStyles())
	for i, c := range cols {
		if c.ID != token.ColTarget {
			continue
		}
		if !strings.Contains(cells[i], token.IconNoUnit) {
			t.Errorf("TARGET のセル = %q, want %q を含む", cells[i], token.IconNoUnit)
		}
	}
}

// 幅が足りなくても STATUS と CHECK は落とさない。
// どちらかが欠けると、行が何の項目かも判定も伝えられなくなる。
func TestDoctorColumnsKeepStatusAndCheck(t *testing.T) {
	t.Parallel()

	for _, width := range []int{80, 60, 40, 20, 10} {
		cols := molecule.Columns(token.DoctorColumns(), width, token.DoctorColumnRules())
		var hasStatus, hasCheck bool
		for _, c := range cols {
			hasStatus = hasStatus || c.ID == token.ColStatus
			hasCheck = hasCheck || c.ID == token.ColCheck
		}
		if !hasStatus || !hasCheck {
			t.Errorf("幅 %d で STATUS=%v CHECK=%v（どちらも落とさないこと）", width, hasStatus, hasCheck)
		}
	}
}

// 最初に落ちるのは CATEGORY、次が TARGET。
func TestDoctorColumnsDropOrder(t *testing.T) {
	t.Parallel()

	all := token.DoctorColumns()
	full := molecule.Columns(all, 80, token.DoctorColumnRules())
	if len(full) != len(all) {
		t.Fatalf("幅 80 の列数 = %d, want %d（保証する幅で列が落ちている）", len(full), len(all))
	}

	narrow := molecule.Columns(all, 45, token.DoctorColumnRules())
	for _, c := range narrow {
		if c.ID == token.ColCategory {
			t.Errorf("幅 45 で CATEGORY が残っている（最初に落とす列）")
		}
	}
}

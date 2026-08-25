package docscheck

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/buildconfig/buildconfigtest"
)

const (
	budgetHeading      = "### 行数の予算"
	budgetNonUIHeading = "#### UI 層の外のディレクトリ"
	budgetTableHeader  = "| ディレクトリ | 行数 | 残り | 判定 |"
	budgetRevisionHead = "## 改訂履歴"
)

// budgetDocRow は行数表の 1 行。例: | `ui/page/disk` | 1997 | 3 | pass |
var budgetDocRow = regexp.MustCompile("(?m)^\\|\\s*`([^`]+)`\\s*\\|\\s*(-?\\d+)\\s*\\|\\s*(-?\\d+)\\s*\\|\\s*([a-z]+)\\s*\\|\\s*$")

// budgetRow は行数表の 1 行を解いたもの。表に書かれた順序も保つ。
type budgetRow struct {
	dir       string
	lines     int
	remaining int
	verdict   string
}

// linterlyReport は `go tool linterly check --format json` の出力のうち、
// 行数表の突き合わせに要る欄だけを受ける。
type linterlyReport struct {
	Results []struct {
		Path     string `json:"path"`
		Type     string `json:"type"`
		Lines    int    `json:"lines"`
		Limit    int    `json:"limit"`
		Severity string `json:"severity"`
	} `json:"results"`
}

// 行数表は `go tool linterly check` の実測と一致していなければならない。
//
// 同じ実測値が本書の 3 か所（2 つの行数表・散文・改訂履歴）に手で写されているのに、
// これまでどのテストも「書かれた数」と「実際に数えた行数」を突き合わせていなかった。
// 検査していたのは依存グラフの図とパッケージ名の一覧だけで、行数は誰も見ていない。
// そのため表と散文は `make check` が緑のまま静かに古くなる。実害も出ている——散文が
// 後続の Issue へ「移せるのは 68 行まで」と指示した時点で本当の予算は 65 行であり、
// 3 行の超過を指示していた（Issue #164）。
//
// ここでは実測を正とする。表がずれていたら**表を実測に合わせる**のであって、
// 実測の側を動かすのではない。
func TestLineBudgetTablesMatchLinterly(t *testing.T) {
	root := buildconfigtest.RepoRoot(t)

	uiRows, nonUIRows := parseBudgetTables(t)
	uiWant, nonUIWant := measureLineBudgets(t, root)

	compareBudgetTable(t, "行数の予算", uiRows, uiWant)
	compareBudgetTable(t, budgetNonUIHeading, nonUIRows, nonUIWant)
	checkBudgetOrder(t, "行数の予算", uiRows)
	checkBudgetOrder(t, budgetNonUIHeading, nonUIRows)
}

// readAtomicDesign は docs/ui/atomic-design.md の本文を返す。
func readAtomicDesign(t *testing.T) string {
	t.Helper()

	path := filepath.Join(buildconfigtest.RepoRoot(t), "docs", "ui", "atomic-design.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("atomic-design.md を読めない: %v", err)
	}
	return string(body)
}

// parseBudgetTables は UI 層の表と UI 層の外の表を、行に書かれた順序のまま返す。
//
// 表の位置は見出しで引く。行番号で決め打ちすると、本書へ 1 行足しただけで
// 検査の対象がずれる。
func parseBudgetTables(t *testing.T) (ui, nonUI []budgetRow) {
	t.Helper()

	doc := readAtomicDesign(t)
	_, section, ok := strings.Cut(doc, budgetHeading)
	if !ok {
		t.Fatalf("atomic-design.md に %q の見出しが無い", budgetHeading)
	}
	uiPart, nonUIPart, ok := strings.Cut(section, budgetNonUIHeading)
	if !ok {
		t.Fatalf("atomic-design.md に %q の見出しが無い", budgetNonUIHeading)
	}
	// UI 層の外の表の後ろには判断の記録（`#####` の小節）が続く。表だけを見る。
	if head, _, cut := strings.Cut(nonUIPart, "\n##### "); cut {
		nonUIPart = head
	}

	ui, nonUI = parseBudgetRows(t, uiPart), parseBudgetRows(t, nonUIPart)
	// 見出しの改名やパスの書き方の変更で 1 行も拾えないまま緑になるのを防ぐ。
	if len(ui) == 0 {
		t.Fatal("行数の予算の表から 1 行も読めない")
	}
	if len(nonUI) == 0 {
		t.Fatalf("%q の表から 1 行も読めない", budgetNonUIHeading)
	}
	return ui, nonUI
}

// parseBudgetRows は節の本文から行数表の行を拾う。
func parseBudgetRows(t *testing.T, section string) []budgetRow {
	t.Helper()

	rows := make([]budgetRow, 0, len(section)/64)
	for _, m := range budgetDocRow.FindAllStringSubmatch(section, -1) {
		rows = append(rows, budgetRow{
			dir:       m[1],
			lines:     atoi(t, m[2]),
			remaining: atoi(t, m[3]),
			verdict:   m[4],
		})
	}
	return rows
}

// measureLineBudgets は linterly の実測を、UI 層の表と UI 層の外の表の分に振り分ける。
//
// 行数チェックはリポジトリ全体を見るが、表が載せるのはソースのディレクトリだけである。
// リポジトリ直下・CI 設定・`testdata/` のフィクスチャは表の対象外（本節の断り書き）。
func measureLineBudgets(t *testing.T, root string) (ui, nonUI map[string]budgetRow) {
	t.Helper()

	cmd := exec.Command("go", "tool", "linterly", "check", "--format", "json")
	cmd.Dir = root
	out, err := cmd.Output()
	var report linterlyReport
	if jerr := json.Unmarshal(out, &report); jerr != nil {
		t.Fatalf("linterly の JSON を解析できない: %v（実行時のエラー: %v）", jerr, err)
	}

	ui, nonUI = map[string]budgetRow{}, map[string]budgetRow{}
	for _, r := range report.Results {
		if r.Type != "directory" || strings.Contains(r.Path, "testdata/") {
			continue
		}
		dir := strings.TrimSuffix(r.Path, "/")
		switch dir {
		case ".", ".github", ".github/workflows":
			continue
		}
		row := budgetRow{dir: dir, lines: r.Lines, remaining: r.Limit - r.Lines, verdict: r.Severity}
		// UI 層の表はパスを `internal/` 抜きで書く（`ui` / `ui/page/disk` など）。
		if rest, isUI := strings.CutPrefix(dir, "internal/"); isUI && (rest == "ui" || strings.HasPrefix(rest, "ui/")) {
			row.dir = rest
			ui[rest] = row
			continue
		}
		nonUI[dir] = row
	}
	if len(ui) == 0 || len(nonUI) == 0 {
		t.Fatalf("linterly の実測からディレクトリを拾えない（UI %d 件・UI 以外 %d 件）", len(ui), len(nonUI))
	}
	return ui, nonUI
}

// compareBudgetTable は表の行と実測を突き合わせる。
func compareBudgetTable(t *testing.T, table string, rows []budgetRow, want map[string]budgetRow) {
	t.Helper()

	got := make(map[string]budgetRow, len(rows))
	for _, row := range rows {
		if _, dup := got[row.dir]; dup {
			t.Errorf("%s: `%s` の行が表に複数ある", table, row.dir)
		}
		got[row.dir] = row
	}
	for _, dir := range sortedBudgetKeys(want) {
		w := want[dir]
		g, ok := got[dir]
		if !ok {
			t.Errorf("%s: `%s`（実測 %d 行）の行が表に無い", table, dir, w.lines)
			continue
		}
		if g.lines != w.lines {
			t.Errorf("%s: `%s` の行数は実測 %d 行だが、表は %d 行としている", table, dir, w.lines, g.lines)
		}
		if g.remaining != w.remaining {
			t.Errorf("%s: `%s` の残りは実測 %d 行だが、表は %d 行としている", table, dir, w.remaining, g.remaining)
		}
		if g.verdict != w.verdict {
			t.Errorf("%s: `%s` の判定は実測 %s だが、表は %s としている", table, dir, w.verdict, g.verdict)
		}
	}
	for _, row := range rows {
		if _, ok := want[row.dir]; !ok {
			t.Errorf("%s: 表が `%s`（%d 行）を挙げているが実測に無い（削除・改名に追随できていない）", table, row.dir, row.lines)
		}
	}
}

// checkBudgetOrder は表が行数の多い順に並んでいることを確かめる。
// 表の直前の一文が「行数の多い順に並べる」と約束している。
func checkBudgetOrder(t *testing.T, table string, rows []budgetRow) {
	t.Helper()

	for i := 1; i < len(rows); i++ {
		if rows[i-1].lines < rows[i].lines {
			t.Errorf("%s: `%s`（%d 行）が `%s`（%d 行）より前に並んでいる（行数の多い順ではない）",
				table, rows[i-1].dir, rows[i-1].lines, rows[i].dir, rows[i].lines)
		}
	}
}

func sortedBudgetKeys(m map[string]budgetRow) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

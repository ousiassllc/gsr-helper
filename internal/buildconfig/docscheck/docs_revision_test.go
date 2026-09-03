package docscheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ousiassllc/gsr-helper/internal/buildconfig/buildconfigtest"
)

// revisionRow は改訂履歴表の行頭の版番号を拾う。表は `| 版 | 日付 | ... |` の形で、
// 先頭列が `1.7` のような 2 桁の版番号になっている。
var revisionRow = regexp.MustCompile(`(?m)^\|\s*(\d+)\.(\d+)\s*\|`)

// revisionRowRunes は改訂履歴表の 1 行に許す文字数の上限である。
const revisionRowRunes = 1500

// revisionHeaderFirstCell は改訂履歴表の見出し行の先頭セル。行を見分けるためだけに使う。
const revisionHeaderFirstCell = "| 版 |"

// revisionHeaderRow は改訂履歴表の見出し行（`| 版 | 日付 | ... |`）を拾う。
var revisionHeaderRow = regexp.MustCompile(`(?m)^\|\s*版\s*\|.*\|\s*$`)

// forEachRevisionHistory は docs/ 配下で `## 改訂履歴` を持つ文書ごとに fn を呼ぶ。
// tail はその見出しより後ろの本文（表と「改訂の詳細」節）である。
func forEachRevisionHistory(t *testing.T, fn func(name, tail string)) {
	t.Helper()
	root := buildconfigtest.RepoRoot(t)

	var checked int
	err := filepath.WalkDir(filepath.Join(root, "docs"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		body, rerr := os.ReadFile(path)
		if rerr != nil {
			t.Errorf("%s を読めない: %v", path, rerr)
			return nil
		}
		_, tail, found := strings.Cut(string(body), "## 改訂履歴")
		if !found {
			return nil
		}
		checked++
		rel, _ := filepath.Rel(root, path)
		fn(rel, tail)
		return nil
	})
	if err != nil {
		t.Fatalf("docs を走査できない: %v", err)
	}
	// 走査対象を取り違えた（パスの誤り等で 0 件になった）まま緑になるのを防ぐ。
	if checked == 0 {
		t.Fatal("改訂履歴を持つドキュメントが 1 件も見つからない")
	}
}

// `docs/` 配下の改訂履歴を持つ全文書について、版番号が重複せず昇順でなければならない。
//
// 実際に docs/environment/setup.md でブランチのマージ時に `1.8` が 2 行残り、
// `1.7` が `1.8` の後ろに並んだ（Issue #59）。
func TestDocRevisionHistoryVersionsUniqueAndAscending(t *testing.T) {
	forEachRevisionHistory(t, func(name, tail string) {
		checkRevisionVersions(t, name, tail)
	})
}

// `docs/` 配下の改訂履歴を持つ全文書について、表の 1 行が上限（1500 文字）に収まらなければ
// ならない。表のセルは改行を持てないので、欄が伸びると 1 文字の修正でも行まるごとが差分に出る。
// **検査は言い回しを一切見ず、文字数だけを見る**（方針は下記「改訂履歴の欄は要約と参照に留める」）。
//
// 上限を超えた欄は `## 改訂履歴` の下の「改訂の詳細」節へ `#### 改訂 N.M` の小節として出し、
// 表には要約 1 文とその小節への参照だけを残す。小節の中は改行できるので、1 文字の修正は
// 1 行の差分で済む。方針は docs/environment/setup.md の「改訂履歴の欄は要約と参照に留める」にある。
//
// この検査は言い回しを一切見ない。「書きすぎた欄」を文言のパターンで拾う手は、行数の散文の検査が
// 2 周かけて捨てた方式である（docs/ui/atomic-design.md の「行数の実測値は表だけが持つ」）。
func TestDocRevisionHistoryRowsFitInBudget(t *testing.T) {
	forEachRevisionHistory(t, func(name, tail string) {
		for _, line := range strings.Split(tail, "\n") {
			m := revisionRow.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			if n := utf8.RuneCountInString(line); n > revisionRowRunes {
				t.Errorf("%s: 改訂履歴の版 %s.%s の行が %d 文字あり、上限 %d 文字を超えている。"+
					"欄を「改訂の詳細」節の小節へ出し、表には要約と参照だけを残すこと",
					name, m[1], m[2], n, revisionRowRunes)
			}
		}
	})
}

// checkRevisionVersions は改訂履歴表の版番号が重複せず昇順であることを検査する。
func checkRevisionVersions(t *testing.T, name, table string) {
	t.Helper()

	seen := map[string]bool{}
	prevMajor, prevMinor := -1, -1
	for _, m := range revisionRow.FindAllStringSubmatch(table, -1) {
		major, minor := buildconfigtest.Atoi(t, m[1]), buildconfigtest.Atoi(t, m[2])
		v := m[1] + "." + m[2]
		if seen[v] {
			t.Errorf("%s: 改訂履歴に版 %s の行が複数ある", name, v)
		}
		seen[v] = true
		if major < prevMajor || (major == prevMajor && minor <= prevMinor) {
			t.Errorf("%s: 改訂履歴の版 %s が直前の %d.%d より後ろに並んでいない",
				name, v, prevMajor, prevMinor)
		}
		prevMajor, prevMinor = major, minor
	}
}

// `docs/` 配下の改訂履歴を持つ全文書について、表の各行が見出し行と同じ数のセルを持たなければ
// ならない。Markdown は足りないセルを空として描くので、列が欠けても表は崩れず、その版の
// 変更理由だけが黙って空欄になる。
//
// 実際に PR #173 が 3 文書へ 1 行ずつ、変更理由のセルを持たない行を足していた（Issue #175）。
// 版番号の重複と昇順を見る検査も 1 行の文字数を見る検査も、セルの数え方を持たないので拾えなかった。
//
// **検査はセル数の一致だけを見る。** 欄の中身は見ない——「変更理由になっていない欄」を文言で
// 拾う手は、行数の散文の検査が 2 周かけて捨てた方式である（docs/ui/atomic-design.md の
// 「行数の実測値は表だけが持つ」）。列の欠落は「見出しと違うセル数の行が無いこと」という不在の
// 形で書けるので、この節の他の検査と同じ性格に収まる。
//
// 見出し行のセル数は文書ごとに読む。定数へ写すと、列を増やす改訂のたびに検査の側も直す必要が
// 生まれ、写しが 1 つ増える。
func TestDocRevisionHistoryRowsHaveEveryColumn(t *testing.T) {
	forEachRevisionHistory(t, func(name, tail string) {
		header := revisionHeaderRow.FindString(tail)
		if header == "" {
			t.Errorf("%s: 改訂履歴の見出し行（`%s`）が無い", name, revisionHeaderFirstCell)
			return
		}
		want := countTableCells(header)
		for _, line := range strings.Split(tail, "\n") {
			m := revisionRow.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			if n := countTableCells(line); n != want {
				t.Errorf("%s: 改訂履歴の版 %s.%s の行のセルが %d 個で、見出し行の %d 個と違う。"+
					"Markdown は足りないセルを空として描くので、表は崩れずに欄だけが黙って空になる",
					name, m[1], m[2], n, want)
			}
		}
	})
}

// countTableCells は Markdown の表の 1 行のセル数を数える。
//
// エスケープした `\|` はセルの区切りではなく本文なので、数える前に落とす（`docs/architecture/
// data-model.md` の表が実際に使っている）。行頭と行末の `|` の外側は空文字になるため、
// 分割した数から両端の 2 つを引く。
func countTableCells(line string) int {
	return len(strings.Split(strings.ReplaceAll(line, `\|`, ""), "|")) - 2
}

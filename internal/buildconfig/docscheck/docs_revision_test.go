package docscheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

// 改訂履歴の版番号は重複せず昇順でなければならない。
// 版番号が重複すると、変更理由が版番号で参照している行（setup.md の 1.12 / 1.13 など）を
// 一意に引けなくなる。順序が崩れると、表を上から読んだ順序と版の前後関係が食い違う。
//
// 実際に docs/environment/setup.md でブランチのマージ時に `1.8` が 2 行残り、
// `1.7` が `1.8` の後ろに並んだ（Issue #59）。
func TestDocRevisionHistoryVersionsUniqueAndAscending(t *testing.T) {
	forEachRevisionHistory(t, func(name, tail string) {
		checkRevisionVersions(t, name, tail)
	})
}

// 改訂履歴の表の 1 行は上限を超えてはならない。
// 表のセルは改行を持てないので、1 つの版で直した点を欄に列挙していくと 1 行が数千文字まで伸び、
// 1 文字の修正でもその行がまるごと変更行として差分に出る。版を足すたびに長大な行が積まれるため、
// 放っておくと編集とレビューのコストは伸びる一方になる（Issue #170）。
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
		major, minor := atoi(t, m[1]), atoi(t, m[2])
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

func atoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("版番号 %q を数値にできない: %v", s, err)
	}
	return n
}

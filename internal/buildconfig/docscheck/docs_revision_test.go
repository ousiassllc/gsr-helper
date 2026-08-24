package docscheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/buildconfig/buildconfigtest"
)

// revisionRow は改訂履歴表の行頭の版番号を拾う。表は `| 版 | 日付 | ... |` の形で、
// 先頭列が `1.7` のような 2 桁の版番号になっている。
var revisionRow = regexp.MustCompile(`(?m)^\|\s*(\d+)\.(\d+)\s*\|`)

// 改訂履歴の版番号は重複せず昇順でなければならない。
// 版番号が重複すると、変更理由が版番号で参照している行（setup.md の 1.12 / 1.13 など）を
// 一意に引けなくなる。順序が崩れると、表を上から読んだ順序と版の前後関係が食い違う。
//
// 実際に docs/environment/setup.md でブランチのマージ時に `1.8` が 2 行残り、
// `1.7` が `1.8` の後ろに並んだ（Issue #59）。
func TestDocRevisionHistoryVersionsUniqueAndAscending(t *testing.T) {
	root := buildconfigtest.RepoRoot(t)
	docs := filepath.Join(root, "docs")

	var checked int
	err := filepath.WalkDir(docs, func(path string, d fs.DirEntry, err error) error {
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
		checkRevisionVersions(t, rel, tail)
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

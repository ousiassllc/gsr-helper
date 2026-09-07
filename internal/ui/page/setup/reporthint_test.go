package setup

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/gh"
	dsetup "github.com/ousiassllc/gsr-helper/internal/setup"
)

// 結果報告は API の失敗の Hint を独立した行に出す。
//
// gh.APIError.Error() は Hint を末尾に置くが、結果報告は 1 行ずつ幅で切り詰めて描く
// （chrome.go の reportView）。1 行に詰めたままだと、不足しているスコープと
// `gh auth refresh` のコマンド例だけがちょうど落ちて、403 の理由が読めなくなる。
func TestReportLinesPutsAPIHintOnItsOwnLine(t *testing.T) {
	const hint = "権限が不足しています。必要なスコープは admin:org です"
	err := &gh.APIError{
		Op:     "runner_downloads",
		Scope:  "org:NonEntropyJapan",
		Status: http.StatusForbidden,
		Hint:   hint,
		Err:    errors.New("GET https://api.github.com/orgs/NonEntropyJapan/actions/runners/downloads: 403"),
	}

	got := reportLines(dsetup.Result{Failed: "runner-1", Phase: "tarball 取得"}, err)

	if !slices.ContainsFunc(got, func(l string) bool { return strings.Contains(l, hint) }) {
		t.Fatalf("Hint を含む行が無い\n報告:\n%s", strings.Join(got, "\n"))
	}
	// Hint だけの行になっていること（生のエラーと同じ行に詰めない）。
	for _, l := range got {
		if !strings.Contains(l, hint) {
			continue
		}
		if strings.Contains(l, "api.github.com") {
			t.Errorf("Hint が生のエラーと同じ行に詰まっている: %q", l)
		}
	}
}

// API の失敗でないエラーには Hint の行を足さない。空の行が増えると報告が読みにくい。
func TestReportLinesOmitsHintLineForPlainError(t *testing.T) {
	got := reportLines(dsetup.Result{Failed: "runner-1", Phase: "展開"}, errors.New("展開に失敗しました"))

	for _, l := range got {
		if strings.HasPrefix(strings.TrimSpace(l), "→") {
			t.Errorf("API の失敗でないのに Hint の行がある: %q", l)
		}
	}
}

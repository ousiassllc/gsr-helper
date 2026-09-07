package report_test

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/setup"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/setup/report"
)

// 結果報告は API の失敗の Hint を独立した行に出す。
//
// gh.APIError.Error() は Hint を末尾に置くが、結果報告は 1 行ずつ幅で切り詰めて描く
// （chrome.go の reportView）。1 行に詰めたままだと、不足しているスコープと
// `gh auth refresh` のコマンド例だけがちょうど落ちて、403 の理由が読めなくなる。
func TestLinesPutAPIHintOnItsOwnLine(t *testing.T) {
	const hint = "権限が不足しています。必要なスコープは admin:org です"
	err := &gh.APIError{
		Op:     "runner_downloads",
		Scope:  "org:NonEntropyJapan",
		Status: http.StatusForbidden,
		Hint:   hint,
		Err:    errors.New("GET https://api.github.com/orgs/NonEntropyJapan/actions/runners/downloads: 403"),
	}

	got := report.Lines(setup.Result{Failed: "runner-1", Phase: "tarball 取得"}, err)

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
func TestLinesOmitHintLineForPlainError(t *testing.T) {
	got := report.Lines(setup.Result{Failed: "runner-1", Phase: "展開"}, errors.New("展開に失敗しました"))

	for _, l := range got {
		if strings.HasPrefix(strings.TrimSpace(l), "→") {
			t.Errorf("API の失敗でないのに Hint の行がある: %q", l)
		}
	}
}

// Lines は AC-5 の結果報告そのものである。
//
// 失敗したときは「どこで止まったか」と「成功分がそのまま残っていること」を必ず出す。
// **残る旨が消えると、作りかけの runner を手で消すべきか判断できない**（FR-15）。
func TestLinesReportSucceededAndRemaining(t *testing.T) {
	t.Parallel()

	if got := report.Lines(setup.Result{Succeeded: []string{"a", "b"}, Failed: "",
		Phase: "", Err: nil, Remaining: nil}, nil); len(got) != 1 || got[0] != "完了: 2 台" {
		t.Errorf("成功時の報告 = %q, want [完了: 2 台]", got)
	}

	res := setup.Result{
		Succeeded: []string{"build01-1", "build01-2"}, Failed: "build01-3",
		Phase: "サービス登録", Err: errors.New("boom"), Remaining: []string{"build01-4"},
	}
	got := strings.Join(report.Lines(res, res.Err), "\n")
	for _, w := range []string{
		"✗ build01-3 のサービス登録で失敗しました", "  boom",
		"完了: 2 台（build01-1, build01-2）", "未実行: 1 台（build01-4）",
		"build01-1, build01-2 はそのまま残っています。",
	} {
		if !strings.Contains(got, w) {
			t.Errorf("結果報告に %q が無い:\n%s", w, got)
		}
	}
}

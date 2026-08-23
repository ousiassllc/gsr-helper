package doctor

import (
	"testing"

	dom "github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 診断の完了を取り込んだあと、起動時の前提チェック（FR-44）の件数を親へ届ける
// 経路を検証する。ヘッダと状態行に描くのは親（internal/ui）の仕事であり、
// 何を不備と数えるかは internal/ui/hostreq が持つ。

// startupID は起動時の前提チェックの対象になる識別子を i 番目から返す。
//
// 実際のレジストリから引くのは、数える対象を決めるのがレジストリだからである。
// 検証用の識別子を書くと、起動時の顔ぶれが変わったときに緑のまま意味を失う。
func startupID(t *testing.T, i int) string {
	t.Helper()

	checks := dom.Startup(dom.Default())
	if len(checks) <= i {
		t.Fatalf("起動時の項目 = %d 件, want %d 件以上", len(checks), i+1)
	}
	return checks[i].ID()
}

// 再実行の結果は起動時の前提チェックの件数として親へ届く（FR-44 / FR-34）。
//
// 届けないと、sudo や docker グループを直して Doctor タブで再実行しても
// `⚠ ホスト前提 N 件（5 で詳細）` がセッション終了まで残り、全て OK になった
// タブへ誘導し続ける。
//
// **数えるのは起動時の項目だけである。** タブは登録された項目をすべて走らせる
// ので、全体の件数を渡すとヘッダと状態行が別のものを数えた値になる。
func TestRerunReportsStartupCountToParent(t *testing.T) {
	t.Parallel()

	first, second := startupID(t, 0), startupID(t, 1)
	const cat = "ジョブ実行の前提"

	tests := map[string]struct {
		results []dom.CheckResult
		want    int
	}{
		"起動時の項目の不備を数える": {
			results: []dom.CheckResult{
				result(first, cat, "build01", dom.Fail),
				result(second, cat, "", dom.OK),
			},
			want: 1,
		},
		"直っていれば 0 件を届ける": {
			results: []dom.CheckResult{
				result(first, cat, "build01", dom.OK),
				result(second, cat, "", dom.Skip),
			},
			want: 0,
		},
		"起動時以外の不備は数えない": {
			results: []dom.CheckResult{
				result(first, cat, "build01", dom.OK),
				result("net.reach", "ネットワーク", "", dom.Fail),
			},
			want: 0,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m, _ := activated(t)
			_, cmd := deliver(t, m, tt.results)

			got, ok := pagetest.HostReqOf(cmd)
			if !ok {
				t.Fatal("件数が親へ届いていない（ヘッダと状態行が古いまま残る）")
			}
			if got.Bad != tt.want {
				t.Errorf("親へ届いた件数 = %d, want %d", got.Bad, tt.want)
			}
		})
	}
}

// 個別再実行でもマージ後の結果全体から数え直す（FR-34）。
//
// 1 項目だけを直したときに数え直さないと、その項目の警告だけが残り続ける。
func TestSingleRecheckRecountsStartup(t *testing.T) {
	t.Parallel()

	id := startupID(t, 0)
	m, _ := activated(t)
	m, cmd := deliver(t, m, []dom.CheckResult{
		result(id, "ジョブ実行の前提", "build01", dom.Fail),
	})
	if got, _ := pagetest.HostReqOf(cmd); got.Bad != 1 {
		t.Fatalf("全体再実行で届いた件数 = %d, want 1", got.Bad)
	}

	_, cmd = send(t, m, doneMsg{
		id:      id,
		results: []dom.CheckResult{result(id, "ジョブ実行の前提", "build01", dom.OK)},
		at:      finishedAt,
	})

	got, ok := pagetest.HostReqOf(cmd)
	if !ok {
		t.Fatal("個別再実行のあとに件数が届いていない")
	}
	if got.Bad != 0 {
		t.Errorf("個別再実行で届いた件数 = %d, want 0", got.Bad)
	}
}

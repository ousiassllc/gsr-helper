package discovery

import (
	"errors"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// errBudget は期限切れの周期を模した値。
var errBudget = errors.New("検出が間に合いませんでした")

// 期限切れ・失敗した周期の部分結果では直前の成功結果を上書きしない。
//
// runner.Discover は ctx がキャンセルされた時点で残りの systemctl show を発行せず、
// 取れた分だけを返す。その部分結果を採ると systemd 管理の runner が run.sh / - と
// 誤表示され、⚠ が誤って点き、孤児ユニットも過少報告される。
func TestReconcileKeepsLastResultOnError(t *testing.T) {
	cur := runner.Result{
		Runners:     []runner.Runner{{Dir: "/srv/runner"}},
		OrphanUnits: []runner.SvcState{{Unit: "actions.runner.foo.old.service"}},
	}

	// 期限切れの周期。取れた分だけの部分結果（runner 0 台・孤児 0 件）が届く。
	out := Reconcile(1, cur, nil, Msg{Seq: 2, Result: runner.Result{}, Err: errBudget})
	if out.Stale {
		t.Fatal("失敗した周期を追い抜かれた周期として捨てている")
	}
	if len(out.Result.Runners) != 1 || len(out.Result.OrphanUnits) != 1 {
		t.Errorf("部分結果で上書きしている: %+v", out.Result)
	}
	if !errors.Is(out.Err, errBudget) {
		t.Errorf("Outcome.Err = %v, want %v（状態行に警告が出ない）", out.Err, errBudget)
	}
	if out.Applied != 2 {
		t.Errorf("Outcome.Applied = %d, want 2（失敗した周期も取り込み済みとして進める）", out.Applied)
	}
	// **起動時の前提チェック（FR-44）は成功周期でだけ許可する。** 失敗した周期で
	// 許可すると、1 度きりの実行を空の Runners で使い切り、runner ごとに判定する
	// 2 項目がセッション中一度も走らず警告も出ない。
	if out.StartHostReq {
		t.Error("失敗した周期で起動時の前提チェックを許可している")
	}
}

// 成功した周期では一覧を置き換え、直前の失敗のエラーを消す。
func TestReconcileReplacesOnSuccess(t *testing.T) {
	cur := runner.Result{Runners: []runner.Runner{{Dir: "/srv/old"}}}
	next := runner.Result{Runners: []runner.Runner{{Dir: "/srv/new"}}}

	out := Reconcile(2, cur, errBudget, Msg{Seq: 3, Result: next, Err: nil})
	if len(out.Result.Runners) != 1 || out.Result.Runners[0].Dir != "/srv/new" {
		t.Errorf("成功した周期で結果が更新されていない: %+v", out.Result)
	}
	if out.Err != nil {
		t.Errorf("Outcome.Err = %v, want nil（直前の失敗が状態行に残り続ける）", out.Err)
	}
	if out.Applied != 3 {
		t.Errorf("Outcome.Applied = %d, want 3", out.Applied)
	}
	if !out.StartHostReq {
		t.Error("成功した周期で起動時の前提チェックが許可されていない")
	}
}

// 追い抜かれた周期は丸ごと捨て、保持していた値をそのまま返す。
func TestReconcileDropsStaleCycle(t *testing.T) {
	cur := runner.Result{Runners: []runner.Runner{{Dir: "/srv/new"}}}

	out := Reconcile(2, cur, nil, Msg{Seq: 1, Result: runner.Result{}, Err: nil})
	if !out.Stale {
		t.Fatal("追い抜かれた周期を捨てていない")
	}
	if out.Applied != 2 {
		t.Errorf("Outcome.Applied = %d, want 2（取り込み済みの番号を巻き戻している）", out.Applied)
	}
	if out.StartHostReq {
		t.Error("捨てる周期で起動時の前提チェックを許可している")
	}
}

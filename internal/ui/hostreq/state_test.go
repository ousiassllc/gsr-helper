package hostreq_test

import (
	"sync/atomic"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/ui/hostreq"
)

// 前提チェックの進行状況（1 度きり・項目が空なら発行しない・件数の取り込み）を
// 検証する。親 Model 側の配線——いつ発行するか、届いた件数を chrome の入力へ写すか
// ——は internal/ui の hostreq_test が見る。

// 起動時の前提チェックは 1 度しか発行しない。
//
// 判定対象はホストの構成であって秒単位で変わらず、`sudo -l -U` は監査ログに
// 記録されるため、再検出のたびに走らせると他のレコードを押し流す。
func TestStateStartsOnlyOnce(t *testing.T) {
	t.Parallel()

	var runs atomic.Int32
	s := hostreq.State{Checks: []doctor.Check{stub{id: "a", status: check.Fail, runs: &runs}}}

	cmd := s.StartOnce(doctor.Input{})
	if cmd == nil {
		t.Fatal("1 度目が発行されない")
	}
	if s.StartOnce(doctor.Input{}) != nil {
		t.Error("2 度目が発行されている（再検出のたびに sudo -l -U が走る）")
	}

	run(t, cmd)
	if got := runs.Load(); got != 1 {
		t.Errorf("実行回数 = %d, want 1", got)
	}
}

// 項目が空なら発行せず、「1 度きり」も使い切らない。
//
// 使い切ると、runner ごとに判定する 2 項目（NOPASSWD sudo / docker グループ所属）が
// セッション中一度も走らないままになる。
func TestStateWithoutChecksDoesNotConsumeTheOneShot(t *testing.T) {
	t.Parallel()

	var s hostreq.State
	if s.StartOnce(doctor.Input{}) != nil {
		t.Fatal("項目が無いのに発行された")
	}

	s.Checks = []doctor.Check{stub{id: "a", status: check.Fail, runs: nil}}
	if s.StartOnce(doctor.Input{}) == nil {
		t.Error("項目が入っても発行されない（1 度きりを空振りで使い切っている）")
	}
}

// 届いた件数を取り込み、届き直せば上書きする。
//
// 据え置くと、sudo や docker グループを直して Doctor タブで再実行しても、
// 全て OK になったタブへ誘導し続けることになる。
func TestStateAppliesCount(t *testing.T) {
	t.Parallel()

	var s hostreq.State
	if got := s.Bad(); got != 0 {
		t.Errorf("取り込む前の件数 = %d, want 0", got)
	}

	s.Apply(hostreq.Msg{Bad: 2})
	if got := s.Bad(); got != 2 {
		t.Errorf("件数 = %d, want 2", got)
	}

	s.Apply(hostreq.Msg{Bad: 0})
	if got := s.Bad(); got != 0 {
		t.Errorf("再実行の結果が届いても古い件数が残っている: %d, want 0", got)
	}
}

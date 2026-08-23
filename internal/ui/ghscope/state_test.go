package ghscope_test

import (
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/ui/ghscope"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// 取得の進行状況（1 度だけ・トークンが無ければ発行しない）を検証する。

func TestStateStartsOnlyOnce(t *testing.T) {
	var s ghscope.State

	if s.Start(exec.NewFake(), true) == nil {
		t.Fatal("1 度目が始まらない")
	}
	if s.Start(exec.NewFake(), true) != nil {
		t.Error("2 度目が始まっている（保有スコープはトークンごとの属性で毎回引く必要は無い）")
	}
}

// トークンを取得できない環境では引きに行かない。
//
// 判定は塞がない側に倒れるので、発行しないことによる不利は無い。
func TestStateWithoutTokenDoesNotStart(t *testing.T) {
	var s ghscope.State

	if s.Start(exec.NewFake(), false) != nil {
		t.Error("トークンが無いのに取得を始めている")
	}
	if s.Scopes().Known {
		t.Error("Known = true, want false（引いていないのに判定済みになっている）")
	}
}

// 取り込む前は Known が偽で、操作を塞がない。
func TestStateIsUnknownBeforeApply(t *testing.T) {
	var s ghscope.State

	if s.Scopes().Known {
		t.Error("取得前から Known = true になっている")
	}
	s.Apply(ghscope.Msg{State: page.ScopeState{
		Scopes: gh.Scopes{Held: []string{"repo"}, Classic: true}, Known: true,
	}})
	if !s.Scopes().Known || !s.Scopes().Scopes.Has("repo") {
		t.Errorf("取り込んだ結果が反映されていない: %+v", s.Scopes())
	}
}

// Package ghscope はトークンの保有スコープの取得を親 Model の代わりに駆動する
// （Issue #79）。
//
// 親 Model（ui.App）から分けているのは internal/ui/hostreq / internal/ui/workscan と
// 同じ理由による。取得は GitHub API への往復という副作用であり、internal/ui 直下は
// 1 ディレクトリ 2000 行の上限に対して余裕が無い。
//
// **能力判定（appconfig.Caps）には載せない。** hostcaps.Detect は起動シーケンス上で
// 同期的に走り 800ms の予算を持つが、保有スコープの確認は API への往復を要する。
// Caps に混ぜると「起動から一覧表示まで 1 秒以内」（non-functional.md）を壊す。
// 取得は起動後に非同期で 1 度だけ行い、確定した時点で共有状態（page.ScopeState）
// として配る。
//
// **フッタの描画から呼んではならない。** 可否の判定（action.Allow）は 1 フレームに
// 何度も走る。そこから API を叩けないことが、そもそもこの経路が要る理由である
// （docs/ui/screens.md「無効な操作の表示」の 6 段目）。
package ghscope

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// Budget は取得 1 回に与える上限。
//
// トークンの取得（gh auth token）と /rate_limit への 1 往復だけなので短くてよい。
// 上限を置くのは、応答しない相手で goroutine がプロセスの寿命ぶん残らないように
// するためである。期限切れは「取得できなかった」として扱い、操作は塞がない。
const Budget = 10 * time.Second

// Msg は保有スコープの取得結果。親はこれを共有状態へ写して配る。
type Msg struct {
	State page.ScopeState
}

// Start はトークンの保有スコープを引く Cmd を返す。
//
// newClient は API クライアントの生成を差し替える口である。**テストが本物の
// GitHub へ出ないようにするための継ぎ目でもある**（doctor の
// check.Input.NewClient と同じ形）。nil なら既定（gh.Token → gh.New）を使う。
//
// **失敗しても Known は真にしない。** 取得できなかったことと「スコープを
// 持っていない」ことは違う。真にすると、確認に失敗しただけで権限の足りている
// トークンの操作を塞ぐ。判定前と同じく塞がない側へ倒す（page.ScopeState.Known）。
func Start(ex exec.Executor, newClient func(context.Context) (*gh.Client, error)) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), Budget)
		defer cancel()

		sc, err := fetch(ctx, ex, newClient)
		if err != nil {
			return Msg{State: page.ScopeState{Scopes: gh.Scopes{}, Known: false}}
		}
		return Msg{State: page.ScopeState{Scopes: sc, Known: true}}
	}
}

// fetch はクライアントを組み立てて保有スコープを引く。
func fetch(ctx context.Context, ex exec.Executor, newClient func(context.Context) (*gh.Client, error)) (gh.Scopes, error) {
	client, err := build(ctx, ex, newClient)
	if err != nil {
		return gh.Scopes{}, err
	}
	return client.TokenScopes(ctx)
}

// build は API クライアントを返す。newClient が nil ならトークンから組み立てる。
func build(ctx context.Context, ex exec.Executor, newClient func(context.Context) (*gh.Client, error)) (*gh.Client, error) {
	if newClient != nil {
		return newClient(ctx)
	}
	token, err := gh.Token(ctx, ex)
	if err != nil {
		return nil, err
	}
	return gh.New(token)
}

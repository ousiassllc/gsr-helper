// Package job は計画（setup.Plan）を実行するのに要る外部資源を揃えて実行まで運ぶ。
//
// 短命トークンの取得・tarball の取得と検証・setup.Apply の呼び出しをここでまとめる。
// UI 層から分けているのは、この一連が bubbletea を知らない普通の関数として書け、
// TUI なしでテストできるためである（docs/architecture/overview.md の依存の方向）。
package job

import (
	"context"
	"sync"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/setup"
)

// tokenCache はスコープごとの短命トークンを 1 本だけ発行して使い回す。
//
// 一括追加では有効期限内なら 1 つのトークンを台数分の登録に使い回す（FR-14）。
// 削除は対象が複数のスコープにまたがりうるため（screens.md の削除の画面例）、
// スコープをキーにして持つ。
//
// 取得したトークンは Secrets へ預け、使い終わったら忘れる。預けている間だけ
// 監査ログとエラー文言の値一致マスクが効く（docs/architecture/security.md）。
type tokenCache struct {
	client  *gh.Client
	secrets *gh.Secrets

	mu     sync.Mutex
	byKey  map[string]gh.ShortToken
	issued []string
	now    func() time.Time
}

// newTokenCache は空のキャッシュを作る。
func newTokenCache(c *gh.Client, s *gh.Secrets) *tokenCache {
	return &tokenCache{
		client: c, secrets: s,
		mu: sync.Mutex{}, byKey: make(map[string]gh.ShortToken), issued: nil, now: time.Now,
	}
}

// forPlan は計画に応じた TokenFor を返す。短命トークンが不要なら nil を返す。
//
// 追加はフォームで指定した 1 つのスコープ、削除は台ごとのスコープを使う。
// 取得するトークンの種類も操作で変わる（registration / remove）。
func (t *tokenCache) forPlan(plan setup.Plan, sc scope.Scope) func(context.Context, setup.Unit) (string, error) {
	if !plan.NeedsToken {
		return nil
	}

	switch plan.Kind {
	case setup.KindAdd:
		return func(ctx context.Context, _ setup.Unit) (string, error) {
			return t.get(ctx, sc, setup.KindAdd)
		}
	case setup.KindRemove:
		return func(ctx context.Context, u setup.Unit) (string, error) {
			return t.get(ctx, u.Runner.Scope, setup.KindRemove)
		}
	case setup.KindUpdate:
		return nil
	default:
		return nil
	}
}

// get はスコープに対応する短命トークンを返す。期限内のものがあれば使い回す。
func (t *tokenCache) get(ctx context.Context, sc scope.Scope, kind setup.Kind) (string, error) {
	key := kind.String() + "\x00" + sc.String()

	t.mu.Lock()
	defer t.mu.Unlock()

	if tok, ok := t.byKey[key]; ok && tok.Valid(t.now()) {
		return tok.Value, nil
	}

	tok, err := t.issue(ctx, sc, kind)
	if err != nil {
		return "", err
	}

	t.byKey[key] = tok
	t.issued = append(t.issued, tok.Value)
	t.secrets.Add(tok.Value)

	return tok.Value, nil
}

// issue は操作に応じた短命トークンを取得する。
func (t *tokenCache) issue(ctx context.Context, sc scope.Scope, kind setup.Kind) (gh.ShortToken, error) {
	if kind == setup.KindRemove {
		return t.client.RemoveToken(ctx, sc)
	}
	return t.client.RegistrationToken(ctx, sc)
}

// forget は預けたトークンをマスク対象から外す。
//
// 短命（既定 1 時間）とはいえ、有効な間は登録・解除ができる強い権限を持つ。
// 使い終えたら参照を破棄する（docs/architecture/security.md）。
func (t *tokenCache) forget() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, v := range t.issued {
		t.secrets.Forget(v)
	}
	t.issued = nil
	t.byKey = make(map[string]gh.ShortToken)
}

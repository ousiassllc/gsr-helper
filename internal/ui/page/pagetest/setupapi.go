package pagetest

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/setup/job"
	"github.com/ousiassllc/gsr-helper/internal/setup/tarball"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// ErrNoFetchInTests はテストが tarball の取得へ到達したことを表すエラー。
//
// **外へ出るのではなく落とす。** UI の検証で本物のダウンロードが走る筋書きは無く、
// 到達したなら差し替えの穴か検証の組み立て間違いである。黙って成功させると
// その穴に気付けない。
var ErrNoFetchInTests = errors.New("テストでは tarball を取得しない")

// SetupAPI は Setup タブが触る外部資源（GitHub API と tarball の取得）を模した
// 差し替え一式であり、呼ばれた記録も持つ。
//
// **UI のテストを本物の GitHub から切り離すために置く。** 差し替えが無いと
// internal/setup/job は gh.Token へ落ち、周囲の GH_TOKEN を拾って
// api.github.com に短命トークン（remove-token）を発行してしまう
// （page.SetupDeps.NewClient の doc）。自己ホストの CI ではトークンが居る前提な
// ので、差し替えを共有の道具にして「どのタブのテストも外へ出ない」を 1 か所で
// 担保する。
//
// 記録を持たせるのは、差し替えが実際に効いているかを検証できるようにするためで
// ある。記録が空なら本物の経路を通った疑いがある。
type SetupAPI struct {
	mu      sync.Mutex
	paths   []string
	clients int
	fetches int
	// newClient は模したサーバへ向いたクライアントの生成。Deps が載せる。
	newClient func(ctx context.Context, d job.Deps) (*gh.Client, error)
}

// NewSetupAPI は GitHub API を模したサーバを立て、その差し替え一式を返す。
//
// cleanup には t.Cleanup を渡す。このパッケージは testing を import しない
// （pagetest.go の doc）ため、後始末の登録口を呼び出し側から受け取る。
func NewSetupAPI(cleanup func(func())) *SetupAPI {
	a := &SetupAPI{mu: sync.Mutex{}, paths: nil, clients: 0, fetches: 0, newClient: nil}
	srv := httptest.NewServer(http.HandlerFunc(a.serve))
	cleanup(srv.Close)

	// サーバを立ててからでないと向き先が決まらないので、組み立て後に入れる。
	a.newClient = func(context.Context, job.Deps) (*gh.Client, error) {
		a.record(&a.clients)
		return gh.New("pat", gh.WithBaseURL(srv.URL), gh.WithHTTPClient(srv.Client()))
	}
	return a
}

// serve は Setup タブが引く経路だけに応答する。
func (a *SetupAPI) serve(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	a.paths = append(a.paths, r.URL.Path)
	a.mu.Unlock()

	body, ok := setupAPIBody(r.URL.Path)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	// 書き込みの失敗はテストの相手（同一プロセスの http.Client）が消えたときだけで、
	// そのときは呼び出し側が先に落ちている。ここで報告する先が無い。
	_, _ = io.WriteString(w, body)
}

// setupAPIBody はパスに対する応答の本文を返す。知らないパスは偽を返す。
func setupAPIBody(path string) (string, bool) {
	switch {
	case strings.HasSuffix(path, "/registration-token"),
		strings.HasSuffix(path, "/remove-token"):
		return `{"token":"TOK","expires_at":"2099-01-01T00:00:00Z"}`, true
	case strings.HasSuffix(path, "/runners/downloads"):
		return `[{"os":"linux","architecture":"x64","download_url":"https://example.test/x",` +
			`"filename":"runner.tar.gz","sha256_checksum":"AA"}]`, true
	case strings.HasSuffix(path, "/releases/latest"):
		return `{"tag_name":"v2.311.0"}`, true
	default:
		return "", false
	}
}

// Deps は差し替えを載せた page.SetupDeps を返す。
//
// defaults は設定ファイルの既定（appconfig.Default().Defaults など）を渡す。
// 既定を振るテストは戻り値の Defaults だけを差し替える。
func (a *SetupAPI) Deps(host string, defaults appconfig.Defaults) page.SetupDeps {
	return page.SetupDeps{
		Host:      host,
		Defaults:  defaults,
		Secrets:   gh.NewSecrets(),
		NewClient: a.newClient,
		Fetch: func(context.Context, tarball.Info, string) (string, error) {
			a.record(&a.fetches)
			return "", ErrNoFetchInTests
		},
	}
}

// Clients は API クライアントを作った回数を返す。
func (a *SetupAPI) Clients() int { return a.read(&a.clients) }

// Fetches は tarball の取得を試みた回数を返す。
func (a *SetupAPI) Fetches() int { return a.read(&a.fetches) }

// Paths は模したサーバが受けたパスを受けた順に返す。
func (a *SetupAPI) Paths() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.paths...)
}

// record は数え上げを 1 つ進める。差し替えは worker の goroutine から呼ばれる。
func (a *SetupAPI) record(n *int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	*n++
}

// read は数え上げを読む。
func (a *SetupAPI) read(n *int) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return *n
}

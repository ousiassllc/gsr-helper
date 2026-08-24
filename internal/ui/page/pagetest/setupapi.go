package pagetest

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/setup/job"
	"github.com/ousiassllc/gsr-helper/internal/setup/tarball"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// WideBody はコマンド全文が 1 行に収まる本体の幅。既定の 100 桁では切り詰められる。
const WideBody = 160

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

// WaitFetches は tarball の取得が試みられるまで待ち、その回数を返す。
//
// **Fetch は承認後に走る worker の goroutine から呼ばれる**（Deps の doc）ため、
// 承認のキーを送った直後に Fetches() を読むと、まだ 0 のことがある。実際に
// CI（負荷の高い環境）でだけ 0 になって落ちた。取得そのものは同期的に待てる
// 対象ではないので、上限つきで待つ。
//
// 上限に達しても 0 を返すだけで、失敗の判定は呼び出し側に委ねる。
func (a *SetupAPI) WaitFetches(timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for {
		if n := a.Fetches(); n > 0 {
			return n
		}
		if time.Now().After(deadline) {
			return 0
		}

		time.Sleep(waitPoll)
	}
}

// waitPoll は WaitFetches の見に行く間隔。
const waitPoll = 5 * time.Millisecond

// read は数え上げを読む。
func (a *SetupAPI) read(n *int) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return *n
}

// SetupState は Setup タブ用の共有状態と、そこに載せた外部資源の差し替えを返す。
//
// **外部資源は必ず差し替える。** 差し替えないと internal/setup/job は gh.Token へ
// 落ち、周囲の GH_TOKEN で本物の api.github.com へ短命トークンを発行してしまう
// （page.SetupDeps.NewClient の doc。テストは t.Parallel を使うので t.Setenv でも
// 塞げない）。**Setup タブの共有状態は必ずここを通して作ること。**
//
// cleanup には t.Cleanup を渡す（NewSetupAPI と同じ理由）。
func SetupState(cleanup func(func()), rs ...runner.Runner) (page.StateMsg, *SetupAPI) {
	api := NewSetupAPI(cleanup)
	st := State(100, 30, rs...)
	st.Setup = api.Deps("build01", appconfig.Default().Defaults)
	return st, api
}

// FakeOf は共有状態の Executor をテスト実装として取り出す。
//
// 偽を返すのは差し替えが外れた場合だけで、そのときは検証の組み立てを疑うこと。
func FakeOf(st page.StateMsg) (*exec.Fake, bool) {
	f, ok := st.Exec.(*exec.Fake)
	return f, ok
}

// ChromeAfter は領域を配り直し、そのとき発行された状態行とフッタを返す。
// 発行されていなければ cmdtest.ErrNotFound、戻らない Cmd で辿り切れなければ
// cmdtest.ErrCmdTimeout を返す（ChromeOf の返しをそのまま渡す）。
//
// 状態行は Cmd としてしか外へ出ない（page.ChromeMsg）ため、読むには何か 1 つ
// Msg を配る必要がある。表示を変えない tea.WindowSizeMsg を使う。
func ChromeAfter(m tea.Model) (page.ChromeMsg, error) {
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	return cmdtest.ChromeOf(cmd, cmdtest.CmdTimeout)
}

// RemoveRequest は runner の削除を一覧側から依頼する Msg を返す。
func RemoveRequest(rs ...runner.Runner) page.SetupRequestMsg {
	return page.SetupRequestMsg{Op: page.SetupRemove, Runners: rs}
}

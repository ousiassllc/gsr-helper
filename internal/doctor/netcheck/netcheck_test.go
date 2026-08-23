package netcheck_test

import (
	"context"
	"errors"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/doctor/netcheck"
)

// checkByID はこのパッケージの項目を識別子で引く。
func checkByID(t *testing.T, id string) check.Check {
	t.Helper()

	for _, c := range netcheck.Checks() {
		if c.ID() == id {
			return c
		}
	}
	t.Fatalf("項目 %q が Checks() に無い", id)
	return nil
}

// run は項目を 1 つ実行して結果を返す。
func run(t *testing.T, id string, in check.Input) []check.Result {
	t.Helper()
	return checkByID(t, id).Run(context.Background(), in)
}

// only は結果がちょうど 1 件であることを確かめてその 1 件を返す。
func only(t *testing.T, got []check.Result) check.Result {
	t.Helper()

	if len(got) != 1 {
		t.Fatalf("件数 = %d, want 1（%+v）", len(got), got)
	}
	return got[0]
}

// env は環境変数の差し替え口を返す。
func env(kv map[string]string) func(string) string {
	return func(name string) string { return kv[name] }
}

// okDial は必ず繋がるダイヤラを返し、要求された宛先を記録する。
func okDial(mu *sync.Mutex, seen *[]string) func(context.Context, string, string) (net.Conn, error) {
	return func(_ context.Context, _, addr string) (net.Conn, error) {
		mu.Lock()
		*seen = append(*seen, addr)
		mu.Unlock()
		client, server := net.Pipe()
		_ = server.Close()
		return client, nil
	}
}

// ネットワークの項目は起動時の自動判定（FR-44）に入れない。
// 起動時に外へ TCP を張ると、回線の遅い環境で起動そのものが待たされる。
func TestNetworkChecksAreNotRunAtStartup(t *testing.T) {
	t.Parallel()

	for _, c := range netcheck.Checks() {
		if c.Startup() {
			t.Errorf("%s が起動時の対象になっている", c.ID())
		}
		if got := c.Category(); got != check.CatNetwork {
			t.Errorf("%s の Category = %q, want %q", c.ID(), got, check.CatNetwork)
		}
	}
}

// 宛先ごとに 1 行を返す。1 つ落ちているだけで他が隠れないようにするため。
func TestReachReturnsOneRowPerEndpoint(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var seen []string
	in := check.Input{Dial: okDial(&mu, &seen), Getenv: env(nil)}

	got := run(t, "net.reach", in)
	if len(got) < 5 {
		t.Fatalf("件数 = %d, want 5 以上（github.com / api / pipelines / results / pkg-containers）", len(got))
	}
	for _, r := range got {
		if r.Status != check.OK {
			t.Errorf("%s の Status = %v, want %v", r.Summary, r.Status, check.OK)
		}
		if r.Target != "" {
			t.Errorf("Target = %q, want 空（ホスト全体の確認）", r.Target)
		}
	}

	// 仕様が挙げる宛先を実際に叩いている。
	mu.Lock()
	defer mu.Unlock()
	for _, want := range []string{
		"github.com:443", "api.github.com:443",
		"pipelines.actions.githubusercontent.com:443",
		"results-receiver.actions.githubusercontent.com:443",
		"pkg-containers.githubusercontent.com:443",
	} {
		if !slices.Contains(seen, want) {
			t.Errorf("宛先 %q へ接続していない（実際: %q）", want, seen)
		}
	}
}

// 並びは宣言順で固定する。実行のたびに行が入れ替わるとカーソルが意味を失う。
func TestReachKeepsDeclaredOrder(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var seen []string
	in := check.Input{Dial: okDial(&mu, &seen), Getenv: env(nil)}

	first := run(t, "net.reach", in)
	second := run(t, "net.reach", in)

	for i := range first {
		if first[i].Summary != second[i].Summary {
			t.Fatalf("%d 番目が実行ごとに変わる: %q → %q", i, first[i].Summary, second[i].Summary)
		}
	}
}

// 到達できなければ FAIL。ただしプロキシ配下では直接の TCP が塞がれているのが
// 正常なこともあるので WARN に落とす。
func TestReachSeverityDependsOnProxy(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		env  map[string]string
		want check.Status
	}{
		"プロキシ無し": {env: nil, want: check.Fail},
		"プロキシ配下": {env: map[string]string{"https_proxy": "http://proxy:3128"}, want: check.Warn},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			in := check.Input{
				Dial: func(context.Context, string, string) (net.Conn, error) {
					return nil, errors.New("接続できません")
				},
				Getenv: env(tt.env),
			}
			got := run(t, "net.reach", in)
			if len(got) == 0 {
				t.Fatal("結果が空")
			}
			for _, r := range got {
				if r.Status != tt.want {
					t.Errorf("%s の Status = %v, want %v", r.Summary, r.Status, tt.want)
				}
			}
		})
	}
}

// プロキシ設定の整合。
func TestProxy(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		env     map[string]string
		want    check.Status
		wantSub string
	}{
		"設定なし": {
			env: nil, want: check.OK, wantSub: "設定されていません",
		},
		"小文字と大文字が一致": {
			env: map[string]string{
				"https_proxy": "http://proxy:3128", "HTTPS_PROXY": "http://proxy:3128",
				"no_proxy": "localhost,127.0.0.1",
			},
			want: check.OK, wantSub: "整合",
		},
		"小文字と大文字が食い違う": {
			env: map[string]string{
				"https_proxy": "http://a:3128", "HTTPS_PROXY": "http://b:3128",
				"no_proxy": "localhost,127.0.0.1",
			},
			want: check.Warn, wantSub: "値が違う",
		},
		"URL として読めない": {
			env:  map[string]string{"https_proxy": "proxy:3128"},
			want: check.Fail, wantSub: "スキーム",
		},
		"no_proxy にホスト内の宛先が無い": {
			env:  map[string]string{"https_proxy": "http://proxy:3128"},
			want: check.Warn, wantSub: "no_proxy",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := only(t, run(t, "net.proxy", check.Input{Getenv: env(tt.env)}))
			if got.Status != tt.want {
				t.Errorf("Status = %v, want %v（Summary: %s / Detail: %s）",
					got.Status, tt.want, got.Summary, got.Detail)
			}
			if !strings.Contains(got.Summary+got.Detail, tt.wantSub) {
				t.Errorf("文言に %q が無い: %s / %s", tt.wantSub, got.Summary, got.Detail)
			}
		})
	}
}

// プロキシの認証情報を画面へ写さない。診断はスクリーンショットで共有されうる。
func TestProxyRedactsCredentials(t *testing.T) {
	t.Parallel()

	in := check.Input{Getenv: env(map[string]string{
		"https_proxy": "http://user:s3cret@proxy:3128",
		"HTTPS_PROXY": "http://user:s3cret@proxy:3128",
		"no_proxy":    "localhost,127.0.0.1",
	})}

	got := only(t, run(t, "net.proxy", in))
	for _, field := range []string{got.Summary, got.Detail, got.Impact, got.Remedy} {
		if strings.Contains(field, "s3cret") {
			t.Errorf("プロキシの認証情報が画面の文言へ写っている: %s", field)
		}
	}
	if !strings.Contains(got.Detail, "proxy:3128") {
		t.Errorf("Detail にホストが出ていない（判断材料が無い）: %s", got.Detail)
	}
}

// 食い違いの詳細にも認証情報を出さない。
func TestProxyRedactsCredentialsOnConflict(t *testing.T) {
	t.Parallel()

	in := check.Input{Getenv: env(map[string]string{
		"https_proxy": "http://user:s3cret@a:3128",
		"HTTPS_PROXY": "http://user:other@b:3128",
	})}

	got := only(t, run(t, "net.proxy", in))
	if got.Status != check.Warn {
		t.Fatalf("Status = %v, want %v", got.Status, check.Warn)
	}
	if strings.Contains(got.Detail, "s3cret") || strings.Contains(got.Detail, "other") {
		t.Errorf("認証情報が Detail へ写っている: %s", got.Detail)
	}
}

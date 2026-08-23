package gh_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// repoScope / orgScope / entScope はテスト用のスコープ。
func repoScope() scope.Scope {
	return scope.Scope{Kind: scope.Repo, Owner: "foo", Repo: "bar"}
}
func orgScope() scope.Scope {
	return scope.Scope{Kind: scope.Org, Owner: "foo", Repo: ""}
}
func entScope() scope.Scope {
	return scope.Scope{Kind: scope.Enterprise, Owner: "acme", Repo: ""}
}

// newClient は httptest のサーバへ向けた Client を作る。
func newClient(t *testing.T, h http.Handler) (*gh.Client, *httptest.Server) {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	c, err := gh.New("test-token", gh.WithBaseURL(srv.URL), gh.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("gh.New: %v", err)
	}
	return c, srv
}

func TestShortLivedTokenUsesScopedPath(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sc     scope.Scope
		fn     func(*gh.Client, context.Context, scope.Scope) (gh.ShortToken, error)
		path   string
		method string
	}{
		"repo の registration token": {
			sc:     repoScope(),
			fn:     (*gh.Client).RegistrationToken,
			path:   "/repos/foo/bar/actions/runners/registration-token",
			method: http.MethodPost,
		},
		"org の registration token": {
			sc:     orgScope(),
			fn:     (*gh.Client).RegistrationToken,
			path:   "/orgs/foo/actions/runners/registration-token",
			method: http.MethodPost,
		},
		"enterprise の registration token": {
			sc:     entScope(),
			fn:     (*gh.Client).RegistrationToken,
			path:   "/enterprises/acme/actions/runners/registration-token",
			method: http.MethodPost,
		},
		"repo の remove token": {
			sc:     repoScope(),
			fn:     (*gh.Client).RemoveToken,
			path:   "/repos/foo/bar/actions/runners/remove-token",
			method: http.MethodPost,
		},
		"enterprise の remove token": {
			sc:     entScope(),
			fn:     (*gh.Client).RemoveToken,
			path:   "/enterprises/acme/actions/runners/remove-token",
			method: http.MethodPost,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var gotPath, gotMethod string
			c, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotMethod = r.URL.Path, r.Method
				fmt.Fprint(w, `{"token":"AAA","expires_at":"2026-08-23T13:00:00Z"}`)
			}))

			tok, err := tt.fn(c, context.Background(), tt.sc)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if gotPath != tt.path {
				t.Errorf("パス = %q, want %q", gotPath, tt.path)
			}
			if gotMethod != tt.method {
				t.Errorf("メソッド = %q, want %q", gotMethod, tt.method)
			}
			if tok.Value != "AAA" {
				t.Errorf("トークン = %q, want %q", tok.Value, "AAA")
			}
			if tok.ExpiresAt.IsZero() {
				t.Error("有効期限が読めていない")
			}
		})
	}
}

func TestShortTokenStringIsMasked(t *testing.T) {
	t.Parallel()

	tok := gh.ShortToken{Value: "super-secret-value", ExpiresAt: time.Now()}
	if got := fmt.Sprintf("%v", tok); got != "gh.ShortToken(***)" {
		t.Errorf("%%v = %q, 平文が漏れないようマスクすること", got)
	}
	if got := tok.String(); got != "gh.ShortToken(***)" {
		t.Errorf("String() = %q, 平文が漏れないようマスクすること", got)
	}
}

func TestShortTokenValid(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		tok  gh.ShortToken
		want bool
	}{
		"十分に先なら使い回せる":   {gh.ShortToken{Value: "a", ExpiresAt: now.Add(time.Hour)}, true},
		"期限切れ間際は使い回さない": {gh.ShortToken{Value: "a", ExpiresAt: now.Add(time.Minute)}, false},
		"期限切れ":  {gh.ShortToken{Value: "a", ExpiresAt: now.Add(-time.Minute)}, false},
		"値が空":   {gh.ShortToken{Value: "", ExpiresAt: now.Add(time.Hour)}, false},
		"期限が不明": {gh.ShortToken{Value: "a", ExpiresAt: time.Time{}}, false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := tt.tok.Valid(now); got != tt.want {
				t.Errorf("Valid = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListRunnersFollowsPages(t *testing.T) {
	t.Parallel()

	var paths []string
	c, srv := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		if r.URL.Query().Get("page") == "1" {
			w.Header().Set("Link", `<`+baseOf(r)+`?page=2>; rel="next"`)
			fmt.Fprint(w, `{"total_count":2,"runners":[{"id":1,"name":"a","os":"linux","status":"online","busy":true,"labels":[{"name":"self-hosted"},{"name":"gpu"}]}]}`)
			return
		}
		fmt.Fprint(w, `{"total_count":2,"runners":[{"id":2,"name":"b","os":"linux","status":"offline","busy":false,"labels":[]}]}`)
	}))
	_ = srv

	got, err := c.ListRunners(context.Background(), orgScope())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("件数 = %d, want 2（次ページを辿ること）: %+v", len(got), got)
	}
	if got[0].ID != 1 || got[0].Name != "a" || !got[0].Busy {
		t.Errorf("1 件目 = %+v", got[0])
	}
	if len(got[0].Labels) != 2 || got[0].Labels[1] != "gpu" {
		t.Errorf("ラベル = %v, want [self-hosted gpu]", got[0].Labels)
	}
	if got[1].ID != 2 || got[1].Busy {
		t.Errorf("2 件目 = %+v", got[1])
	}
	if len(paths) != 2 {
		t.Fatalf("リクエスト数 = %d, want 2: %v", len(paths), paths)
	}
}

// baseOf は Link ヘッダに載せる絶対 URL を組み立てる。
func baseOf(r *http.Request) string {
	return "http://" + r.Host + r.URL.Path
}

func TestDeleteRunnerIssuesDelete(t *testing.T) {
	t.Parallel()

	var gotPath, gotMethod string
	c, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	}))

	if err := c.DeleteRunner(context.Background(), repoScope(), 42); err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotPath != "/repos/foo/bar/actions/runners/42" {
		t.Errorf("パス = %q", gotPath)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("メソッド = %q, want DELETE", gotMethod)
	}
}

func TestUnknownScopeIsRejectedBeforeAnyRequest(t *testing.T) {
	t.Parallel()

	called := false
	c, _ := newClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	unknown := scope.Scope{Kind: scope.Unknown, Owner: "", Repo: ""}
	if _, err := c.RegistrationToken(context.Background(), unknown); !errors.Is(err, gh.ErrUnknownScope) {
		t.Errorf("err = %v, want ErrUnknownScope", err)
	}
	if called {
		t.Error("スコープが判定できないのにリクエストを送っている")
	}
}

func TestContextCancellationStopsRequest(t *testing.T) {
	t.Parallel()

	c, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"token":"AAA"}`)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.RegistrationToken(ctx, repoScope()); err == nil {
		t.Error("キャンセル済みの ctx でエラーにならない")
	}
}

func TestListRunnersStopsAtPageCap(t *testing.T) {
	t.Parallel()

	n := 0
	c, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		// 常に「次がある」と答えても打ち切られることを確かめる。
		w.Header().Set("Link", `<`+baseOf(r)+`?page=`+strconv.Itoa(n+1)+`>; rel="next"`)
		fmt.Fprint(w, `{"total_count":1,"runners":[{"id":1,"name":"a"}]}`)
	}))

	if _, err := c.ListRunners(context.Background(), orgScope()); err != nil {
		t.Fatalf("err = %v", err)
	}
	if n > 100 {
		t.Errorf("リクエスト数 = %d, 上限で打ち切ること", n)
	}
}

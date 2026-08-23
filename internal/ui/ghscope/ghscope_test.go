package ghscope_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/ui/ghscope"
)

// 保有スコープの取得（Issue #79）を検証する。
//
// **本物の GitHub へ出ないよう、必ず newClient の継ぎ目を使う。** 既定の経路は
// gh.Token（gh auth token）を叩くため、周囲の GH_TOKEN を拾いかねない。

// serve は X-OAuth-Scopes を返すサーバへ向いたクライアントを作る継ぎ目を返す。
func serve(t *testing.T, h http.Handler) func(context.Context) (*gh.Client, error) {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return func(context.Context) (*gh.Client, error) {
		return gh.New("t", gh.WithBaseURL(srv.URL), gh.WithHTTPClient(srv.Client()))
	}
}

// run は Cmd を実行して Msg を取り出す。
func run(t *testing.T, cmd tea.Cmd) ghscope.Msg {
	t.Helper()

	msg, ok := cmd().(ghscope.Msg)
	if !ok {
		t.Fatal("ghscope.Msg が返っていない")
	}
	return msg
}

// classic PAT の保有スコープを引けたら Known が真になる。
func TestStartReportsHeldScopes(t *testing.T) {
	newClient := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo, admin:org")
		w.WriteHeader(http.StatusOK)
	}))

	got := run(t, ghscope.Start(nil, newClient)).State

	if !got.Known {
		t.Fatal("Known = false, want true（取得できている）")
	}
	if !got.Scopes.Classic {
		t.Error("Classic = false, want true")
	}
	if want := []string{"repo", "admin:org"}; !slices.Equal(got.Scopes.Held, want) {
		t.Errorf("Held = %q, want %q", got.Scopes.Held, want)
	}
}

// fine-grained PAT は Classic が偽のまま取得完了として扱う。
//
// 「スコープが無い」と混同すると、権限が十分なトークンを塞ぐ（gh.Scopes の doc）。
func TestStartReportsNonClassicToken(t *testing.T) {
	newClient := serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	got := run(t, ghscope.Start(nil, newClient)).State

	if !got.Known {
		t.Error("Known = false, want true（往復そのものは成功している）")
	}
	if got.Scopes.Classic {
		t.Error("Classic = true, want false（X-OAuth-Scopes が返っていない）")
	}
}

// 取得に失敗しても Known は偽のままにする。
//
// 真にすると、確認に失敗しただけで権限の足りているトークンの操作を塞ぐ。
func TestStartKeepsUnknownOnFailure(t *testing.T) {
	newClient := func(context.Context) (*gh.Client, error) {
		return nil, errors.New("トークンを取得できない")
	}

	got := run(t, ghscope.Start(nil, newClient)).State

	if got.Known {
		t.Error("Known = true, want false（取得に失敗している）")
	}
}

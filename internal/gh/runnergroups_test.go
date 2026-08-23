package gh_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// TestListRunnerGroupsPath は org が /orgs/、enterprise が /enterprises/ を叩くこと、
// 次ページを辿ることを見る。取り違えは型では気付けず 404 になるだけなので固定する。
func TestListRunnerGroupsPath(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sc   scope.Scope
		want string
	}{
		"org":        {sc: orgScope(), want: "/orgs/foo/actions/runner-groups"},
		"enterprise": {sc: entScope(), want: "/enterprises/acme/actions/runner-groups"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var paths []string
			c, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				if r.URL.Query().Get("page") == "1" {
					if got := r.URL.Query().Get("per_page"); got != "100" {
						t.Errorf("per_page = %q, want 100", got)
					}
					w.Header().Set("Link", `<`+baseOf(r)+`?page=2>; rel="next"`)
					fmt.Fprint(w, `{"total_count":2,"runner_groups":[{"id":1,"name":"Default"}]}`)
					return
				}
				fmt.Fprint(w, `{"total_count":2,"runner_groups":[{"id":2,"name":"gpu"}]}`)
			}))

			got, err := c.ListRunnerGroups(context.Background(), tt.sc)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if len(paths) != 2 {
				t.Fatalf("リクエスト数 = %d, want 2（次ページを辿ること）: %v", len(paths), paths)
			}
			if paths[0] != tt.want {
				t.Errorf("パス = %q, want %q", paths[0], tt.want)
			}
			if len(got) != 2 || got[0] != (gh.RunnerGroup{ID: 1, Name: "Default"}) {
				t.Fatalf("runner group = %+v", got)
			}
			if got[1] != (gh.RunnerGroup{ID: 2, Name: "gpu"}) {
				t.Errorf("2 件目 = %+v", got[1])
			}
		})
	}
}

// TestListRunnerGroupsRejectsRepoScope は repo スコープを送信前に弾くことを見る。
func TestListRunnerGroupsRejectsRepoScope(t *testing.T) {
	t.Parallel()

	called := false
	c, _ := newClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	_, err := c.ListRunnerGroups(context.Background(), repoScope())
	if !errors.Is(err, gh.ErrNoRunnerGroups) {
		t.Errorf("err = %v, want ErrNoRunnerGroups", err)
	}
	if called {
		t.Error("repo スコープなのにリクエストを送っている")
	}
}

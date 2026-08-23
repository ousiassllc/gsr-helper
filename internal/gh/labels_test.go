package gh_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// labelsJSON はラベル系 4 エンドポイントに共通のレスポンス本文。
const labelsJSON = `{"total_count":2,"labels":[{"id":1,"name":"self-hosted"},{"id":2,"name":"gpu"}]}`

// TestRunnerLabelEndpoints はメソッド・パス・リクエスト本文が仕様どおりかを見る。
// URL の形（特に {scope} の解決）を取り違えても型では気付けないため、ここで固定する。
func TestRunnerLabelEndpoints(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		sc     scope.Scope
		call   func(*gh.Client, context.Context, scope.Scope) ([]string, error)
		method string
		path   string
		body   string
	}{
		"取得は repo スコープのパスへ GET": {
			sc: repoScope(),
			call: func(c *gh.Client, ctx context.Context, sc scope.Scope) ([]string, error) {
				return c.RunnerLabels(ctx, sc, 42)
			},
			method: http.MethodGet,
			path:   "/repos/foo/bar/actions/runners/42/labels",
			body:   "",
		},
		"置換は org スコープのパスへ PUT": {
			sc: orgScope(),
			call: func(c *gh.Client, ctx context.Context, sc scope.Scope) ([]string, error) {
				return c.ReplaceRunnerLabels(ctx, sc, 7, []string{"gpu", "cuda"})
			},
			method: http.MethodPut,
			path:   "/orgs/foo/actions/runners/7/labels",
			body:   `{"labels":["gpu","cuda"]}`,
		},
		"置換で nil を渡しても null は送らない": {
			sc: orgScope(),
			call: func(c *gh.Client, ctx context.Context, sc scope.Scope) ([]string, error) {
				return c.ReplaceRunnerLabels(ctx, sc, 7, nil)
			},
			method: http.MethodPut,
			path:   "/orgs/foo/actions/runners/7/labels",
			body:   `{"labels":[]}`,
		},
		"追加は enterprise スコープのパスへ POST": {
			sc: entScope(),
			call: func(c *gh.Client, ctx context.Context, sc scope.Scope) ([]string, error) {
				return c.AddRunnerLabels(ctx, sc, 7, []string{"gpu"})
			},
			method: http.MethodPost,
			path:   "/enterprises/acme/actions/runners/7/labels",
			body:   `{"labels":["gpu"]}`,
		},
		"個別削除はラベル名をパスに付けて DELETE": {
			sc: repoScope(),
			call: func(c *gh.Client, ctx context.Context, sc scope.Scope) ([]string, error) {
				return c.RemoveRunnerLabel(ctx, sc, 42, "gpu run")
			},
			method: http.MethodDelete,
			path:   "/repos/foo/bar/actions/runners/42/labels/gpu run",
			body:   "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var gotPath, gotMethod, gotBody string
			c, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				gotPath, gotMethod = r.URL.Path, r.Method
				gotBody = strings.TrimSpace(string(b))
				fmt.Fprint(w, labelsJSON)
			}))

			got, err := tt.call(c, context.Background(), tt.sc)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if gotMethod != tt.method || gotPath != tt.path {
				t.Errorf("%s %s, want %s %s", gotMethod, gotPath, tt.method, tt.path)
			}
			if gotBody != tt.body {
				t.Errorf("本文 = %q, want %q", gotBody, tt.body)
			}
			if len(got) != 2 || got[0] != "self-hosted" || got[1] != "gpu" {
				t.Errorf("ラベル = %v, want [self-hosted gpu]", got)
			}
		})
	}
}

// TestRunnerLabelsReturnsAPIError は失敗が APIError（ヒント付き）になることを見る。
func TestRunnerLabelsReturnsAPIError(t *testing.T) {
	t.Parallel()

	c, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"Resource not accessible by personal access token"}`)
	}))

	_, err := c.RunnerLabels(context.Background(), repoScope(), 42)
	var apiErr *gh.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *gh.APIError", err)
	}
	if apiErr.Op != "runner_labels" || apiErr.Status != http.StatusForbidden {
		t.Errorf("APIError = %+v, want op=runner_labels status=403", apiErr)
	}
	if apiErr.Hint == "" {
		t.Error("権限不足のヒントが空（次の一手を示すこと）")
	}
}

package authz_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/doctor/authz"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// checkByID はこのパッケージの項目を識別子で引く。
func checkByID(t *testing.T, id string) check.Check {
	t.Helper()

	for _, c := range authz.Checks() {
		if c.ID() == id {
			return c
		}
	}
	t.Fatalf("項目 %q が Checks() に無い", id)
	return nil
}

func run(t *testing.T, id string, in check.Input) []check.Result {
	t.Helper()
	return checkByID(t, id).Run(context.Background(), in)
}

func only(t *testing.T, got []check.Result) check.Result {
	t.Helper()

	if len(got) != 1 {
		t.Fatalf("件数 = %d, want 1（%+v）", len(got), got)
	}
	return got[0]
}

// runnerDir は FSRoot 配下に runner のディレクトリを作り、指定した
// パーミッションでファイルを置く。
func runnerDir(t *testing.T, root, dir string, modes map[string]os.FileMode) {
	t.Helper()

	full := filepath.Join(root, dir)
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for name, mode := range modes {
		path := filepath.Join(full, name)
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatalf("Chmod: %v", err)
		}
	}
}

func newRunner(name string) runner.Runner {
	return runner.Runner{
		Dir:    "/opt/runners/" + name,
		Config: runner.Config{AgentName: name},
	}
}

func orgRunner(name string) runner.Runner {
	r := newRunner(name)
	r.Scope = scope.Scope{Kind: scope.Org, Owner: "acme"}
	return r
}

// scopesInput は X-OAuth-Scopes を返すサーバへ向けた Input を返す。
func scopesInput(t *testing.T, header string, hasHeader bool, runners []runner.Runner) check.Input {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hasHeader {
			w.Header().Set("X-OAuth-Scopes", header)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	return check.Input{
		Runners: runners,
		Caps:    appconfig.Caps{GitHubToken: true},
		NewClient: func(context.Context) (*gh.Client, error) {
			return gh.New("t", gh.WithBaseURL(srv.URL+"/"))
		},
	}
}

// この分類は起動時の自動判定（FR-44）に入れない。
func TestAuthzChecksAreNotRunAtStartup(t *testing.T) {
	t.Parallel()

	for _, c := range authz.Checks() {
		if c.Startup() {
			t.Errorf("%s が起動時の対象になっている", c.ID())
		}
		if got := c.Category(); got != check.CatAuthz {
			t.Errorf("%s の Category = %q, want %q", c.ID(), got, check.CatAuthz)
		}
	}
}

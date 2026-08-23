package authz_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/doctor/authz"
	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/procs"
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

// .credentials は所有者だけが読める状態でなければならない。
func TestPermJudgesCredentialsMode(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		modes map[string]os.FileMode
		want  check.Status
	}{
		"0600 と 0644 は適切": {
			modes: map[string]os.FileMode{".credentials": 0o600, ".runner": 0o644},
			want:  check.OK,
		},
		"credentials が group 読み取り可": {
			modes: map[string]os.FileMode{".credentials": 0o640, ".runner": 0o644},
			want:  check.Fail,
		},
		"credentials が other 読み取り可": {
			modes: map[string]os.FileMode{".credentials": 0o604, ".runner": 0o644},
			want:  check.Fail,
		},
		"runner が world-writable": {
			modes: map[string]os.FileMode{".credentials": 0o600, ".runner": 0o666},
			want:  check.Warn,
		},
		"両方とも無い runner は未設定": {
			modes: nil,
			want:  check.Skip,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			runnerDir(t, root, "/opt/runners/build01", tt.modes)
			in := check.Input{Runners: []runner.Runner{newRunner("build01")}, FSRoot: root}

			got := only(t, run(t, "authz.perm", in))
			if got.Status != tt.want {
				t.Errorf("Status = %v, want %v（Detail: %s）", got.Status, tt.want, got.Detail)
			}
			if got.Target != "build01" {
				t.Errorf("Target = %q, want %q", got.Target, "build01")
			}
		})
	}
}

// 所有者が runner の実行ユーザーと違えば注意を出す。
func TestPermJudgesOwner(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runnerDir(t, root, "/opt/runners/build01", map[string]os.FileMode{
		".credentials": 0o600, ".runner": 0o644,
	})

	r := newRunner("build01")
	// 実ファイルの所有者は必ずテスト実行ユーザーなので、別の UID を持つ
	// Listener を与えれば食い違いを作れる。
	r.Listener = &procs.Process{PID: 1, Kind: procs.Listener, Dir: r.Dir, UID: os.Getuid() + 1}

	got := only(t, run(t, "authz.perm", check.Input{Runners: []runner.Runner{r}, FSRoot: root}))
	if got.Status != check.Warn {
		t.Errorf("Status = %v, want %v（Detail: %s）", got.Status, check.Warn, got.Detail)
	}
}

// runner が 1 台も無ければ SKIP。
func TestPermWithoutRunners(t *testing.T) {
	t.Parallel()

	if got := only(t, run(t, "authz.perm", check.Input{})).Status; got != check.Skip {
		t.Errorf("Status = %v, want %v", got, check.Skip)
	}
}

// hidepid は 2 でなければ注意を出す。トークンが /proc から読めるためである。
func TestHidepid(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mounts string
		want   check.Status
	}{
		"hidepid=2": {
			mounts: "proc /proc proc rw,nosuid,nodev,noexec,relatime,hidepid=2 0 0\n",
			want:   check.OK,
		},
		"hidepid=invisible": {
			mounts: "proc /proc proc rw,hidepid=invisible 0 0\n",
			want:   check.OK,
		},
		"hidepid=1": {
			mounts: "proc /proc proc rw,hidepid=1 0 0\n",
			want:   check.Warn,
		},
		"設定なし": {
			mounts: "proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0\n",
			want:   check.Warn,
		},
		"proc の行が無い": {
			mounts: "/dev/sda1 / ext4 rw 0 0\n",
			want:   check.Skip,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			path := filepath.Join(root, "proc", "mounts")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(path, []byte(tt.mounts), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			got := only(t, run(t, "authz.hidepid", check.Input{FSRoot: root}))
			if got.Status != tt.want {
				t.Errorf("Status = %v, want %v（Detail: %s）", got.Status, tt.want, got.Detail)
			}
		})
	}
}

// /proc/mounts が読めなければ SKIP。
func TestHidepidWithoutMounts(t *testing.T) {
	t.Parallel()

	got := only(t, run(t, "authz.hidepid", check.Input{FSRoot: t.TempDir()}))
	if got.Status != check.Skip {
		t.Errorf("Status = %v, want %v", got.Status, check.Skip)
	}
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

func orgRunner(name string) runner.Runner {
	r := newRunner(name)
	r.Scope = scope.Scope{Kind: scope.Org, Owner: "acme"}
	return r
}

// org レベルの runner には admin:org が要る。gh auth login の既定では付かない。
func TestScopesDetectsMissingAdminOrg(t *testing.T) {
	t.Parallel()

	in := scopesInput(t, "repo, workflow, read:org", true, []runner.Runner{orgRunner("build01")})

	got := only(t, run(t, "authz.scopes", in))
	if got.Status != check.Warn {
		t.Fatalf("Status = %v, want %v（Detail: %s）", got.Status, check.Warn, got.Detail)
	}
	if !strings.Contains(got.Remedy, "gh auth refresh -h github.com -s admin:org") {
		t.Errorf("Remedy に gh auth refresh が無い: %s", got.Remedy)
	}
}

// 足りていれば OK。
func TestScopesSatisfied(t *testing.T) {
	t.Parallel()

	in := scopesInput(t, "repo, admin:org", true, []runner.Runner{orgRunner("build01")})

	if got := only(t, run(t, "authz.scopes", in)).Status; got != check.OK {
		t.Errorf("Status = %v, want %v", got, check.OK)
	}
}

// fine-grained PAT はスコープを持たない。**「不足」と扱ってはならない。**
func TestScopesFineGrainedTokenIsSkipped(t *testing.T) {
	t.Parallel()

	in := scopesInput(t, "", false, []runner.Runner{orgRunner("build01")})

	got := only(t, run(t, "authz.scopes", in))
	if got.Status != check.Skip {
		t.Errorf("Status = %v, want %v（Detail: %s）", got.Status, check.Skip, got.Detail)
	}
}

// トークンが無ければ SKIP（FAIL と区別する）。
func TestScopesWithoutToken(t *testing.T) {
	t.Parallel()

	in := check.Input{
		Runners: []runner.Runner{orgRunner("build01")},
		Caps:    appconfig.Caps{GitHubToken: false},
	}
	if got := only(t, run(t, "authz.scopes", in)).Status; got != check.Skip {
		t.Errorf("Status = %v, want %v", got, check.Skip)
	}
}

// 登録先を判定できる runner が無ければ、必要なスコープが決まらないので SKIP。
func TestScopesWithoutDeterminableScope(t *testing.T) {
	t.Parallel()

	in := check.Input{
		Runners: []runner.Runner{newRunner("build01")},
		Caps:    appconfig.Caps{GitHubToken: true},
	}
	if got := only(t, run(t, "authz.scopes", in)).Status; got != check.Skip {
		t.Errorf("Status = %v, want %v", got, check.Skip)
	}
}

// 必要なスコープが複数あれば行を分ける。並びは権限の強い順で固定する。
func TestScopesReturnsOneRowPerRequiredScope(t *testing.T) {
	t.Parallel()

	repo := newRunner("build02")
	repo.Scope = scope.Scope{Kind: scope.Repo, Owner: "acme", Repo: "app"}
	in := scopesInput(t, "workflow", true, []runner.Runner{orgRunner("build01"), repo})

	got := run(t, "authz.scopes", in)
	if len(got) != 2 {
		t.Fatalf("件数 = %d, want 2（%+v）", len(got), got)
	}
	if !strings.Contains(got[0].Summary, "admin:org") {
		t.Errorf("1 行目 = %q, want admin:org が先", got[0].Summary)
	}
	if !strings.Contains(got[1].Summary, "repo") {
		t.Errorf("2 行目 = %q, want repo", got[1].Summary)
	}
}

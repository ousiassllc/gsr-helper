package edit_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/gh"
)

// 書き込み前に退避すること（FR-38）。元の内容が .bak に残る。
func TestCommitBacksUpBeforeWriting(t *testing.T) {
	t.Parallel()

	r := sample(t, "build01-1", "PATH=/usr/bin\n")
	next, was := envValues(), envValues()
	next[envIndex(t, "PATH")] = "/opt/bin"
	was[envIndex(t, "PATH")] = "/usr/bin"

	c, err := edit.BuildEnv(edit.Loader{DropInRoot: ""}, r, next, was)
	if err != nil {
		t.Fatalf("BuildEnv() でエラー: %v", err)
	}
	if got := c.BackupPath(); got != filepath.Join(r.Dir, ".env.bak") {
		t.Errorf("退避先 = %q", got)
	}

	in := edit.CommitInput{Change: c, Runner: r, Client: nil}
	if cerr := edit.Commit(context.Background(), in); cerr != nil {
		t.Fatalf("Commit() でエラー: %v", cerr)
	}

	if got := read(t, filepath.Join(r.Dir, ".env")); got != "PATH=/opt/bin\n" {
		t.Errorf("書き込み後 = %q", got)
	}
	if got := read(t, filepath.Join(r.Dir, ".env.bak")); got != "PATH=/usr/bin\n" {
		t.Errorf("バックアップ = %q, want 元の内容", got)
	}
}

// 元のファイルが無い場合は退避せずに書けること。新規作成では戻す先が無い。
func TestCommitWithoutExistingFile(t *testing.T) {
	t.Parallel()

	r := sample(t, "build01-1", "")
	next := envValues()
	next[envIndex(t, "PATH")] = "/opt/bin"

	c, err := edit.BuildEnv(edit.Loader{DropInRoot: ""}, r, next, envValues())
	if err != nil {
		t.Fatalf("BuildEnv() でエラー: %v", err)
	}
	if err := edit.Commit(context.Background(), edit.CommitInput{
		Change: c, Runner: r, Client: nil,
	}); err != nil {
		t.Fatalf("Commit() でエラー: %v", err)
	}

	if got := read(t, filepath.Join(r.Dir, ".env")); got != "PATH=/opt/bin\n" {
		t.Errorf("書き込み後 = %q", got)
	}
}

// ラベルの変更が置換 API を叩くこと。runner の ID は一覧から名前で引き当てる。
func TestCommitReplacesLabels(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/labels") {
			gotMethod, gotPath = req.Method, req.URL.Path
			buf := make([]byte, req.ContentLength)
			_, _ = req.Body.Read(buf)
			gotBody = string(buf)
			fmt.Fprint(w, `{"total_count":1,"labels":[{"id":1,"name":"gpu"}]}`)

			return
		}
		fmt.Fprint(w, `{"total_count":1,"runners":[{"id":42,"name":"build01-1","os":"linux","status":"online","busy":false,"labels":[]}]}`)
	}))
	defer srv.Close()

	r := sample(t, "build01-1", "")
	c := edit.BuildLabels("build01-1", []string{"old"}, []string{"gpu", "cuda"})

	err := edit.Commit(context.Background(), edit.CommitInput{
		Change: c, Runner: r,
		Client: func(context.Context) (*gh.Client, error) {
			return gh.New("t", gh.WithBaseURL(srv.URL), gh.WithHTTPClient(srv.Client()))
		},
	})
	if err != nil {
		t.Fatalf("Commit() でエラー: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("メソッド = %s, want PUT", gotMethod)
	}
	if want := "/orgs/foo/actions/runners/42/labels"; gotPath != want {
		t.Errorf("パス = %s, want %s", gotPath, want)
	}
	if !strings.Contains(gotBody, "gpu") || !strings.Contains(gotBody, "cuda") {
		t.Errorf("本文 = %s, want 置き換えるラベル全量", gotBody)
	}
}

// GitHub 側に同名の runner がいなければ、書き込まずに理由を返すこと。
func TestCommitReportsMissingRunner(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"total_count":0,"runners":[]}`)
	}))
	defer srv.Close()

	r := sample(t, "build01-1", "")
	err := edit.Commit(context.Background(), edit.CommitInput{
		Change: edit.BuildLabels("build01-1", nil, []string{"gpu"}), Runner: r,
		Client: func(context.Context) (*gh.Client, error) {
			return gh.New("t", gh.WithBaseURL(srv.URL), gh.WithHTTPClient(srv.Client()))
		},
	})
	if !errors.Is(err, edit.ErrNoRunnerID) {
		t.Fatalf("エラー = %v, want ErrNoRunnerID", err)
	}
}

// 認証に失敗したら書き込まずに理由を返すこと。
func TestCommitReportsClientFailure(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("トークンがありません")
	err := edit.Commit(context.Background(), edit.CommitInput{
		Change: edit.BuildLabels("build01-1", nil, []string{"gpu"}), Runner: sample(t, "build01-1", ""),
		Client: func(context.Context) (*gh.Client, error) { return nil, sentinel },
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("エラー = %v, want %v を包んだもの", err, sentinel)
	}
}

// Loader が runner ディレクトリ直下のファイルを指すこと。
func TestLoaderPaths(t *testing.T) {
	t.Parallel()

	r := sample(t, "build01-1", "")
	ld := edit.Loader{DropInRoot: "/tmp/root"}

	if got, want := ld.EnvPath(r), filepath.Join(r.Dir, ".env"); got != want {
		t.Errorf("EnvPath() = %q, want %q", got, want)
	}
	if got, want := ld.PathPath(r), filepath.Join(r.Dir, ".path"); got != want {
		t.Errorf("PathPath() = %q, want %q", got, want)
	}

	got, ok := ld.DropInPath(r)
	if !ok {
		t.Fatal("ユニット名があるのに drop-in のパスが決まらない")
	}
	if want := "/tmp/root/" + r.UnitName + ".d/override.conf"; got != want {
		t.Errorf("DropInPath() = %q, want %q", got, want)
	}

	r.UnitName = ""
	if _, ok := ld.DropInPath(r); ok {
		t.Error("ユニット名が無いのに drop-in のパスが決まっている")
	}
}

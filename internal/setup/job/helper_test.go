package job_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/procs"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/setup/job"
	"github.com/ousiassllc/gsr-helper/internal/setup/tarball"
)

// api は GitHub API を模したサーバを立て、そこへ向いた Deps を返す。
//
// 発行されたパスを記録するので、スコープごとの呼び分けを検証できる。
func api(t *testing.T, ex exec.Executor) (job.Deps, *[]string) {
	t.Helper()

	paths := new([]string)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/registration-token"),
			strings.HasSuffix(r.URL.Path, "/remove-token"):
			fmt.Fprintf(w, `{"token":%q,"expires_at":"2099-01-01T00:00:00Z"}`, "TOK"+r.URL.Path)
		case strings.HasSuffix(r.URL.Path, "/runners/downloads"):
			fmt.Fprint(w, `[{"os":"linux","architecture":"x64","download_url":"https://example.test/x",`+
				`"filename":"runner.tar.gz","sha256_checksum":"AA"}]`)
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			fmt.Fprint(w, `{"tag_name":"v2.311.0"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	d := job.Deps{
		Exec:    ex,
		Secrets: gh.NewSecrets(),
		NewClient: func(context.Context, job.Deps) (*gh.Client, error) {
			return gh.New("pat", gh.WithBaseURL(srv.URL), gh.WithHTTPClient(srv.Client()))
		},
		Fetch: nil,
	}
	return d, paths
}

// fakeFetch は tarball を取得したことにして、空のファイルを置く。
func fakeFetch(t *testing.T, calls *int32) func(context.Context, tarball.Info, string) (string, error) {
	t.Helper()

	return func(_ context.Context, _ tarball.Info, dir string) (string, error) {
		atomic.AddInt32(calls, 1)
		p := filepath.Join(dir, "runner.tar.gz")
		return p, os.WriteFile(p, miniTarball(t), 0o600)
	}
}

// miniTarball は runner 本体を模した最小の tar.gz を返す。
func miniTarball(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	body := []byte("#!/bin/sh\n")
	for _, name := range []string{"config.sh", "svc.sh"} {
		hdr := &tar.Header{
			Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("準備に失敗: %v", err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatalf("準備に失敗: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}
	return buf.Bytes()
}

// testRunner はテスト用の runner を組み立てる。
func testRunner(name, dir, unit string, sc scope.Scope, running bool) runner.Runner {
	r := runner.Runner{
		Dir: dir, Config: runner.Config{AgentName: name}, Scope: sc,
		Version: "2.310.0", WorkDir: dir + "/_work", UnitName: unit, RunAsUser: "runner",
		Managed: runner.ManagedSystemd, Svc: nil, Listener: nil, Workers: nil,
	}
	if running {
		r.Listener = &procs.Process{PID: 1, Kind: procs.Listener, Dir: dir}
	}
	return r
}

// issued は Fake が記録したコマンド行を返す。
func issued(f *exec.Fake) []string {
	calls := f.Calls()
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		out = append(out, c.String())
	}
	return out
}

// count は s を含む要素の数を返す。
func count(paths []string, s string) int {
	n := 0
	for _, p := range paths {
		if strings.Contains(p, s) {
			n++
		}
	}
	return n
}

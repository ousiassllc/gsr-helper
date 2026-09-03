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
	"sync"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/procs"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/setup/job"
	"github.com/ousiassllc/gsr-helper/internal/setup/tarball"
)

// wantSHA256 は downloads エンドポイントが返す tarball のチェックサム。
//
// 実物と同じ 64 桁の 16 進にしてあるのは、job.Run がこの値を素通しで
// tarball.Fetch へ渡していること（AC-3）を、取り違えようのない形で確かめる
// ためである。短い目印だと別の値と偶然一致しうる。
const wantSHA256 = "3b1f8c2d4e6a70b95c13d82f4a6e0c7b19d35f8a2c4e60b7d91f3a5c7e9b0d24"

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
			fmt.Fprintf(w, `[{"os":"linux","architecture":"x64","download_url":"https://example.test/x",`+
				`"filename":"runner.tar.gz","sha256_checksum":%q}]`, wantSHA256)
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

// fetchLog は fake の Fetch が受け取った内容の記録。
//
// AC-3（展開に使う SHA-256 は downloads エンドポイントの値）は、Fetch へ実際に
// 渡った tarball.Info でしか確かめられない。捨ててしまうと job.go の詰め替えを
// 壊してもテストが通る。-race で読み書きするためロックで守る。
type fetchLog struct {
	mu    sync.Mutex
	calls int
	last  tarball.Info
}

// fetch は tarball を取得したことにして最小の tar.gz を置き、受け取った Info を記録する。
func (l *fetchLog) fetch(t *testing.T) func(context.Context, tarball.Info, string) (string, error) {
	t.Helper()

	return func(_ context.Context, in tarball.Info, dir string) (string, error) {
		l.mu.Lock()
		l.calls++
		l.last = in
		l.mu.Unlock()

		p := filepath.Join(dir, "runner.tar.gz")
		return p, os.WriteFile(p, miniTarball(t), 0o600)
	}
}

// count は Fetch が呼ばれた回数を返す。
func (l *fetchLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

// info は最後に Fetch が受け取った tarball.Info を返す。
func (l *fetchLog) info() tarball.Info {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.last
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

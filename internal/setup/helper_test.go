package setup_test

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/procs"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/setup"
)

// addSpec は最小限の妥当な AddSpec を返す。
func addSpec() setup.AddSpec {
	return setup.AddSpec{
		URL:           "https://github.com/orgs/foo",
		Scope:         scope.Scope{Kind: scope.Org, Owner: "foo", Repo: ""},
		NamePrefix:    "build01",
		Count:         3,
		StartIndex:    5,
		Names:         nil,
		Labels:        []string{"gpu"},
		WorkDir:       "_work",
		RunnerGroup:   "Default",
		Ephemeral:     false,
		DisableUpdate: false,
		InstallBase:   "/opt/runners",
		RunAsUser:     "",
		Version:       "2.311.0",
		Existing:      []string{"build01-1"},
		Busy:          nil,
	}
}

// testRunner はテスト用の runner を組み立てる。
func testRunner(name, dir, unit string, running, busy bool) runner.Runner {
	r := runner.Runner{
		Dir:       dir,
		Config:    runner.Config{AgentName: name},
		Scope:     scope.Scope{Kind: scope.Org, Owner: "foo", Repo: ""},
		Version:   "2.310.0",
		WorkDir:   dir + "/_work",
		UnitName:  unit,
		RunAsUser: "runner",
		Managed:   runner.ManagedUnknown,
		Svc:       nil,
		Listener:  nil,
		Workers:   nil,
	}
	if unit != "" {
		r.Managed = runner.ManagedSystemd
	}
	if running {
		r.Listener = &procs.Process{PID: 100, Kind: procs.Listener, Dir: dir}
	}
	if busy {
		r.Workers = []procs.Process{{PID: 200, Kind: procs.Worker, Dir: dir}}
	}
	return r
}

// phases は手順のフェーズ名を実行順に返す。
func phases(u setup.Unit) []string {
	out := make([]string, 0, len(u.Steps))
	for _, s := range u.Steps {
		out = append(out, s.Phase)
	}
	return out
}

// findExtract は展開の手順の Keep を返す。
func findExtract(t *testing.T, u setup.Unit) []string {
	t.Helper()

	for _, s := range u.Steps {
		if s.Kind == setup.StepExtract {
			return s.Keep
		}
	}
	t.Fatalf("展開の手順が無い: %v", phases(u))
	return nil
}

// makeTarball は runner 本体を模した最小の tar.gz を作り、そのパスを返す。
//
// tarball 側にも .runner / .env を入れてある。これが無いと FR-21（保持対象を
// 上書きしない）の検証が、そもそも衝突しないだけの空振りになる。中身は既存の
// ファイルと必ず食い違う値にして、上書きが起きれば読み取りで分かるようにする。
func makeTarball(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "runner.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}
	defer func() { _ = f.Close() }()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, e := range []struct {
		name string
		body string
	}{
		{"config.sh", "#!/bin/sh\n"},
		{"svc.sh", "#!/bin/sh\n"},
		{"bin/runnerversion", "2.311.0\n"},
		{".runner", `{"agentId":9999}`},
		{".env", "TARBALL_ENV=1\n"},
	} {
		hdr := &tar.Header{
			Name: e.name, Mode: 0o755, Size: int64(len(e.body)), Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("準備に失敗: %v", err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatalf("準備に失敗: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}
	return path
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

// addPlanIn は base 配下へ count 台追加する計画を返す。
func addPlanIn(t *testing.T, base string, count int) setup.Plan {
	t.Helper()

	spec := addSpec()
	spec.InstallBase = base
	spec.Count = count
	spec.Existing = nil

	p, err := setup.PlanAdd(spec)
	if err != nil {
		t.Fatalf("PlanAdd: %v", err)
	}
	return p
}

// Package setuptest は internal/setup のテストが共用するフィクスチャを置く。
//
// _test.go ではなく通常のパッケージなのは、**1 ディレクトリ 2000 行の上限**
// （docs/ui/atomic-design.md「ディレクトリの行数」）に収めるためである。中身は
// setup の型を組み立てる純粋なフィクスチャと、tar.gz を作る道具だけで、本番の
// 構造には 1 つも手を入れていない。
//
// 代償として本番からも import できてしまうので、`page/pagetest/import_test.go` の
// `fixtures` へ登録してある（`TestNoProductionCodeImportsTestFixtures` が検査する）。
//
// **`testing` を import する点だけは page/pagetest の規則の例外である。**
// あちらは「テスト用のフラグが本番のバイナリ側の依存に現れる」ことを避けて
// `testing` を持たず、合否の判定を呼び出し側の _test.go に残している（pagetest/run.go）。
// ここは `t.Helper()` と `t.Fatalf` で準備の失敗をその場で止める形を採った——
// フィクスチャの準備（tar.gz の作成・PlanAdd の成功）が失敗した場合に呼び出し側へ
// error を返しても、全呼び出し元が同じ 3 行を書くだけになるためである。
// 本番から import されないことは上記の検査が担保する。
package setuptest

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/runner/procs"
	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
	"github.com/ousiassllc/gsr-helper/internal/setup"
)

// AddSpec は最小限の妥当な setup.AddSpec を返す。
func AddSpec() setup.AddSpec {
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

// Runner はテスト用の runner を組み立てる。
func Runner(name, dir, unit string, running, busy bool) runner.Runner {
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

// Phases は手順のフェーズ名を実行順に返す。
func Phases(u setup.Unit) []string {
	out := make([]string, 0, len(u.Steps))
	for _, s := range u.Steps {
		out = append(out, s.Phase)
	}
	return out
}

// FindExtract は展開の手順の Keep を返す。
func FindExtract(t *testing.T, u setup.Unit) []string {
	t.Helper()

	for _, s := range u.Steps {
		if s.Kind == setup.StepExtract {
			return s.Keep
		}
	}
	t.Fatalf("展開の手順が無い: %v", Phases(u))
	return nil
}

// MakeTarball は runner 本体を模した最小の tar.gz を作り、そのパスを返す。
//
// tarball 側にも .runner / .env を入れてある。これが無いと FR-21（保持対象を
// 上書きしない）の検証が、そもそも衝突しないだけの空振りになる。中身は既存の
// ファイルと必ず食い違う値にして、上書きが起きれば読み取りで分かるようにする。
func MakeTarball(t *testing.T) string {
	t.Helper()

	// os.Create ではなく os.CreateTemp を使うのは、**gosec の G304 を抑制せずに
	// 済ませるため**である。このファイルは _test.go ではない（1 ディレクトリの行数
	// 上限のため通常のパッケージにしてある）ので、抑制を書くと本番コードの抑制の
	// 棚卸し（docs/environment/setup.md「抑制の方針」）にテスト用フィクスチャの
	// 行が混ざる。
	f, err := os.CreateTemp(t.TempDir(), "runner-*.tar.gz")
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
	return f.Name()
}

// Issued は Fake が記録したコマンド行を返す。
func Issued(f *exec.Fake) []string {
	calls := f.Calls()
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		out = append(out, c.String())
	}
	return out
}

// AddPlanIn は base 配下へ count 台追加する計画を返す。
func AddPlanIn(t *testing.T, base string, count int) setup.Plan {
	t.Helper()

	spec := AddSpec()
	spec.InstallBase = base
	spec.Count = count
	spec.Existing = nil

	p, err := setup.PlanAdd(spec)
	if err != nil {
		t.Fatalf("PlanAdd: %v", err)
	}
	return p
}

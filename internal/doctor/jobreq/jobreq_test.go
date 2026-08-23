package jobreq_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/doctor/jobreq"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// 起動時の自動判定（FR-44）が拾うのは「ジョブ実行の前提」の 4 項目だけである。
//
// docker daemon の稼働は `docker info` で daemon への往復が要り、ホスト内の
// 読み取りと軽量なコマンドという FR-44 の条件を外れる。この対応が崩れると、
// 起動が daemon の応答待ちに引きずられる。
func TestStartupSetIsExactlyTheFourJobRequirements(t *testing.T) {
	t.Parallel()

	var startup, other []string
	for _, c := range jobreq.Checks() {
		if c.Startup() {
			startup = append(startup, c.ID())
		} else {
			other = append(other, c.ID())
		}
	}
	slices.Sort(startup)

	want := []string{"job.buildx", "job.docker", "job.dockergroup", "job.sudo"}
	if !slices.Equal(startup, want) {
		t.Errorf("起動時の対象 = %q, want %q", startup, want)
	}
	if !slices.Equal(other, []string{"docker.daemon"}) {
		t.Errorf("起動時の対象外 = %q, want [docker.daemon]", other)
	}
}

// 分類は functional.md のチェック項目一覧表と揃える。
func TestCategories(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"docker.daemon":   check.CatDocker,
		"job.sudo":        check.CatJobReq,
		"job.docker":      check.CatJobReq,
		"job.buildx":      check.CatJobReq,
		"job.dockergroup": check.CatJobReq,
	}
	for _, c := range jobreq.Checks() {
		if got := c.Category(); got != want[c.ID()] {
			t.Errorf("%s の Category = %q, want %q", c.ID(), got, want[c.ID()])
		}
	}
}

// docker の欠落はジョブが必ず失敗するので FAIL、buildx は WARN である
// （runner-host-setup.md の「doctor での検出」の表）。
func TestDockerAndBuildxSeverity(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		id       string
		lookPath func(string) (string, error)
		result   exec.Result
		want     check.Status
	}{
		"docker がある":               {id: "job.docker", lookPath: lookOnly("docker"), result: okResult(""), want: check.OK},
		"docker が無い":               {id: "job.docker", lookPath: lookOnly(), result: okResult(""), want: check.Fail},
		"buildx がある":               {id: "job.buildx", lookPath: lookOnly("docker"), result: okResult("github.com/docker/buildx v0.14.0"), want: check.OK},
		"buildx が無い":               {id: "job.buildx", lookPath: lookOnly("docker"), result: failResult(125, "unknown command"), want: check.Warn},
		"docker が無いので buildx は未判定": {id: "job.buildx", lookPath: lookOnly(), result: okResult(""), want: check.Skip},
		"daemon が応答する":             {id: "docker.daemon", lookPath: lookOnly("docker"), result: okResult("27.0.3"), want: check.OK},
		"daemon が応答しない":            {id: "docker.daemon", lookPath: lookOnly("docker"), result: failResult(1, "Cannot connect to the Docker daemon"), want: check.Fail},
		"docker が無いので daemon は未判定": {id: "docker.daemon", lookPath: lookOnly(), result: okResult(""), want: check.Skip},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			in := check.Input{
				Runners:  []runner.Runner{newRunner("build01", "runner", 0)},
				Exec:     fakeExec(func(string, []string) (exec.Result, error) { return tt.result, nil }),
				LookPath: tt.lookPath,
			}
			if got := only(t, run(t, tt.id, in)).Status; got != tt.want {
				t.Errorf("Status = %v, want %v", got, tt.want)
			}
		})
	}
}

// 症状の文言は runner-host-setup.md の「欠けているものと症状」から採る。
// 利用者が画面の文言でホストのエラーログを検索できるようにするためである。
func TestImpactUsesRealSymptomStrings(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		id       string
		lookPath func(string) (string, error)
		result   exec.Result
		want     string
	}{
		"docker": {id: "job.docker", lookPath: lookOnly(), result: okResult(""),
			want: "docker: command not found"},
		"buildx": {id: "job.buildx", lookPath: lookOnly("docker"), result: failResult(125, ""),
			want: "BuildKit"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			in := check.Input{
				Runners:  []runner.Runner{newRunner("build01", "runner", 0)},
				Exec:     fakeExec(func(string, []string) (exec.Result, error) { return tt.result, nil }),
				LookPath: tt.lookPath,
			}
			got := only(t, run(t, tt.id, in))
			if !strings.Contains(got.Impact, tt.want) {
				t.Errorf("Impact に %q が無い: %s", tt.want, got.Impact)
			}
		})
	}
}

// Executor が配られていないのはホストの不備ではないので SKIP に倒す。
func TestNoExecutorIsSkippedNotFailed(t *testing.T) {
	t.Parallel()

	in := check.Input{
		Runners:  []runner.Runner{newRunner("build01", "runner", 0)},
		Exec:     nil,
		LookPath: lookOnly("sudo", "docker"),
	}
	for _, id := range []string{"job.sudo", "job.buildx", "docker.daemon"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			if got := only(t, run(t, id, in)).Status; got != check.Skip {
				t.Errorf("Status = %v, want %v", got, check.Skip)
			}
		})
	}
}

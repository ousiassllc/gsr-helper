package check_test

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh"
)

// fake は Of / Skipped の写しを確かめるための最小の Check。
type fake struct{}

func (fake) ID() string       { return "fake.id" }
func (fake) Category() string { return check.CatAuthz }
func (fake) Startup() bool    { return true }
func (fake) Run(context.Context, check.Input) []check.Result {
	return nil
}

var _ check.Check = fake{}

// Of は識別子・分類・起動時対象を写す。手で埋めると ID() と食い違いうる。
func TestOfCopiesIdentity(t *testing.T) {
	t.Parallel()

	got := check.Of(fake{}, check.Result{
		ID: "", Category: "", Target: "build01", Status: check.Warn,
		Summary: "要約", Detail: "詳細", Impact: "影響", Remedy: "対処", Startup: false,
	})
	if got.ID != "fake.id" {
		t.Errorf("ID = %q, want %q", got.ID, "fake.id")
	}
	if got.Category != check.CatAuthz {
		t.Errorf("Category = %q, want %q", got.Category, check.CatAuthz)
	}
	if !got.Startup {
		t.Error("Startup = false, want true（Check.Startup() の写し）")
	}
	if got.Target != "build01" || got.Summary != "要約" {
		t.Errorf("渡した値が失われた: %+v", got)
	}
}

// SKIP は対処すべき不備が見つかっていないので影響と対処を持たない。
func TestSkippedHasNoRemedy(t *testing.T) {
	t.Parallel()

	got := check.Skipped(fake{}, "要約", "docker がありません")
	if got.Status != check.Skip {
		t.Errorf("Status = %v, want %v", got.Status, check.Skip)
	}
	if got.Impact != "" || got.Remedy != "" {
		t.Errorf("Impact = %q / Remedy = %q, want どちらも空", got.Impact, got.Remedy)
	}
	if got.ID != "fake.id" {
		t.Errorf("ID = %q, want %q", got.ID, "fake.id")
	}
}

// 診断のコマンドは監査ログに記録する（security.md の記録対象外とする読み取りコマンド）。
func TestProbeRecordsAuditAndSetsDeadline(t *testing.T) {
	t.Parallel()

	f := exec.NewFake()
	in := check.Input{Exec: f}

	if _, err := in.Probe(context.Background(), "doctor.sudo", "sudo", "-l", "-U", "runner"); err != nil {
		t.Fatalf("Probe: %v", err)
	}

	calls := f.Calls()
	if len(calls) != 1 {
		t.Fatalf("呼び出し回数 = %d, want 1", len(calls))
	}
	if calls[0].Options.SkipAudit {
		t.Error("SkipAudit = true, want false（診断のコマンドは記録する）")
	}
	if calls[0].Options.Action != "doctor.sudo" {
		t.Errorf("Action = %q, want %q", calls[0].Options.Action, "doctor.sudo")
	}
	if !calls[0].HasDeadline {
		t.Error("期限が設定されていない（応答しないコマンドで診断が止まる）")
	}
	if got := calls[0].String(); got != "sudo -l -U runner" {
		t.Errorf("コマンド = %q, want %q", got, "sudo -l -U runner")
	}
}

// Executor が無いのはホストの不備ではないので、失敗ではなく識別可能なエラーにする。
func TestProbeWithoutExecutor(t *testing.T) {
	t.Parallel()

	var in check.Input
	res, err := in.Probe(context.Background(), "doctor.x", "docker", "info")
	if !errors.Is(err, check.ErrNoExecutor) {
		t.Errorf("err = %v, want %v", err, check.ErrNoExecutor)
	}
	if res.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1", res.ExitCode)
	}
}

// FSRoot は /proc や /etc の読み取りをテストから差し替えるための起点。
func TestPathAndReadFileUseFSRoot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "proc"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "proc", "mounts"), []byte("body"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	in := check.Input{FSRoot: dir}
	if got, want := in.Path("/proc/mounts"), filepath.Join(dir, "proc", "mounts"); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
	body, err := in.ReadFile("/proc/mounts")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(body) != "body" {
		t.Errorf("中身 = %q, want %q", body, "body")
	}

	// FSRoot が空なら実ファイルシステムのパスをそのまま使う。
	var bare check.Input
	if got := bare.Path("/proc/mounts"); got != "/proc/mounts" {
		t.Errorf("Path = %q, want %q", got, "/proc/mounts")
	}
}

func TestClockAndEnvSeams(t *testing.T) {
	t.Parallel()

	fixed := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	in := check.Input{
		Now:    func() time.Time { return fixed },
		Getenv: func(name string) string { return "値:" + name },
	}
	if got := in.Clock(); !got.Equal(fixed) {
		t.Errorf("Clock() = %v, want %v", got, fixed)
	}
	if got := in.Env("https_proxy"); got != "値:https_proxy" {
		t.Errorf("Env() = %q, want %q", got, "値:https_proxy")
	}

	// 差し替えが無ければ実環境を見る（値そのものは環境依存なので落ちないことだけ確かめる）。
	var bare check.Input
	if bare.Clock().IsZero() {
		t.Error("Clock() がゼロ値を返した")
	}
}

func TestLookAndHasSeams(t *testing.T) {
	t.Parallel()

	in := check.Input{
		LookPath: func(name string) (string, error) {
			if name == "docker" {
				return "/usr/bin/docker", nil
			}
			return "", errors.New("見つかりません")
		},
	}
	if got, err := in.Look("docker"); err != nil || got != "/usr/bin/docker" {
		t.Errorf("Look(docker) = %q, %v", got, err)
	}
	if !in.Has("docker") {
		t.Error("Has(docker) = false, want true")
	}
	if in.Has("buildx") {
		t.Error("Has(buildx) = true, want false")
	}
}

// 到達性の確認はテストから実ネットワークへ出さない。
func TestDialAddrUsesSeam(t *testing.T) {
	t.Parallel()

	var gotAddr, gotNet string
	in := check.Input{
		Dial: func(_ context.Context, network, addr string) (net.Conn, error) {
			gotNet, gotAddr = network, addr
			client, server := net.Pipe()
			_ = server.Close()
			return client, nil
		},
	}
	if err := in.DialAddr(context.Background(), "api.github.com:443"); err != nil {
		t.Fatalf("DialAddr: %v", err)
	}
	if gotNet != "tcp" || gotAddr != "api.github.com:443" {
		t.Errorf("ダイヤル先 = %s %s, want tcp api.github.com:443", gotNet, gotAddr)
	}

	want := errors.New("到達できません")
	bad := check.Input{Dial: func(context.Context, string, string) (net.Conn, error) { return nil, want }}
	if err := bad.DialAddr(context.Background(), "x:443"); !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}

// Client の差し替え口はトークン取得を経ずにクライアントを渡すためにある。
func TestClientUsesSeam(t *testing.T) {
	t.Parallel()

	want, err := gh.New("t")
	if err != nil {
		t.Fatalf("gh.New: %v", err)
	}
	in := check.Input{NewClient: func(context.Context) (*gh.Client, error) { return want, nil }}
	got, err := in.Client(context.Background())
	if err != nil {
		t.Fatalf("Client: %v", err)
	}
	if got != want {
		t.Error("差し替えたクライアントが返らなかった")
	}
}

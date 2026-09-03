package doctor_test

import (
	"context"
	"errors"
	"net"
	"os"
	"slices"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// capabilityBound は systemctl / docker / journalctl / gh の有無で判定が決まる項目。
//
// これらが無い環境でも本ツールは起動でき、該当する項目は FAIL ではなく SKIP を
// 返さなければならない（FR-32 の「能力不足で実行できないチェックは FAIL と
// 区別して SKIP を返す」）。FAIL にすると、docker を使わない運用のホストで赤が
// 消えず、本当の不備が読めなくなる。
var capabilityBound = []string{
	"authz.perm",         // .credentials が無い（未設定の runner）
	"authz.scopes",       // gh のトークンが無い
	"time.ntp",           // timedatectl が無い
	"resource.mem",       // /proc/meminfo を読めない
	"history.oom",        // journalctl が無い
	"docker.daemon",      // docker が無い
	"job.sudo",           // sudo が無い
	"job.buildx",         // docker が無い
	"job.dockergroup",    // /etc/group を読めない
	"systemd.unit",       // systemctl が無い
	"consistency.orphan", // systemctl が無い
	"consistency.units",  // systemctl が無い
}

// 能力が 1 つも無いホストでも診断は完走し、能力不足の項目は SKIP になる。
//
// **判定を実ホストに依存させない。** FSRoot を空のディレクトリに向け、LookPath を
// すべて失敗させ、ダイヤラを差し替えることで、CI と手元で同じ結果になる。
func TestDegradedHostSkipsCapabilityBoundChecks(t *testing.T) {
	t.Parallel()

	failing := exec.NewFake()
	failing.SetFunc(func(string, []string) (exec.Result, error) {
		return exec.Result{Stdout: nil, Stderr: nil, ExitCode: -1}, errors.New("コマンドがありません")
	})

	in := doctor.Input{
		Runners: []runner.Runner{{
			Dir:       "/nonexistent/runners/build01",
			Config:    runner.Config{AgentName: "build01"},
			RunAsUser: "runner",
			Managed:   runner.ManagedStandalone,
		}},
		Caps:     appconfig.Caps{},
		Exec:     failing,
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
		FSRoot:   t.TempDir(),
		Getenv:   func(string) string { return "" },
		Dial: func(context.Context, string, string) (net.Conn, error) {
			client, server := net.Pipe()
			_ = server.Close()
			return client, nil
		},
	}

	got := doctor.Run(context.Background(), in, doctor.Default())
	if len(got) == 0 {
		t.Fatal("診断が 1 件も結果を返さなかった")
	}

	byID := make(map[string][]doctor.CheckResult, len(got))
	for _, r := range got {
		byID[r.ID] = append(byID[r.ID], r)
	}

	for _, id := range capabilityBound {
		rows, ok := byID[id]
		if !ok {
			t.Errorf("%s が 1 行も返っていない（能力が無くても行は消さない）", id)
			continue
		}
		for _, r := range rows {
			if r.Status != doctor.Skip {
				t.Errorf("%s の判定 = %v, want %v（Detail: %s）", id, r.Status, doctor.Skip, r.Detail)
			}
		}
	}
}

// SKIP の行は対処を持たない。対処すべき不備が見つかっていないためである。
//
// 対処を出すと、docker を入れられない環境の運用者が「対処したのに消えない行」を
// 追い続けることになる。
func TestSkippedResultsHaveNoRemedy(t *testing.T) {
	t.Parallel()

	in := doctor.Input{
		Caps:     appconfig.Caps{},
		Exec:     exec.NewFake(),
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
		FSRoot:   t.TempDir(),
		Getenv:   func(string) string { return "" },
		Dial: func(context.Context, string, string) (net.Conn, error) {
			client, server := net.Pipe()
			_ = server.Close()
			return client, nil
		},
	}

	for _, r := range doctor.Run(context.Background(), in, doctor.Default()) {
		if r.Status == doctor.Skip && (r.Remedy != "" || r.Impact != "") {
			t.Errorf("%s は SKIP なのに影響（%q）か対処（%q）を持つ", r.ID, r.Impact, r.Remedy)
		}
	}
}

// 対処が要る判定（WARN / FAIL）は、検出内容・影響・推奨する対処をすべて持つ（FR-33）。
//
// どれかが欠けると、利用者は「何が起きていて次に何をすればよいか」を画面から
// 読み取れない。
func TestActionableResultsCarryDetailImpactAndRemedy(t *testing.T) {
	t.Parallel()

	in := doctor.Input{
		Caps:     appconfig.Caps{},
		Exec:     exec.NewFake(),
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
		FSRoot:   t.TempDir(),
		Getenv:   func(string) string { return "" },
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("到達できません")
		},
	}

	var seen int
	for _, r := range doctor.Run(context.Background(), in, doctor.Default()) {
		if !r.Status.Bad() {
			continue
		}
		seen++
		for label, v := range map[string]string{
			"要約": r.Summary, "検出内容": r.Detail, "影響": r.Impact, "推奨する対処": r.Remedy,
		} {
			if v == "" {
				t.Errorf("%s（%v）の%sが空", r.ID, r.Status, label)
			}
		}
	}
	if seen == 0 {
		t.Fatal("対処が要る行が 1 つも出ていない（検査が空振りしている）")
	}
}

// 一覧の並びは分類の順で固定される。実行のたびに入れ替わるとカーソルが意味を失う。
func TestRunOrdersByCategory(t *testing.T) {
	t.Parallel()

	in := doctor.Input{
		Caps:     appconfig.Caps{},
		Exec:     exec.NewFake(),
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
		FSRoot:   t.TempDir(),
		Getenv:   func(string) string { return "" },
		Dial: func(context.Context, string, string) (net.Conn, error) {
			client, server := net.Pipe()
			_ = server.Close()
			return client, nil
		},
	}

	got := doctor.Run(context.Background(), in, doctor.Default())
	order := doctor.Categories()

	last := -1
	for _, r := range got {
		i := slices.Index(order, r.Category)
		if i < last {
			t.Fatalf("分類 %q が順序どおりに並んでいない（%+v）", r.Category, got)
		}
		last = i
	}
}

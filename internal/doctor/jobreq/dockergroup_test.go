package jobreq_test

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// etcGroup は docker グループに runner が居る /etc/group。
const etcGroup = "root:x:0:\ndocker:x:998:runner,ci\nsudo:x:27:admin\n"

// **未所属と未反映は別の判定である。** グループの変更は既存プロセスに反映
// されないため、usermod 済みでも再起動していなければ permission denied は続く
// （FR-43、runner-host-setup.md「4. docker グループ」）。
func TestDockerGroupDistinguishesUnappliedFromMissing(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		group      string
		user       string
		listener   int
		procGroups []string
		want       check.Status
		wantSum    string
	}{
		"所属していて稼働中プロセスにも反映済み": {
			group: etcGroup, user: "runner", listener: 4242,
			procGroups: []string{"27", "998"},
			want:       check.OK, wantSum: "docker グループに所属",
		},
		"所属しているが稼働中プロセスに未反映": {
			group: etcGroup, user: "runner", listener: 4242,
			procGroups: []string{"27"},
			want:       check.Fail, wantSum: "docker グループが未反映",
		},
		"そもそも未所属": {
			group: etcGroup, user: "other", listener: 4242,
			procGroups: []string{"27"},
			want:       check.Fail, wantSum: "docker グループに未所属",
		},
		"所属していて稼働中プロセスが無い": {
			group: etcGroup, user: "runner", listener: 0,
			procGroups: nil,
			want:       check.OK, wantSum: "docker グループに所属",
		},
		"補助グループを 1 つも持たないプロセス": {
			group: etcGroup, user: "runner", listener: 4242,
			procGroups: []string{},
			want:       check.Fail, wantSum: "docker グループが未反映",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			status := map[int]string{}
			if tt.listener > 0 && tt.procGroups != nil {
				status[tt.listener] = procStatus(tt.procGroups...)
			}
			in := check.Input{
				Runners: []runner.Runner{newRunner("build01", tt.user, tt.listener)},
				FSRoot:  hostFS(t, tt.group, status),
			}

			got := only(t, run(t, "job.dockergroup", in))
			if got.Status != tt.want {
				t.Errorf("Status = %v, want %v（Detail: %s）", got.Status, tt.want, got.Detail)
			}
			if got.Summary != tt.wantSum {
				t.Errorf("Summary = %q, want %q", got.Summary, tt.wantSum)
			}
			if got.Target != "build01" {
				t.Errorf("Target = %q, want %q", got.Target, "build01")
			}
		})
	}
}

// 未反映の詳細は「所属しているが反映されていない」ことと PID を示す。
// 対処は usermod ではなく再起動だけである。
func TestDockerGroupUnappliedDetailNamesPIDAndRestart(t *testing.T) {
	t.Parallel()

	in := check.Input{
		Runners: []runner.Runner{newRunner("build01", "runner", 284102)},
		FSRoot:  hostFS(t, etcGroup, map[int]string{284102: procStatus("27")}),
	}

	got := only(t, run(t, "job.dockergroup", in))
	for _, want := range []string{"284102", "既存プロセス"} {
		if !strings.Contains(got.Detail, want) {
			t.Errorf("Detail に %q が無い: %s", want, got.Detail)
		}
	}
	if strings.Contains(got.Remedy, "usermod") {
		t.Errorf("Remedy に usermod が含まれる（所属済みなので再起動だけでよい）: %s", got.Remedy)
	}
	if !strings.Contains(got.Remedy, "systemctl restart") {
		t.Errorf("Remedy に再起動が無い: %s", got.Remedy)
	}
}

// 未所属の対処は usermod と再起動の両方を出す。
func TestDockerGroupMissingRemedyIncludesUsermodAndRestart(t *testing.T) {
	t.Parallel()

	in := check.Input{
		Runners: []runner.Runner{newRunner("build01", "other", 0)},
		FSRoot:  hostFS(t, etcGroup, nil),
	}

	got := only(t, run(t, "job.dockergroup", in))
	for _, want := range []string{"usermod -aG docker other", "systemctl restart"} {
		if !strings.Contains(got.Remedy, want) {
			t.Errorf("Remedy に %q が無い: %s", want, got.Remedy)
		}
	}
}

// 能力不足は FAIL と区別して SKIP を返す。
func TestDockerGroupSkips(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		group   string
		runners []runner.Runner
	}{
		"docker グループが無い": {
			group:   "root:x:0:\n",
			runners: []runner.Runner{newRunner("build01", "runner", 0)},
		},
		"/etc/group を読めない": {
			group:   "",
			runners: []runner.Runner{newRunner("build01", "runner", 0)},
		},
		"実行ユーザーの分かる runner が無い": {
			group:   etcGroup,
			runners: []runner.Runner{newRunner("build01", "", 0)},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			in := check.Input{Runners: tt.runners, FSRoot: hostFS(t, tt.group, nil)}
			got := only(t, run(t, "job.dockergroup", in))
			if got.Status != check.Skip {
				t.Errorf("Status = %v, want %v", got.Status, check.Skip)
			}
			if got.Remedy != "" {
				t.Errorf("SKIP なのに対処がある: %q", got.Remedy)
			}
		})
	}
}

// UID の 10 進表記でも、稼働中プロセスがあれば補助グループで判定する。
//
// /proc/<pid>/status の Groups 行は GID の並びなので、ユーザー名を解決できなくても
// docker の GID と直接比較できる。一律 SKIP にすると、仕様が最も想定している環境
// （NSS が使えない・LDAP 上のユーザー）でだけ FR-43 の 1 項目が常に未判定になる。
func TestDockerGroupNumericUserJudgesByProcGroups(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		listener   int
		procGroups []string
		want       check.Status
		wantSum    string
	}{
		"補助グループに docker の GID がある": {
			listener: 4242, procGroups: []string{"27", "998"},
			want: check.OK, wantSum: "docker グループに所属",
		},
		"補助グループに docker の GID が無い": {
			listener: 4242, procGroups: []string{"27"},
			want: check.Fail, wantSum: "docker グループが未反映",
		},
		"稼働中プロセスが無いので判定できない": {
			listener: 0, procGroups: nil,
			want: check.Skip, wantSum: "docker グループ所属",
		},
		"稼働中プロセスの補助グループを読めない": {
			listener: 4242, procGroups: nil,
			want: check.Warn, wantSum: "docker グループの反映を確認できない",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			status := map[int]string{}
			if tt.listener > 0 && tt.procGroups != nil {
				status[tt.listener] = procStatus(tt.procGroups...)
			}
			in := check.Input{
				Runners: []runner.Runner{newRunner("build01", "1001", tt.listener)},
				FSRoot:  hostFS(t, etcGroup, status),
			}

			got := only(t, run(t, "job.dockergroup", in))
			if got.Status != tt.want {
				t.Errorf("Status = %v, want %v（Detail: %s）", got.Status, tt.want, got.Detail)
			}
			if got.Summary != tt.wantSum {
				t.Errorf("Summary = %q, want %q", got.Summary, tt.wantSum)
			}
			if got.Target != "build01" {
				t.Errorf("Target = %q, want %q", got.Target, "build01")
			}
			// どの判定に転んでも、名前での突き合わせをしていないことを残す。
			// 残さないと、補助グループだけを見た OK が「メンバー欄も確認済み」と読まれる。
			if !strings.Contains(got.Detail, "/etc/group のメンバー欄") {
				t.Errorf("Detail に /etc/group との突き合わせができない旨が無い: %s", got.Detail)
			}
		})
	}
}

// UID 表記の FAIL は「未所属」と「未反映」を区別できない。
//
// どちらも Groups 行に GID が現れないという同じ形になる。区別できないことを Detail に
// 書いたうえで、対処は usermod を含む側を出す。既に所属しているユーザーへの usermod は
// 無害だが、未所属のユーザーを再起動しても直らないためである。usermod はログイン名しか
// 受け付けないので、UID をそのまま埋めた行は出さない。
func TestDockerGroupNumericUserFailRemedyDoesNotEmbedUID(t *testing.T) {
	t.Parallel()

	in := check.Input{
		Runners: []runner.Runner{newRunner("build01", "1001", 4242)},
		FSRoot:  hostFS(t, etcGroup, map[int]string{4242: procStatus("27")}),
	}

	got := only(t, run(t, "job.dockergroup", in))
	if got.Status != check.Fail {
		t.Fatalf("Status = %v, want %v（Detail: %s）", got.Status, check.Fail, got.Detail)
	}
	if !strings.Contains(got.Detail, "区別できません") {
		t.Errorf("Detail に未所属と未反映を区別できない旨が無い: %s", got.Detail)
	}
	for _, want := range []string{"usermod -aG docker", "systemctl restart"} {
		if !strings.Contains(got.Remedy, want) {
			t.Errorf("Remedy に %q が無い: %s", want, got.Remedy)
		}
	}
	if strings.Contains(got.Remedy, "usermod -aG docker 1001") {
		t.Errorf("Remedy が UID をそのまま埋めている（usermod はログイン名しか受け付けない）: %s", got.Remedy)
	}
}

// runner ごとに 1 行を返す（screens.md の Doctor タブは TARGET 列に runner 名を出す）。
func TestDockerGroupReturnsOneRowPerRunner(t *testing.T) {
	t.Parallel()

	in := check.Input{
		Runners: []runner.Runner{
			newRunner("build01", "runner", 100),
			newRunner("build02", "other", 200),
		},
		FSRoot: hostFS(t, etcGroup, map[int]string{
			100: procStatus("998"),
			200: procStatus("27"),
		}),
	}

	got := run(t, "job.dockergroup", in)
	if len(got) != 2 {
		t.Fatalf("件数 = %d, want 2", len(got))
	}
	if s := byTarget(t, got, "build01").Status; s != check.OK {
		t.Errorf("build01 の Status = %v, want %v", s, check.OK)
	}
	if s := byTarget(t, got, "build02").Status; s != check.Fail {
		t.Errorf("build02 の Status = %v, want %v", s, check.Fail)
	}
}

package runner

import (
	"slices"
	"testing"
	"time"
)

// docs/architecture/overview.md の「起動方式の判定」の表を網羅する。
func TestAttachManagedBy(t *testing.T) {
	lis := []Process{pr(100, ProcListener, d1, zero)}

	tests := []struct {
		name    string
		runner  Runner
		procs   []Process
		units   []SvcState
		managed ManagedBy
		running bool
		orphans []string
	}{
		{"ユニット（.service 経由）+ Listener", rn(d1, u1), lis, []SvcState{sv(u1, "")}, ManagedSystemd, true, nil},
		{"ユニット（WorkingDirectory 経由）+ Listener", rn(d1, ""), lis, []SvcState{sv(u1, d1)}, ManagedSystemd, true, nil},
		{"ユニットのみ（停止中）", rn(d1, u1), nil, []SvcState{sv(u1, "")}, ManagedSystemd, false, nil},
		{"Listener のみ（run.sh 直起動）", rn(d1, ""), lis, nil, ManagedStandalone, true, nil},
		{"どちらも無し（未稼働）", rn(d1, ""), nil, nil, ManagedUnknown, false, nil},
		{"対応ディレクトリなしのユニットは孤児", rn(d1, ""), nil, []SvcState{sv(u2, "/opt/gone")}, ManagedUnknown, false, []string{u2}},
		{"WorkingDir が空で .service にも無いユニットは孤児", rn(d1, ""), nil, []SvcState{sv(u2, "")}, ManagedUnknown, false, []string{u2}},
		// systemctl show が失敗したユニットは Unit だけのプレースホルダ。
		// 状態不明であってディレクトリが消えたわけではないので孤児にしない。
		{"show 失敗のプレースホルダは孤児にしない", rn(d1, ""), nil, []SvcState{{Unit: u2}}, ManagedUnknown, false, nil},
		// 走査ルート外の runner でもプロセス経由で紐付く（FR-02）。
		{"走査ルート外の runner", rn("/data/x", ""), []Process{pr(1, ProcListener, "/data/x", zero)}, nil, ManagedStandalone, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runners := []Runner{tt.runner}
			orphans := attach(runners, tt.procs, tt.units)

			if got := runners[0].Managed; got != tt.managed {
				t.Errorf("Managed = %v, want %v", got, tt.managed)
			}
			if got := runners[0].Running(); got != tt.running {
				t.Errorf("Running() = %v, want %v", got, tt.running)
			}
			if got := unitNames(orphans); !slices.Equal(got, tt.orphans) {
				t.Errorf("孤児 = %q, want %q", got, tt.orphans)
			}
		})
	}
}

// .service ファイルによる照合を WorkingDirectory による照合が上書きしないこと。
func TestAttachPrefersUnitNameOverWorkingDir(t *testing.T) {
	runners := []Runner{rn(d1, u1)}
	// u1 は .service 一致、u2 は WorkingDirectory 一致。u1 が採用される。
	orphans := attach(runners, nil, []SvcState{sv(u1, "/opt/moved"), sv(u2, d1)})

	if runners[0].Svc == nil || runners[0].Svc.Unit != u1 {
		t.Errorf("Svc = %+v, want %q", runners[0].Svc, u1)
	}
	// u2 はディレクトリが見つかっているので孤児ではない。
	if len(orphans) != 0 {
		t.Errorf("孤児 = %q, want 0 件", unitNames(orphans))
	}
}

// 同じ runner を指すユニットが 2 件あっても 2 件目を孤児として誤報告しないこと。
func TestAttachDuplicateUnitIsNotOrphan(t *testing.T) {
	runners := []Runner{rn(d1, "")}
	orphans := attach(runners, nil, []SvcState{sv(u1, d1), sv(u2, d1)})

	if got := runners[0].Svc.Unit; got != u1 {
		t.Errorf("Svc.Unit = %q, want %q（先着）", got, u1)
	}
	if len(orphans) != 0 {
		t.Errorf("孤児 = %q, want 0 件", unitNames(orphans))
	}
}

// Worker の順序が /proc の読み取り順（辞書順なので "10" が "9" より先）に依存しないこと。
func TestAttachSortsWorkersByPID(t *testing.T) {
	runners := []Runner{rn(d1, "")}
	attach(runners, []Process{
		pr(10, ProcWorker, d1, zero), pr(100, ProcWorker, d1, zero), pr(9, ProcWorker, d1, zero),
	}, nil)

	var got []int
	for _, w := range runners[0].Workers {
		got = append(got, w.PID)
	}
	if want := []int{9, 10, 100}; !slices.Equal(got, want) || !runners[0].Busy() {
		t.Errorf("Workers の PID = %v（Busy = %v）, want %v（true）", got, runners[0].Busy(), want)
	}
}

// Listener が複数見えた場合は、プロセスの並び順に依らず起動時刻が新しい方を採用すること。
func TestAttachKeepsNewestListener(t *testing.T) {
	at := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	oldP, newP := pr(1, ProcListener, d1, at), pr(2, ProcListener, d1, at.Add(time.Hour))
	for _, procs := range [][]Process{{oldP, newP}, {newP, oldP}} {
		runners := []Runner{rn(d1, "")}
		attach(runners, procs, nil)
		if got := runners[0].Listener.PID; got != 2 {
			t.Errorf("Listener.PID = %d, want 2（新しい方）", got)
		}
	}
}

func TestAttachIgnoresUnmatchedProcesses(t *testing.T) {
	runners := []Runner{rn(d1, "")}
	attach(runners, []Process{
		pr(1, ProcListener, "", zero),           // Dir が取れなかったプロセス
		pr(2, ProcListener, "/opt/other", zero), // 対応する runner が無い
		pr(3, ProcKind(99), d1, zero),           // 範囲外の種別
	}, nil)

	if runners[0].Listener != nil || runners[0].Busy() || runners[0].Managed != ManagedUnknown {
		t.Errorf("紐付けてはいけないプロセスが付いた: %+v", runners[0])
	}
}

func TestAttachNoRunners(t *testing.T) {
	// Discover はユニット名でソートして渡すため、孤児もその順で出る。
	orphans := attach(nil, nil, []SvcState{sv(u1, "/opt/a"), sv(u2, "/opt/b")})
	if got, want := unitNames(orphans), []string{u1, u2}; !slices.Equal(got, want) {
		t.Errorf("孤児 = %q, want %q", got, want)
	}
}

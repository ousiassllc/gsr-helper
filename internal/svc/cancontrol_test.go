package svc

import (
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// allOps は判定表の対象となる全操作。1 つ足したら網羅の検査（TestCanControlCoversEveryOp）
// が落ちるので、期待値の書き漏らしがそのまま通ることはない。
func allOps() []Op { return []Op{OpStart, OpStop, OpKill, OpDrain, OpRestart, OpEnable} }

// opNames は失敗時に読める操作名。Op は表示用の名前を持たない（UI 側の識別子は
// ui/page/action.ID が持つ）ため、テストの読みやすさのためだけにここへ置く。
func opNames() map[Op]string {
	return map[Op]string{
		OpStart: "start", OpStop: "stop", OpKill: "kill",
		OpDrain: "drain", OpRestart: "restart", OpEnable: "enable",
	}
}

// fullCaps はすべての能力がある状態を返す。
func fullCaps() appconfig.Caps {
	return appconfig.Caps{
		Root: true, Systemd: true, Docker: true, Journal: true,
		GitHubToken: true, SudoUser: "ousiass",
	}
}

// systemdRunner は systemd 管理で稼働中の runner を返す。
func systemdRunner() runner.Runner {
	return runner.Runner{
		Dir:      "/opt/runners/build01-1",
		Config:   runner.Config{AgentName: "build01-1"},
		UnitName: "actions.runner.foo.build01-1.service",
		Managed:  runner.ManagedSystemd,
	}
}

// standaloneRunner は run.sh を直起動している runner を返す。
func standaloneRunner() runner.Runner {
	r := systemdRunner()
	r.UnitName = ""
	r.Managed = runner.ManagedStandalone
	return r
}

// unavailableRunner は systemd の管理状態が判定できない runner を返す。
func unavailableRunner() runner.Runner {
	r := systemdRunner()
	r.Managed = runner.ManagedUnavailable
	return r
}

// docs/ui/screens.md「無効な操作の表示」の各状況で、どの操作にどの理由が出るかを固定する。
//
// 全 6 操作 × 各状況を回し、blocked に挙げた操作だけがその理由で塞がれることを見る。
// 「塞がれる側」だけを見ると、塞ぐ範囲が広がった退行（ドレイン停止まで root を要求する
// など）を検出できないため、挙げていない操作が許可されることも同じループで確かめる。
func TestCanControlReasons(t *testing.T) {
	noRoot := fullCaps()
	noRoot.Root = false
	noSystemd := fullCaps()
	noSystemd.Systemd = false

	tests := map[string]struct {
		caps    appconfig.Caps
		runner  runner.Runner
		blocked []Op
		want    string
	}{
		"非 root": {
			caps: noRoot, runner: systemdRunner(),
			blocked: []Op{OpStart, OpStop, OpKill, OpRestart}, want: ReasonRoot,
		},
		"systemd が無い": {
			caps: noSystemd, runner: systemdRunner(),
			blocked: allOps(), want: ReasonSystemd,
		},
		"run.sh 直起動": {
			caps: fullCaps(), runner: standaloneRunner(),
			blocked: []Op{OpStart, OpStop, OpRestart}, want: ReasonStandalone,
		},
		"起動方式が判定不能": {
			caps: fullCaps(), runner: unavailableRunner(),
			blocked: []Op{OpStart, OpStop, OpRestart, OpEnable}, want: ReasonManagedUnknown,
		},
		"能力が揃っている": {
			caps: fullCaps(), runner: systemdRunner(),
			blocked: nil, want: "",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			for _, op := range allOps() {
				wantBlocked := isOp(op, tt.blocked...)
				ok, reason := CanControl(op, tt.runner, tt.caps)
				if ok == wantBlocked {
					t.Errorf("%s の可否 = %v, want %v", opNames()[op], ok, !wantBlocked)
				}
				if wantBlocked && reason != tt.want {
					t.Errorf("%s の理由 = %q, want %q", opNames()[op], reason, tt.want)
				}
				if !wantBlocked && reason != "" {
					t.Errorf("%s の理由 = %q, want 空", opNames()[op], reason)
				}
			}
		})
	}
}

// 判定は screens.md の表の順に行い、最初に一致した理由を返す。
//
// 能力の問題（root / systemd）を後段の理由で隠さないための順であり、隠すと
// 「systemd 管理外です」だけが出て sudo で起動し直せば使えるのかが読み取れない。
func TestCanControlPrecedence(t *testing.T) {
	noRoot := fullCaps()
	noRoot.Root = false
	noRootNoSystemd := noRoot
	noRootNoSystemd.Systemd = false
	noSystemd := fullCaps()
	noSystemd.Systemd = false

	tests := map[string]struct {
		op     Op
		caps   appconfig.Caps
		runner runner.Runner
		want   string
	}{
		"非 root が systemd 不在より優先":       {OpStop, noRootNoSystemd, systemdRunner(), ReasonRoot},
		"非 root が管理外より優先":               {OpStop, noRoot, standaloneRunner(), ReasonRoot},
		"systemd 不在が管理外より優先":            {OpStop, noSystemd, standaloneRunner(), ReasonSystemd},
		"systemd 不在が判定不能より優先":           {OpEnable, noSystemd, unavailableRunner(), ReasonSystemd},
		"root を要さないドレインは systemd 不在で塞ぐ": {OpDrain, noRootNoSystemd, systemdRunner(), ReasonSystemd},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ok, reason := CanControl(tt.op, tt.runner, tt.caps)
			if ok {
				t.Fatalf("%s が許可されている", opNames()[tt.op])
			}
			if reason != tt.want {
				t.Errorf("理由 = %q, want %q", reason, tt.want)
			}
		})
	}
}

// 強制停止とドレイン停止は、systemd の管理状態に依らず使える。
//
// worker のプロセスに直接作用する操作であり、ユニットの有無とは独立に効く
// （screens.md の 3 行目・4 行目はどちらも X と d を挙げていない）。ここを塞ぐと、
// run.sh 直起動や判定不能の runner を止める手段が UI から無くなる。
func TestCanControlAllowsKillAndDrainWithoutSystemdUnit(t *testing.T) {
	for _, r := range []runner.Runner{standaloneRunner(), unavailableRunner()} {
		for _, op := range []Op{OpKill, OpDrain} {
			if ok, reason := CanControl(op, r, fullCaps()); !ok {
				t.Errorf("%s（%s）が塞がれている: %q", opNames()[op], r.Managed, reason)
			}
		}
	}
}

// 判定表が全操作を見ていることを、操作の一覧の側から固定する。
//
// Op を足したときに「どの状況でも素通りする操作」が黙って増えないようにする。
// systemd が無ければサービス制御は 1 つも成立しないので、新しい操作も必ずここで塞がれる。
func TestCanControlCoversEveryOp(t *testing.T) {
	noSystemd := fullCaps()
	noSystemd.Systemd = false

	for _, op := range allOps() {
		if ok, _ := CanControl(op, systemdRunner(), fullCaps()); !ok {
			t.Errorf("%s が能力の揃ったホストで塞がれている", opNames()[op])
		}
		if ok, reason := CanControl(op, systemdRunner(), noSystemd); ok || reason != ReasonSystemd {
			t.Errorf("%s = %v/%q, want false/%q", opNames()[op], ok, reason, ReasonSystemd)
		}
	}
	if got := len(opNames()); got != len(allOps()) {
		t.Errorf("名前を持つ操作 = %d 件, want %d 件", got, len(allOps()))
	}
}

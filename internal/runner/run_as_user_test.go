package runner

import (
	"strconv"
	"testing"
)

func TestResolveRunAsUser(t *testing.T) {
	// UID → 名前を引く fake。未知の UID は引けなかったものとして扱う。
	names := map[int]string{0: "root", 1001: "runner"}
	lookup := func(uid int) string {
		if n, ok := names[uid]; ok {
			return n
		}
		return strconv.Itoa(uid)
	}
	lis := func(uid int) *Process { return &Process{PID: 1, Kind: ProcListener, UID: uid} }
	svc := func(load, user string) *SvcState { return &SvcState{Unit: u1, Load: load, User: user} }

	tests := []struct {
		name   string
		runner Runner
		want   string
	}{
		{"ユニットの User=", Runner{Svc: svc("loaded", "runner")}, "runner"},
		{"User= 未指定は root", Runner{Svc: svc("loaded", "")}, "root"},
		{"systemd を Listener より優先する", Runner{Svc: svc("loaded", "svcuser"), Listener: lis(1001)}, "svcuser"},
		// show に失敗したユニットは Unit だけのプレースホルダなので Load が空。
		{"show 失敗時は Listener にフォールバック", Runner{Svc: &SvcState{Unit: u1}, Listener: lis(1001)}, "runner"},
		{"not-found のユニットは Listener にフォールバック", Runner{Svc: svc("not-found", ""), Listener: lis(1001)}, "runner"},
		{"systemd 管理外は Listener の UID から引く", Runner{Listener: lis(1001)}, "runner"},
		{"UID 0 は root", Runner{Listener: lis(0)}, "root"},
		{"名前が引けなければ UID の 10 進表記", Runner{Listener: lis(4242)}, "4242"},
		{"UID が取れていない（-1）", Runner{Listener: lis(-1)}, ""},
		{"ユニットも Listener も無い", Runner{}, ""},
	}
	for _, tt := range tests {
		if got := resolveRunAsUser(tt.runner, lookup); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestNewUserLookup(t *testing.T) {
	lookup := newUserLookup()
	// root は必ず存在する。2 回目は memo から返るので同じ値になる。
	if got, again := lookup(0), lookup(0); got != "root" || again != "root" {
		t.Errorf("lookup(0) = (%q, %q), want root", got, again)
	}
	// 存在しない UID は 10 進表記にフォールバックする。
	if got := lookup(2147483000); got != "2147483000" {
		t.Errorf("未知の UID = %q, want 2147483000", got)
	}
}

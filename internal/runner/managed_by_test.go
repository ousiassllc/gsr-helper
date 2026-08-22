package runner

import "testing"

// FR-03 の判定表。systemd のユニット一覧が取れていない場合は「ユニットが無く」を
// 満たすことを確認できないため、run.sh 直起動とも未稼働とも判定しないこと。
func TestManagedBy(t *testing.T) {
	svc := &SvcState{Unit: u1, Load: "loaded", Active: "active"}
	lis := &Process{PID: 1, Kind: ProcListener}

	tests := []struct {
		name        string
		runner      Runner
		unitsListed bool
		want        ManagedBy
	}{
		{"ユニットあり + Listener", Runner{Svc: svc, Listener: lis}, true, ManagedSystemd},
		{"ユニットあり（停止中）", Runner{Svc: svc}, true, ManagedSystemd},
		{"ユニットなし + Listener", Runner{Listener: lis}, true, ManagedStandalone},
		{"どちらも無し", Runner{}, true, ManagedUnknown},
		// 一覧が取れていなくても、紐付いたユニットがあれば systemd 管理と分かる。
		{"一覧なし + ユニットあり", Runner{Svc: svc, Listener: lis}, false, ManagedSystemd},
		{"一覧なし + Listener", Runner{Listener: lis}, false, ManagedUnavailable},
		{"一覧なし + プロセスも無し", Runner{}, false, ManagedUnavailable},
	}
	for _, tt := range tests {
		if got := managedBy(tt.runner, tt.unitsListed); got != tt.want {
			t.Errorf("%s: managedBy = %v, want %v", tt.name, got, tt.want)
		}
	}
}

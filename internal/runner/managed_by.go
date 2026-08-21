package runner

// ManagedBy は runner の起動方式。
type ManagedBy int

// ManagedBy の取り得る値。
const (
	ManagedUnknown    ManagedBy = iota // 停止中でサービス登録もされていない
	ManagedSystemd                     // svc.sh install 済み
	ManagedStandalone                  // run.sh を直接起動している
)

// String はテーブル表示用の短い表記を返す。判定できていなければ "-"。
func (m ManagedBy) String() string {
	switch m {
	case ManagedSystemd:
		return "systemd"
	case ManagedStandalone:
		return "run.sh"
	default:
		return "-"
	}
}

// managedBy は起動方式を判定する。systemd ユニットが無いのにプロセスが
// 動いていれば run.sh 直起動とみなす。
func managedBy(r Runner) ManagedBy {
	switch {
	case r.Svc != nil:
		return ManagedSystemd
	case r.Listener != nil:
		return ManagedStandalone
	default:
		return ManagedUnknown
	}
}

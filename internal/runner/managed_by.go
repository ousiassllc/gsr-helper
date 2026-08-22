package runner

// ManagedBy は runner の起動方式。
type ManagedBy int

// ManagedBy の取り得る値。
const (
	ManagedUnknown    ManagedBy = iota // 停止中でサービス登録もされていない
	ManagedSystemd                     // svc.sh install 済み
	ManagedStandalone                  // run.sh を直接起動している
	// ManagedUnavailable は systemd の状態が判定できないことを表す。systemctl が
	// 無い、または systemctl list-units が失敗した場合で、「ユニットが無い」と
	// 区別できないため run.sh 直起動とも未稼働とも決めない。
	ManagedUnavailable
)

// String はテーブル表示用の短い表記を返す。
// systemd の状態が判定できていなければ "?"、未稼働なら "-"。
func (m ManagedBy) String() string {
	switch m {
	case ManagedSystemd:
		return "systemd"
	case ManagedStandalone:
		return "run.sh"
	case ManagedUnavailable:
		return "?"
	default:
		return "-"
	}
}

// managedBy は起動方式を判定する。systemd ユニットが無いのにプロセスが
// 動いていれば run.sh 直起動とみなす。
//
// unitsListed は systemd のユニット一覧が取れたかどうか。取れていない場合、
// ユニットが紐付いていないのは「登録されていない」からではなく「分からない」
// からである。FR-03 の条件「ユニットが無く」を満たすことを確認できないため、
// run.sh 直起動とも未稼働とも判定せず ManagedUnavailable にする。
// 稼働中の systemd 管理 runner を run.sh 直起動と表示すると、サービス制御が
// できないものとして扱われてしまう。
func managedBy(r Runner, unitsListed bool) ManagedBy {
	switch {
	case r.Svc != nil:
		return ManagedSystemd
	case !unitsListed:
		return ManagedUnavailable
	case r.Listener != nil:
		return ManagedStandalone
	default:
		return ManagedUnknown
	}
}

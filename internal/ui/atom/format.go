package atom

import (
	"fmt"
	"time"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// Duration は経過時間を一覧の桁に収まる表記で返す。
//
// 1 分未満は秒、1 時間未満は分秒、それ以上は時分にする。桁数を抑えるのは
// ELAPSED 列と JOB 列の幅を固定するためである。負の値は不正な計測結果として
// 記号のみを返す（未取得と同じ扱いにする）。
func Duration(d time.Duration) string {
	if d < 0 {
		return token.IconNoUnit
	}

	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

// VersionText は現行バージョンを、素の文字列と表示上の役割の組で返す。
// 最新と異なる場合は注意記号を添える。
//
// latest が空のときは注意記号を付けない。最新バージョンの取得は非同期であり、
// 未取得の状態を「古い」と誤って示さないためである。装飾済みの文字列を
// 返さない理由は StatusText と同じで、列幅に切り詰めてから装飾させるためである。
func VersionText(cur, latest string) (text string, role token.RoleToken) {
	if cur == "" {
		return token.IconNoUnit, token.RoleMuted
	}
	if latest == "" || latest == cur {
		return cur, token.RolePlain
	}
	return cur + " " + token.Icon(token.StateWarn), token.StateWarn.Role()
}

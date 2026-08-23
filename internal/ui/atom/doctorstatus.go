package atom

import (
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// DoctorStatus は診断結果の判定を、素の文字列と表示上の役割の組で返す。
//
// 装飾済みの文字列を返さないのは StatusText と同じ理由である（上位が「列幅に
// 切り詰めてから装飾する」順序を守れるようにする）。
//
// 記号と文字を必ず並べるのは、色を使えない端末でも OK / WARN / FAIL / SKIP を
// 判別できるようにするためである（設計原則 4）。とくに SKIP は「能力不足で
// 実行できなかった」を表し、FAIL と取り違えると対処のしようがない不備として
// 読まれる。
func DoctorStatus(st token.StateToken) (text string, role token.RoleToken) {
	icon := token.Icon(st)
	if icon == "" {
		// 未定義の判定。空文字を返すと列が詰まって桁がずれるので記号を置く。
		return token.IconUnknown, token.RoleMuted
	}
	return icon + " " + st.String(), st.Role()
}

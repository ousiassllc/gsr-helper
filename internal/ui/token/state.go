// Package token は画面の表示に使う色・記号・幅を定義する。
//
// 表示の最下層であり lipgloss 以外に依存しない。状態を表すトークンは
// 必ず色と記号の対で定義する（色だけで区別する状態を作らない。
// screens.md の設計原則 4）。
package token

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// StateToken は状態を表すトークンの識別子。
//
// 状態（OK / WARN / FAIL / SKIP）は必ず色と記号の対で定義する。グレーアウトや
// カーソルの強調は「状態」ではなく表示上の役割であり、記号の対を持たないため
// RoleToken に分けてある。両者を 1 つの型に混ぜると、記号を持たないトークンにも
// 対を要求することになり、Icon(RoleMuted) のような誤用を招く。
type StateToken int

// StateToken の取り得る値。
const (
	StateOK   StateToken = iota // 正常・成功
	StateWarn                   // 警告・閾値超過
	StateFail                   // 異常・失敗
	StateSkip                   // 実行不可・スキップ
)

// RoleToken は文字列に与える表示上の役割の識別子。
//
// 色だけを決め、記号は伴わない。状態を表すトークンは StateToken.Role で
// 対応する役割へ写す。
type RoleToken int

// RoleToken の取り得る値。
const (
	RolePlain  RoleToken = iota // 装飾しない
	RoleOK                      // 正常・成功
	RoleWarn                    // 警告・閾値超過
	RoleFail                    // 異常・失敗
	RoleSkip                    // 実行不可・スキップ
	RoleMuted                   // 補足情報・グレーアウト
	RoleAccent                  // カーソル位置・選択行
	RoleDanger                  // 破壊的操作の警告文
)

// palette は 1 トークンの明背景用・暗背景用の 2 値。
//
// 白背景の端末で補足情報が読めなくなることを防ぐため、色は必ず対で持ち、
// どちらを使うかは呼び出し側から渡される背景の明暗で決める。
type palette struct {
	light color.Color
	dark  color.Color
}

// roleColor は役割に対応する色の対を返す。定義が無ければ ok が false になる。
//
// map のパッケージ変数にしないのは、呼び出し側から書き換えられる状態を
// 作らないためである。RolePlain は装飾しない役割なので色を持たない。
func roleColor(t RoleToken) (p palette, ok bool) {
	switch t {
	case RoleOK:
		return palette{light: lipgloss.Color("#1f7a33"), dark: lipgloss.Color("#5fd75f")}, true
	case RoleWarn:
		return palette{light: lipgloss.Color("#8a6100"), dark: lipgloss.Color("#ffd75f")}, true
	case RoleFail:
		return palette{light: lipgloss.Color("#b3001b"), dark: lipgloss.Color("#ff6b6b")}, true
	case RoleSkip:
		return palette{light: lipgloss.Color("#5f5f87"), dark: lipgloss.Color("#8787af")}, true
	case RoleMuted:
		return palette{light: lipgloss.Color("#666666"), dark: lipgloss.Color("#9e9e9e")}, true
	case RoleAccent:
		return palette{light: lipgloss.Color("#0055cc"), dark: lipgloss.Color("#5fafff")}, true
	case RoleDanger:
		return palette{light: lipgloss.Color("#af0000"), dark: lipgloss.Color("#ff5f5f")}, true
	case RolePlain:
		return palette{}, false
	default:
		return palette{}, false
	}
}

// stateDisplay は状態に対応する記号と役割を返す。定義が無ければ ok が false になる。
//
// 記号と役割を 1 つの switch で返すのは、状態を足したときに記号か色の片方だけが
// 定義されることを構造で防ぐためである。記号は NO_COLOR や色を持たない端末でも
// 状態を判別できるようにするためにある。
func stateDisplay(t StateToken) (icon string, role RoleToken, ok bool) {
	switch t {
	case StateOK:
		return IconOK, RoleOK, true
	case StateWarn:
		return IconWarn, RoleWarn, true
	case StateFail:
		return IconFailed, RoleFail, true
	case StateSkip:
		return IconSkip, RoleSkip, true
	default:
		return "", RolePlain, false
	}
}

// Icon は状態に対応する記号を返す。色を使わない場合もこれは残る。
// 未定義の状態には空文字を返す。
func Icon(t StateToken) string {
	icon, _, _ := stateDisplay(t)
	return icon
}

// String は状態の表記（OK / WARN / FAIL / SKIP）を返す。未定義の状態は空文字。
//
// 表記を token に置くのは、記号（Icon）と色（Role）と同じく「表示の定義」だから
// である。上位で "OK" のような文字列を書き起こすと、記号と文字の対応が 2 箇所に
// 分かれ、状態を足したときに片方だけが増える。
func (t StateToken) String() string {
	switch t {
	case StateOK:
		return "OK"
	case StateWarn:
		return "WARN"
	case StateFail:
		return "FAIL"
	case StateSkip:
		return "SKIP"
	default:
		return ""
	}
}

// Role は状態に対応する表示上の役割を返す。未定義の状態は装飾しない。
func (t StateToken) Role() RoleToken {
	_, role, _ := stateDisplay(t)
	return role
}

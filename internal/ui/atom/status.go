package atom

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// labelIdle はジョブが無いことを表す文字列。
const labelIdle = "idle"

// StatusText は systemd ユニットの状態を、素の文字列と表示上の役割の組で返す。
//
// 装飾済みの文字列を返さないのは、上位の階層が「列幅に切り詰めてから装飾する」
// 順序を守れるようにするためである。装飾してから幅を揃えると、切り詰めで ANSI 列が
// 壊れるため Pad しか使えず、deactivating のような長い状態で列幅を超える
// （bubbles/table が超過分を切り落とし、ヘッダとの桁がずれる）。
//
// active / sub は systemctl の ActiveState / SubState をそのまま受け取る。
// 記号と文字を必ず並べて返すのは、色を使えない端末でも状態を判別できるようにするため。
func StatusText(active, sub string) (text string, role token.RoleToken) {
	switch {
	case active == "":
		// systemd ユニットが無い runner（run.sh 直起動など）。
		return token.IconNoUnit, token.RoleMuted
	case active == "failed" || sub == "failed":
		return token.Icon(token.StateFail) + " failed", token.StateFail.Role()
	case active == "active":
		// ● / ○ はサービスの稼働を表す記号であり、状態トークンの記号（✓ / ⊘）とは
		// 別に定めてある。色だけは状態トークンから引いて他の画面と揃える。
		return token.IconActive + " active", token.StateOK.Role()
	case active == "inactive":
		return token.IconInactive + " inactive", token.RoleMuted
	default:
		// activating / deactivating / reloading など、遷移中の状態。
		return token.Icon(token.StateWarn) + " " + active, token.StateWarn.Role()
	}
}

// StatusUnknown は systemd ユニットの状態を取得できなかったことを返す。
//
// 「ユニットが無い」（IconNoUnit）と同じ記号にしない。systemctl show が失敗した
// ユニットは値の無い SvcState として扱われる（internal/runner のプレースホルダ）が、
// それは**ユニットが存在しないことを意味しない**。記号を分けないと、仕様が書き分けて
// いる 2 つの状態が SVC 列で区別できず、「サービス登録されていない」と誤読される。
//
// 記号は runner.ManagedBy.String が判定不能に使う "?" と同じものにする（MANAGED 列と
// SVC 列で同じ意味の記号が違うと読み替えが必要になる）。
func StatusUnknown() (text string, role token.RoleToken) {
	return token.IconUnknown + " unknown", token.StateWarn.Role()
}

// JobText はジョブの実行状態と経過時間を、素の文字列と表示上の役割の組で返す。
// 装飾を返さない理由は StatusText と同じ。
func JobText(busy bool, d time.Duration) (text string, role token.RoleToken) {
	if !busy {
		return labelIdle, token.RoleMuted
	}
	return token.IconJob + " " + Duration(d), token.RoleAccent
}

// Cursor はカーソル位置の記号を返す。カーソル行でない場合も同じ幅の空白を返し、
// 行の桁がずれないようにする。
func Cursor(on bool, s token.Styles) string {
	if !on {
		return blank(token.IconCursor)
	}
	return s.Cursor.Render(token.IconCursor)
}

// CheckState は複数選択の表示状態。
type CheckState int

// CheckState の取り得る値。
const (
	CheckHidden CheckState = iota // 選択モードでないため表示しない
	CheckOff                      // 未選択
	CheckOn                       // 選択済み
)

// Checkbox は複数選択の状態を返す。非表示のときも同じ幅の空白を返す。
func Checkbox(st CheckState, s token.Styles) string {
	switch st {
	case CheckOn:
		return s.Selected.Render(token.IconChecked)
	case CheckOff:
		return token.IconUnchecked
	case CheckHidden:
		return blank(token.IconUnchecked)
	default:
		return blank(token.IconUnchecked)
	}
}

// WarnMark は行に注意事項があることを示す記号を返す。無い場合は同じ幅の空白を返す。
//
// 記号は 1 セル固定なので、装飾済みの文字列を返しても列幅を超えない。
func WarnMark(on bool, s token.Styles) string {
	if !on {
		return blank(token.IconWarn)
	}
	return s.Warn.Render(token.IconWarn)
}

// blank は記号と同じ表示幅の空白を返す。記号の有無で桁がずれないようにする。
func blank(icon string) string {
	return strings.Repeat(" ", lipgloss.Width(icon))
}

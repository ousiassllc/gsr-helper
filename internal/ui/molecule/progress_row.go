package molecule

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// progressNameWidth は対象名に配る桁。
//
// screens.md の Setup タブのモック（`✓ build01-5   登録・サービス起動 完了`）に合わせ、
// 名前の後ろが空白 3 つになる幅を取る。名前を装飾前に空白 1 つ足してから埋めるので、
// 桁を超える名前でも説明との間に必ず 1 つは空白が残る。
//
// 名前は中略しない。一括追加の対象は連番で、どの台の話なのかが読めなくなると
// 逐次表示の意味が無くなる。溢れる分は行末（説明）が削れる。
const progressNameWidth = 12

// ProgressState は進捗 1 行の状態。
type ProgressState int

// ProgressState の取り得る値。
const (
	ProgressWaiting ProgressState = iota // 未着手
	ProgressRunning                      // 実行中
	ProgressDone                         // 完了
	ProgressFailed                       // 失敗
)

// ProgressView は進捗 1 行の表示用の構造体。
//
// 何件目かや全体の件数は持たない。分母の表示とバーは行の外側（pane.ProgressList）の
// 責務であり、行に持たせると同じ数字が行ごとに繰り返される。
type ProgressView struct {
	Name   string        // 対象の名前（runner 名など）
	State  ProgressState // 進み具合
	Detail string        // 説明（「config.sh 実行中…」「失敗（終了コード 1）」）
}

// ProgressRow は進捗 1 行を返す。
//
// 記号・名前・説明を並べ、幅に収まらない分は末尾を中略する。**記号は状態ごとに
// 必ず置き（未着手は同じ幅の空白）**、色を使えない端末でも進み具合を読み分けられる
// ようにする（screens.md の設計原則 4）。
//
// **行全体を素の文字列で組んでから 1 度に装飾する**（ActionRow の無効な行と同じ）。
// 記号・名前・説明はどれも同じ状態を指しており 1 つの装飾で足りるうえ、装飾前なら
// 幅に収める中略を atom に任せられる。装飾済みの文字列を切り詰めると ANSI 列が壊れる
// （atom.Truncate の契約）。
func ProgressRow(v ProgressView, width int, s token.Styles) string {
	icon, role := progressDisplay(v.State)

	// 名前が空でも桁は空けたままにする。行ごとに説明の開始位置がずれると、
	// 縦に並べたときにどれが同じ列なのか読めなくなる。
	row := icon + " " + atom.Pad(v.Name+" ", progressNameWidth, atom.Left) + v.Detail
	// 説明が無い行の末尾に埋めた空白が残らないようにする。
	row = strings.TrimRight(row, " ")

	return s.Style(role).Render(atom.Truncate(row, width))
}

// progressDisplay は状態に対応する記号と表示上の役割を返す。
//
// 記号と役割を 1 つの switch で返すのは token.stateDisplay と同じ理由で、状態を
// 足したときに記号か色の片方だけが定義されることを構造で防ぐためである。
//
// 完了と失敗は状態トークン（token.StateOK / StateFail）から引き、実行中は
// atom.JobText と同じ ▶ と Accent にする。実行中は「正常か異常か」ではなく
// 「今そこに居る」ことを示す表示であり、状態トークンに対応する記号を持たない。
func progressDisplay(st ProgressState) (icon string, role token.RoleToken) {
	switch st {
	case ProgressDone:
		return token.Icon(token.StateOK), token.StateOK.Role()
	case ProgressFailed:
		return token.Icon(token.StateFail), token.StateFail.Role()
	case ProgressRunning:
		return token.IconJob, token.RoleAccent
	case ProgressWaiting:
		return progressBlank(), token.RoleMuted
	default:
		return progressBlank(), token.RoleMuted
	}
}

// progressBlank は記号と同じ表示幅の空白を返す。記号の有無で桁がずれないようにする。
func progressBlank() string {
	return strings.Repeat(" ", lipgloss.Width(token.IconOK))
}

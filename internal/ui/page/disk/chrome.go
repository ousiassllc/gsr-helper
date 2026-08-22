package disk

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// 本体以外の描画（要約行の材料・状態行・入力中・フッタ）と、本体の高さの取り分を
// 集める。disk.go から分けたのは 1 ファイル 300 行の上限に収めるためであり、
// 責務の境界ではない（runners.go は 1 ファイルに収まっている）。

const (
	// inputFilter は入力中であることを状態行に出すときの名称（screens.md の入力中）。
	inputFilter = "絞り込み"
	// emptyScanning は集計中でまだ 1 件も判明していないときの表示。
	emptyScanning = "集計中です…"
	// emptyNone は集計が終わって対象が 1 件も無かったときの表示。
	//
	// 集計中と書き分けるのは、待てば埋まるのかどうかを読み分けられるようにする
	// ためである（listrow が SIZE 列で「集計中…」と「失敗」を書き分けるのと同じ理由）。
	emptyNone = "クリーンアップ対象はありません"
	// summaryLines は本体の先頭に置くファイルシステム要約が占める行数（要約 + 空行）。
	summaryLines = 2
)

// tableHeight は一覧に渡す高さを返す。要約行と空行のぶんを本体領域から引く。
//
// 引かないと表の最終行が枠の外へ押し出される。1 行未満にはしない（bubbles/table は
// 高さ 0 で見出しすら描けない）。
func tableHeight(bodyH int) int {
	return max(bodyH-summaryLines, 1)
}

// summaryView はファイルシステム要約の表示用の構造体を返す。
//
// 取得に失敗した場合はゼロ値を渡したうえで **Unavailable を立てる。** 立てないと
// 使用率が「0%」として描かれ、枯渇しているのに潤沢に見える（FSSummaryView の doc）。
// パスと容量の併記はゼロ値のまま molecule 側が「値なし」「空」に落とす。
//
// Warn は常に偽である。閾値（appconfig.DiskThresholds）を page へ運ぶ経路が
// page.StateMsg にまだ無いためで、判定を持てるようになった時点でここだけを直す
// （molecule.FSSummaryLine の doc）。
func (m Model) summaryView() molecule.FSSummaryView {
	s := m.stats
	failed := m.statsErr != nil
	if failed {
		s = disk.Stats{}
	}
	return molecule.FSSummaryView{
		Path:         s.Path,
		UsedPercent:  s.UsedPercent(),
		UsedBytes:    s.UsedBytes,
		TotalBytes:   s.TotalBytes,
		InodePercent: s.InodePercent(),
		Unavailable:  failed,
		Warn:         false,
	}
}

// emptyMessage は行が 1 件も無いときの文言を返す。
func (m Model) emptyMessage() string {
	if m.scan != nil {
		return emptyScanning
	}
	return emptyNone
}

// chrome は親へ本体以外の状態を知らせる Cmd を返す。
func (m Model) chrome() tea.Cmd {
	c := page.ChromeMsg{
		Tab:    m.tab,
		Modal:  m.overlay.Active(),
		Input:  m.input(),
		Status: m.status(),
		Footer: m.footer(),
	}
	return func() tea.Msg { return c }
}

// input は入力中の名称を返す。入力中でなければ空文字を返す。
func (m Model) input() string {
	if m.tbl.Filtering() {
		return inputFilter
	}
	return ""
}

// status は状態行に出す page 側の文を返す。
//
// 優先順は「入力中 > クリーンアップ中 > 結果報告 > 選択件数」である。進行中の破壊的
// 操作を選択件数で隠さないことと、その報告を次の打鍵まで読めることを優先する。
func (m Model) status() string {
	switch {
	case m.input() != "":
		return "入力中: " + m.input()
	case m.clean != nil:
		return "クリーンアップ中 (" + strconv.Itoa(m.clean.done) + "/" +
			strconv.Itoa(m.clean.total) + ")"
	case m.notice != "":
		return m.notice
	}

	checked := m.tbl.Checked()
	if len(checked) == 0 {
		return ""
	}
	return "選択: " + strconv.Itoa(len(checked)) + " 件（合計 " + atom.Bytes(checkedBytes(checked)) + "）"
}

// checkedBytes は選択された対象の容量の合計を返す。
//
// 集計に失敗した対象（Bytes が確定していない）は 0 として足す。負の値を混ぜると
// 合計が実際より小さくなり、選択を増やしたのに合計が減る表示になる。
func checkedBytes(rows []row) int64 {
	var total int64
	for _, r := range rows {
		if r.usage.Bytes > 0 {
			total += r.usage.Bytes
		}
	}
	return total
}

// footer はフッタのキーヒントを返す。
//
// キー表記は page.BindingKey、説明文は keymap の Help().Desc から取る。フッタとヘルプで
// 文言を食い違わせないためである（runners.go の listHints の doc）。
//
// c は選択が 0 件のとき無効として出す。押しても何も起きないキーを黙って並べない
// （screens.md の無効な操作の表示）。**押せない理由と、押したときに状態行へ出る案内
// （noticeNoTarget）は同じ文言である。**
//
// r の説明は screens.md の Disk タブのキーマップでは「再集計」だが、ここでは
// keymap の「再読み込み」をそのまま出す。**r は自分の再集計と親の再検出の両方を
// 起こすキーであり**（handleKey）、ヘルプ（? の全キー一覧）は Global のキーとして
// 「再読み込み」と出す。フッタだけを書き換えると、同じキーの説明が 2 通りになる。
func (m Model) footer() []atom.Hint {
	if m.overlay.Active() {
		return m.overlay.Hints()
	}

	l, g, d := m.st.Keys.List, m.st.Keys.Global, m.st.Keys.Disk
	reason := ""
	if len(m.tbl.Checked()) == 0 {
		reason = noticeNoTarget
	}
	return []atom.Hint{
		{Key: page.BindingKey(l.Toggle), Desc: l.Toggle.Help().Desc, Enabled: true, Reason: ""},
		{
			Key:     page.BindingKey(d.Clean),
			Desc:    d.Clean.Help().Desc,
			Enabled: reason == "",
			Reason:  reason,
		},
		{Key: page.BindingKey(g.Refresh), Desc: g.Refresh.Help().Desc, Enabled: true, Reason: ""},
		{Key: page.BindingKey(l.Filter), Desc: l.Filter.Help().Desc, Enabled: true, Reason: ""},
	}
}

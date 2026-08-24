// Package itemview は Config タブの設定一覧の 1 行を組み立てる。
//
// page/config 本体から分けているのは 2 つの理由による。
//   - ここにあるのは編集対象の要約（edit.Summary）を表示用の値へ落とす**純粋関数**
//     であり、tea.Model を組み立てずに検証できる。
//   - 1 ディレクトリ 2000 行の上限に対して page/config に余裕が無い（Issue #141 で
//     警告帯に入った）。上限値を緩めるのではなく分けた（Issue #147）。
//
// 先例は page/runners/rowview と page/disk/cleanview（どちらもドメインの値を
// 表示用の値へ落とす純粋関数である）。
package itemview

import (
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule/listrow"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// Item は一覧の 1 行。
type Item struct {
	// Kind は選んだときに何を編集するか。
	Kind edit.Kind
	// View は表示に落とした値。行の描画はこれだけを見る。
	View listrow.SettingView
	// Enabled は選べるか。再登録が要る項目だけが偽になる。
	Enabled bool
}

// 区画の添字。table.Model へ SetItems するときに使う。
const (
	SecMain int = iota
	SecReregister
	SecCopy
	SecCount
)

// Columns は一覧の列を返す。幅 80 に収まるよう定める（行頭 2 + 列幅 70 + 列間 4 = 76）。
//
// 項目名は切り詰めない幅を確保し、現在値と備考を詰める。値は長さがまちまち
// （PATH は 1 行で数十文字になる）で、切り詰めても「どの項目か」は失われないが、
// 項目名が切れると何を選ぶ行なのか読めなくなるためである。
func Columns() []token.Column {
	return []token.Column{
		{ID: token.ColName, Title: "項目", Width: 26, Right: false},
		{ID: listrow.ColValue, Title: "現在値", Width: 22, Right: false},
		{ID: token.ColNote, Title: "備考", Width: 22, Right: false},
	}
}

// MainItems は編集できる 5 項目を返す。
//
// drop-in は systemd ユニットが無ければ選べない。置く先が決まらず BuildDropIn が
// ErrNoUnit を返すだけなので、他の行と同じく**選ぶ前に理由を備考へ出す**
// （screens.md の無効な操作の表示）。
func MainItems(s edit.Summary) []Item {
	return []Item{
		newItem(edit.KindEnv, ".env", envValue(s.EnvCount), "環境変数・プロキシ", false, s.Editable),
		newItem(edit.KindPath, ".path", s.Path, "", false, s.Editable),
		newItem(edit.KindDropIn, "systemd drop-in", s.DropIn, dropInNote(s.HasUnit),
			false, s.Editable && s.HasUnit),
		newItem(edit.KindLabels, "ラベル", "", "即時反映（GitHub）", false, s.Editable),
		newItem(edit.KindGroup, "runner group", "", "即時反映（GitHub）", false, s.Editable),
	}
}

// dropInNote は drop-in の行の備考を返す。
func dropInNote(hasUnit bool) string {
	if !hasUnit {
		return "systemd ユニット無し"
	}
	return "daemon-reload が要る"
}

// ReregisterItems は再登録が要る項目を返す。選べない行として出す。
//
// 行を消さずに残すのは、「ここでは変えられない」ことと「なぜ変えられないか」を
// 同じ場所で示すためである（screens.md の無効な操作の表示と同じ考え方）。
func ReregisterItems() []Item {
	return []Item{
		newItem(edit.KindReregister, "名前・work dir・ephemeral", "", "再登録が必要", true, false),
	}
}

// CopyItems は複製の行を返す（FR-40）。
func CopyItems(editable bool) []Item {
	return []Item{
		newItem(edit.KindCopy, "他の runner へ複製", ".env をまとめて適用", "", false, editable),
	}
}

// newItem は 1 行を組み立てる。
func newItem(k edit.Kind, name, value, note string, warn, enabled bool) Item {
	return Item{
		Kind:    k,
		View:    listrow.SettingView{Item: name, Value: value, Note: note, Warn: warn},
		Enabled: enabled,
	}
}

// envValue は .env の現在値の要約を返す。
func envValue(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n) + " 項目"
}

// Matches は絞り込みの一致判定。項目名と現在値のどちらかに含まれれば残す。
func Matches(it Item, q string) bool {
	return containsFold(it.View.Item, q) || containsFold(it.View.Value, q)
}

// DisabledReason は行を選択できない理由を返す。
//
// 理由は行の備考欄（SettingView.Note）が既に持っているため、ここでは空文字を
// 返して二重に出さない。返すのは可否だけである。
func DisabledReason(it Item) (string, bool) { return "", !it.Enabled }

// containsFold は大文字小文字を無視して含むかを返す。
func containsFold(s, q string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(q))
}

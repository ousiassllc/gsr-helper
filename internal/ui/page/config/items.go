package config

import (
	"strconv"

	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule/listrow"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// item は一覧の 1 行。
type item struct {
	kind edit.Kind
	// view は表示に落とした値。行の描画はこれだけを見る。
	view listrow.SettingView
	// enabled は選べるか。再登録が要る項目だけが偽になる。
	enabled bool
}

// 区画の添字。table.Model へ SetItems するときに使う。
const (
	secMain int = iota
	secReregister
	secCopy
	secCount
)

// 一覧の列。幅 80 に収まるよう定める（行頭 2 + 列幅 70 + 列間 4 = 76）。
//
// 項目名は切り詰めない幅を確保し、現在値と備考を詰める。値は長さがまちまち
// （PATH は 1 行で数十文字になる）で、切り詰めても「どの項目か」は失われないが、
// 項目名が切れると何を選ぶ行なのか読めなくなるためである。
func settingColumns() []token.Column {
	return []token.Column{
		{ID: token.ColName, Title: "項目", Width: 26, Right: false},
		{ID: listrow.ColValue, Title: "現在値", Width: 22, Right: false},
		{ID: token.ColNote, Title: "備考", Width: 22, Right: false},
	}
}

// mainItems は編集できる 5 項目を返す。
//
// drop-in は systemd ユニットが無ければ選べない。置く先が決まらず BuildDropIn が
// ErrNoUnit を返すだけなので、他の行と同じく**選ぶ前に理由を備考へ出す**
// （screens.md の無効な操作の表示）。
func mainItems(s edit.Summary) []item {
	return []item{
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

// reregisterItems は再登録が要る項目を返す。選べない行として出す。
//
// 行を消さずに残すのは、「ここでは変えられない」ことと「なぜ変えられないか」を
// 同じ場所で示すためである（screens.md の無効な操作の表示と同じ考え方）。
func reregisterItems() []item {
	return []item{
		newItem(edit.KindReregister, "名前・work dir・ephemeral", "", "再登録が必要", true, false),
	}
}

// copyItems は複製の行を返す（FR-40）。
func copyItems(editable bool) []item {
	return []item{
		newItem(edit.KindCopy, "他の runner へ複製", ".env をまとめて適用", "", false, editable),
	}
}

// newItem は 1 行を組み立てる。
func newItem(k edit.Kind, name, value, note string, warn, enabled bool) item {
	return item{
		kind:    k,
		view:    listrow.SettingView{Item: name, Value: value, Note: note, Warn: warn},
		enabled: enabled,
	}
}

// envValue は .env の現在値の要約を返す。
func envValue(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n) + " 項目"
}

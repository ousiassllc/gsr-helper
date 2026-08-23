package config

import (
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/runner"
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

// summary は対象 runner の現在値をまとめたもの。
//
// 画面の組み立てに要る値だけを持ち、ドメインの型を一覧へ持ち込まない。
// 読み取りに失敗した項目は空文字にして「値なし」として描く（一覧が出せなく
// なるより、その項目だけ値が読めていないと分かる方がよい）。
//
// **ラベルと runner group の現在値は持たない。** どちらも GitHub 側の値であり、
// 一覧を組み直すたびに API を呼ぶことになる。3 秒ポーリングでは API を呼ばない
// 方針（docs/api/external-interfaces.md）に従い、項目を選んだ時点で取りに行く。
type summary struct {
	envCount int
	path     string
	dropIn   string
	editable bool
}

// mainItems は編集できる 5 項目を返す。
func mainItems(s summary) []item {
	return []item{
		newItem(edit.KindEnv, ".env", envValue(s.envCount), "環境変数・プロキシ", false, s.editable),
		newItem(edit.KindPath, ".path", s.path, "", false, s.editable),
		newItem(edit.KindDropIn, "systemd drop-in", s.dropIn, "daemon-reload が要る", false, s.editable),
		newItem(edit.KindLabels, "ラベル", "", "即時反映（GitHub）", false, s.editable),
		newItem(edit.KindGroup, "runner group", "", "即時反映（GitHub）", false, s.editable),
	}
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

// summarize は runner の現在値を読み取って要約する。
//
// 読み取りはファイル 3 つと検出済みの値だけで、GitHub API は呼ばない（3 秒
// ポーリングで API を呼ばない方針に沿う。ラベルは検出結果が持っている）。
func summarize(r runner.Runner, ld edit.Loader) summary {
	s := summary{envCount: 0, path: "", dropIn: "", editable: true}
	if r.Dir == "" {
		s.editable = false
		return s
	}

	if env, err := ld.Env(r); err == nil {
		s.envCount = len(env.Keys())
	}
	if p, err := ld.PathFile(r); err == nil {
		s.path = p.Value
	}
	s.dropIn = dropInValue(r, ld)

	return s
}

// dropInValue は drop-in の現在値の要約を返す。
func dropInValue(r runner.Runner, ld edit.Loader) string {
	if r.UnitName == "" {
		return ""
	}

	d, err := ld.DropIn(r)
	if err != nil || len(d.Directives) == 0 {
		return ""
	}

	parts := make([]string, 0, len(d.Directives))
	for _, dir := range d.Directives {
		parts = append(parts, dir.Key+"="+dir.Value)
	}
	return strings.Join(parts, ", ")
}

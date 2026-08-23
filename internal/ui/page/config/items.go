package config

import (
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule/listrow"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// kind は編集できる設定項目の種類。並びは画面仕様の Config タブのモックに従う。
type kind int

const (
	// kindEnv は .env（環境変数・プロキシ・job hooks）。
	kindEnv kind = iota
	// kindPath は .path。
	kindPath
	// kindDropIn は systemd drop-in。
	kindDropIn
	// kindLabels はラベル（GitHub API で即時反映）。
	kindLabels
	// kindGroup は runner group（GitHub API で即時反映）。
	kindGroup
	// kindReregister は名前 / work dir / ephemeral。再登録が要るため編集できない。
	kindReregister
	// kindCopy は .env を他の runner へ複製する（FR-40）。
	kindCopy
	// kindSelf は本ツール自身の設定（FR-41〜FR-42）。一覧には出さず、
	// 対象の選択から直接開く。
	kindSelf
)

// item は一覧の 1 行。
type item struct {
	kind kind
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

// 一覧の列。幅 80 に収まるよう定める（行頭 6 + 列幅 70 + 列間 4 = 80）。
func settingColumns() []token.Column {
	return []token.Column{
		{ID: token.ColName, Title: "項目", Width: 26, Right: false},
		{ID: listrow.ColValue, Title: "現在値", Width: 30, Right: false},
		{ID: token.ColNote, Title: "備考", Width: 14, Right: false},
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
	envKeys  int
	path     string
	dropIn   string
	editable bool
}

// mainItems は編集できる 5 項目を返す。
func mainItems(s summary) []item {
	return []item{
		newItem(kindEnv, ".env", envValue(s.envKeys), "環境変数・プロキシ・job hooks", false, s.editable),
		newItem(kindPath, ".path", s.path, "", false, s.editable),
		newItem(kindDropIn, "systemd drop-in", s.dropIn, "再起動に daemon-reload", false, s.editable),
		newItem(kindLabels, "ラベル", "", "選ぶと取得（即時反映）", false, s.editable),
		newItem(kindGroup, "runner group", "", "選ぶと取得（即時反映）", false, s.editable),
	}
}

// reregisterItems は再登録が要る項目を返す。選べない行として出す。
//
// 行を消さずに残すのは、「ここでは変えられない」ことと「なぜ変えられないか」を
// 同じ場所で示すためである（screens.md の無効な操作の表示と同じ考え方）。
func reregisterItems() []item {
	return []item{
		newItem(kindReregister, "名前 / work dir / ephemeral", "", "変更には再登録が必要", true, false),
	}
}

// copyItems は複製の行を返す（FR-40）。
func copyItems(editable bool) []item {
	return []item{
		newItem(kindCopy, "この設定を他の runner に複製", ".env をまとめて適用", "", false, editable),
	}
}

// newItem は 1 行を組み立てる。
func newItem(k kind, name, value, note string, warn, enabled bool) item {
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
func summarize(r runner.Runner, ld loader) summary {
	s := summary{envKeys: 0, path: "", dropIn: "", editable: true}
	if r.Dir == "" {
		s.editable = false
		return s
	}

	if env, err := ld.env(r); err == nil {
		s.envKeys = len(env.Keys())
	}
	if p, err := ld.pathFile(r); err == nil {
		s.path = p.Value
	}
	s.dropIn = dropInValue(r, ld)

	return s
}

// dropInValue は drop-in の現在値の要約を返す。
func dropInValue(r runner.Runner, ld loader) string {
	if r.UnitName == "" {
		return ""
	}

	d, err := ld.dropIn(r)
	if err != nil || len(d.Directives) == 0 {
		return ""
	}

	parts := make([]string, 0, len(d.Directives))
	for _, dir := range d.Directives {
		parts = append(parts, dir.Key+"="+dir.Value)
	}
	return strings.Join(parts, ", ")
}

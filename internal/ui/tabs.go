package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/jobs"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runners"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// tab はタブ 1 枚のメタ情報。タブの追加は tabSpecs への 1 要素追加だけで済む。
//
// 親 Model は []tab を走査するだけで個別のタブを知らない。共有状態は 1 本の Msg で
// 全 page に配られ、モーダルと入力中の有無も page が Msg で報告するため、親は
// タブの内部状態を持たない（atomic-design.md の「タブを 1 つ追加するときに触る箇所」）。
//
// Key と page が持つタブ番号はどちらも []tab の添字から機械的に決まる（newTabs）。
// タブ番号を書き写す場所を残さないのは、ずれても例外もログも出ないためである
// （ずれると app.go の ChromeMsg / TabMsg の突き合わせが恒久的に外れる）。
type tab struct {
	Key     string    // "1".."7"（添字 + 1）
	Title   string    // "Runners"
	Model   tea.Model // page。未実装のタブは nil
	Enabled bool
	Reason  string // 無効の理由（TabBar のグレーアウトに使う）
}

// tabSpec はタブ 1 枚の定義。
//
// **番号とタブ番号は持たない。** どちらも並び順から決まる値であり、ここに書けば
// 「スライスの添字」「Key の数字」「page へ渡すタブ番号」の 3 系統を人手で
// そろえ続けることになる。newTabs が添字 1 つから全部を導く。
type tabSpec struct {
	// Title はタブ行に出す名前。
	Title string
	// New は page を組み立てる。tab には newTabs が []tab の添字を渡す。
	// nil はこの版で未実装のタブを表し、Model を持たない無効なタブになる。
	New func(tab int, st page.StateMsg) tea.Model
}

// tabSpecs は表示するタブの並びを返す。タブを増やすときに触るのはこの関数だけ。
//
// タブ行に 7 枚すべてを出すのは、押しても何も起きないキーを作らないためである
// （screens.md の共通レイアウトは 7 タブを常に出す）。3〜7 は未実装なので New を
// 持たず、無効なタブになる。無効なタブはグレーアウトし、番号キーを押したら理由を
// 状態行に出す。無効なタブへはキーも StateMsg も配らない（app.go の live）。
// 後続 Issue は該当する 1 行に New を足すだけで有効化できる。
func tabSpecs() []tabSpec {
	return []tabSpec{
		{Title: "Runners", New: func(i int, st page.StateMsg) tea.Model { return runners.New(i, st) }},
		{Title: "Jobs", New: func(i int, st page.StateMsg) tea.Model { return jobs.New(i, st) }},
		{Title: "Disk", New: nil},
		{Title: "Logs", New: nil},
		{Title: "Doctor", New: nil},
		{Title: "Config", New: nil},
		{Title: "Setup", New: nil},
	}
}

// newTabs は表示するタブを組み立てる。
//
// 引数から作る page.StateMsg を初期のスナップショットとして各 page に渡す。最初の
// リサイズと検出が届く前でも、page が配色とキー定義を持った状態で描画できるように
// するためである。
//
// 番号キー（Key）と page へ渡すタブ番号は、どちらも添字 i から機械的に決める。
// tabSpecs の並びを変えても番号は自動で追随し、ずれようがない。
//
// 未実装のタブの理由には page.ReasonUnsupported を使う。未対応の操作（page.Allow）と
// 同じ文言になるのは、利用者にとって「この版ではまだ使えない」という同じ意味だから
// である。
//
// Enabled / Reason は能力（Caps）でタブを無効化する枠も兼ねる。実装済みの 2 枚は
// いずれも能力を必要としない（runner の一覧とジョブの一覧はホスト内の読み取りだけで
// 成立し、systemd が無くても run.sh 直起動の runner を表示できる）ため、常に有効で
// ある。能力を必要とするタブ（追加・削除を行う Setup など）を足す Issue が、Caps を
// 見て Enabled と Reason を決める判断を tabSpec に足す。
func newTabs(caps appconfig.Caps, ex exec.Executor, keys keymap.Set, s token.Styles, dark bool) []tab {
	init := page.StateMsg{
		Result: runner.Result{},
		Caps:   caps,
		Styles: s,
		Keys:   keys,
		Exec:   ex,
		Dark:   dark,
		BodyW:  0,
		BodyH:  0,
		Err:    nil,
	}

	specs := tabSpecs()
	tabs := make([]tab, 0, len(specs))
	for i, sp := range specs {
		t := tab{
			Key:     strconv.Itoa(i + 1),
			Title:   sp.Title,
			Model:   nil,
			Enabled: false,
			Reason:  page.ReasonUnsupported,
		}
		if sp.New != nil {
			t.Model = sp.New(i, init)
			t.Enabled = true
			t.Reason = ""
		}
		tabs = append(tabs, t)
	}
	return tabs
}

// notice は無効なタブの番号キーを押したときに状態行へ出す文を返す。
func (t tab) notice() string {
	return "[" + t.Key + "]" + t.Title + " " + t.Reason
}

// Package tabset はタブ 1 枚ぶんのメタ情報と、表示するタブの並びを組み立てる。
//
// **個別のタブ（page/<tab>）を import する唯一の場所である。** 親 Model（ui）は
// []Tab を走査するだけで、どのタブが何をするかを知らない。この分担をパッケージ境界で
// 強制するために ui 直下から分けている。タブを 1 枚足す Issue が触るのはこのパッケージ
// であって親 Model ではない、という約束が import の向きから読み取れる
// （atomic-design.md の「タブを 1 つ追加するときに触る箇所」）。
//
// 分けたもう 1 つの理由は行数である。1 ディレクトリ 2000 行（テスト込み）の上限に対し、
// タブが増えるたびに ui 直下が太る形だと、親 Model 自身のテストを足す余地が先に尽きる
// （Issue #35）。
package tabset

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/disk"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/jobs"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runners"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// Tab はタブ 1 枚のメタ情報。タブの追加は specs への 1 要素追加だけで済む。
//
// 共有状態は 1 本の Msg で全 page に配られ、モーダルと入力中の有無も page が Msg で
// 報告するため、親はタブの内部状態を持たない。
//
// Key と page が持つタブ番号はどちらも []Tab の添字から機械的に決まる（New）。
// タブ番号を書き写す場所を残さないのは、ずれても例外もログも出ないためである
// （ずれると親の ChromeMsg / TabMsg の突き合わせが恒久的に外れる。Issue #39）。
type Tab struct {
	Key     string    // "1".."7"（添字 + 1）
	Title   string    // "Runners"
	Model   tea.Model // page。未実装のタブは nil
	Enabled bool
	Reason  string // 無効の理由（TabBar のグレーアウトに使う）
}

// Notice は無効なタブの番号キーを押したときに状態行へ出す文を返す。
func (t Tab) Notice() string {
	return "[" + t.Key + "]" + t.Title + " " + t.Reason
}

// spec はタブ 1 枚の定義。
//
// **番号とタブ番号は持たない。** どちらも並び順から決まる値であり、ここに書けば
// 「スライスの添字」「Key の数字」「page へ渡すタブ番号」の 3 系統を人手で
// そろえ続けることになる。New が添字 1 つから全部を導く。
type spec struct {
	// Title はタブ行に出す名前。
	Title string
	// New は page を組み立てる。tab には New が []Tab の添字を渡す。
	// nil はこの版で未実装のタブを表し、Model を持たない無効なタブになる。
	New func(tab int, st page.StateMsg) tea.Model
}

// specs は表示するタブの並びを返す。タブを増やすときに触るのはこの関数だけ。
//
// タブ行に 7 枚すべてを出すのは、押しても何も起きないキーを作らないためである
// （screens.md の共通レイアウトは 7 タブを常に出す）。3〜7 は未実装なので New を
// 持たず、無効なタブになる。無効なタブはグレーアウトし、番号キーを押したら理由を
// 状態行に出す。無効なタブへはキーも StateMsg も配らない（ui の live）。
// 後続 Issue は該当する 1 行に New を足すだけで有効化できる。
func specs() []spec {
	return []spec{
		{Title: "Runners", New: func(i int, st page.StateMsg) tea.Model { return runners.New(i, st) }},
		{Title: "Jobs", New: func(i int, st page.StateMsg) tea.Model { return jobs.New(i, st) }},
		{Title: "Disk", New: func(i int, st page.StateMsg) tea.Model { return disk.New(i, st) }},
		{Title: "Logs", New: nil},
		{Title: "Doctor", New: nil},
		{Title: "Config", New: nil},
		{Title: "Setup", New: nil},
	}
}

// New は表示するタブを組み立てる。
//
// 引数から作る page.StateMsg を初期のスナップショットとして各 page に渡す。最初の
// リサイズと検出が届く前でも、page が配色とキー定義を持った状態で描画できるように
// するためである。
//
// 番号キー（Key）と page へ渡すタブ番号は、どちらも添字 i から機械的に決める。
// specs の並びを変えても番号は自動で追随し、ずれようがない。
//
// 未実装のタブの理由には page.ReasonUnsupported を使う。未対応の操作（page.Allow）と
// 同じ文言になるのは、利用者にとって「この版ではまだ使えない」という同じ意味だから
// である。
//
// Enabled / Reason は能力（Caps）でタブを無効化する枠も兼ねる。実装済みの 2 枚は
// いずれも能力を必要としない（runner の一覧とジョブの一覧はホスト内の読み取りだけで
// 成立し、systemd が無くても run.sh 直起動の runner を表示できる）ため、常に有効で
// ある。能力を必要とするタブ（追加・削除を行う Setup など）を足す Issue が、Caps を
// 見て Enabled と Reason を決める判断を spec に足す。
func New(caps appconfig.Caps, ex exec.Executor, keys keymap.Set, s token.Styles, dark bool) []Tab {
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

	defs := specs()
	tabs := make([]Tab, 0, len(defs))
	for i, sp := range defs {
		t := Tab{
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

package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/jobs"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runners"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// tab はタブ 1 枚のメタ情報。タブの追加はこのスライスへの 1 要素追加だけで済む。
//
// 親 Model は []tab を走査するだけで個別のタブを知らない。共有状態は 1 本の Msg で
// 全 page に配られ、モーダルと入力中の有無も page が Msg で報告するため、親は
// タブの内部状態を持たない（atomic-design.md の「タブを 1 つ追加するときに触る箇所」）。
type tab struct {
	Key     string    // "1".."7"
	Title   string    // "Runners"
	Model   tea.Model // page
	Enabled bool
	Reason  string // 無効の理由（TabBar のグレーアウトに使う）
}

// newTabs は表示するタブを組み立てる。タブを増やすときに触るのはこの関数だけ。
//
// 引数から作る page.StateMsg を初期のスナップショットとして各 page に渡す。最初の
// リサイズと検出が届く前でも、page が配色とキー定義を持った状態で描画できるように
// するためである。
//
// タブ行に 7 枚すべてを出すのは、押しても何も起きないキーを作らないためである
// （screens.md の共通レイアウトは 7 タブを常に出す）。3〜7 は未実装なので Model を
// 持たず、Enabled を false にして理由を添える。無効なタブはグレーアウトし、番号キーを
// 押したらこの理由を状態行に出す。無効なタブへはキーも StateMsg も配らない
// （app.go の live）。後続 Issue は該当する 1 行を実装済みのタブに差し替えるだけで
// 有効化できる。
//
// 理由には page.ReasonUnsupported を使う。未対応の操作（page.Allow）と同じ文言に
// なるのは、利用者にとって「この版ではまだ使えない」という同じ意味だからである。
//
// Enabled / Reason は能力（Caps）でタブを無効化する枠も兼ねる。実装済みの 2 枚は
// いずれも能力を必要としない（runner の一覧とジョブの一覧はホスト内の読み取りだけで
// 成立し、systemd が無くても run.sh 直起動の runner を表示できる）ため、常に有効で
// ある。能力を必要とするタブ（追加・削除を行う Setup など）を足す Issue が、その
// タブの行で Caps を見て Enabled と Reason を決める。
func newTabs(caps appconfig.Caps, keys keymap.Set, s token.Styles, dark bool) []tab {
	init := page.StateMsg{
		Result: runner.Result{},
		Caps:   caps,
		Styles: s,
		Keys:   keys,
		Dark:   dark,
		BodyW:  0,
		BodyH:  0,
		Err:    nil,
	}
	return []tab{
		{Key: "1", Title: "Runners", Model: runners.New(0, init), Enabled: true, Reason: ""},
		{Key: "2", Title: "Jobs", Model: jobs.New(1, init), Enabled: true, Reason: ""},
		{Key: "3", Title: "Disk", Model: nil, Enabled: false, Reason: page.ReasonUnsupported},
		{Key: "4", Title: "Logs", Model: nil, Enabled: false, Reason: page.ReasonUnsupported},
		{Key: "5", Title: "Doctor", Model: nil, Enabled: false, Reason: page.ReasonUnsupported},
		{Key: "6", Title: "Config", Model: nil, Enabled: false, Reason: page.ReasonUnsupported},
		{Key: "7", Title: "Setup", Model: nil, Enabled: false, Reason: page.ReasonUnsupported},
	}
}

// notice は無効なタブの番号キーを押したときに状態行へ出す文を返す。
func (t tab) notice() string {
	return "[" + t.Key + "]" + t.Title + " " + t.Reason
}

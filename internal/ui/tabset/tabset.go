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
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule/chromebar"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/config"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/disk"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/doctor"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/jobs"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/logs"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/runners"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/setup"
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
// （screens.md の共通レイアウトは 7 タブを常に出す）。7 枚すべてが実装済みに
// なったので、この版で無効になるタブは無い。無効なタブはグレーアウトし、
// 番号キーを押したら理由を
// 状態行に出す。無効なタブへはキーも StateMsg も配らない（ui の live）。
// 後続 Issue は該当する 1 行に New を足すだけで有効化できる。
func specs() []spec {
	return []spec{
		{Title: "Runners", New: func(i int, st page.StateMsg) tea.Model { return runners.New(i, st) }},
		{Title: "Jobs", New: func(i int, st page.StateMsg) tea.Model { return jobs.New(i, st) }},
		{Title: "Disk", New: func(i int, st page.StateMsg) tea.Model { return disk.New(i, st) }},
		{Title: page.TabLogs, New: func(i int, st page.StateMsg) tea.Model { return logs.New(i, st) }},
		{Title: page.TabDoctor, New: func(i int, st page.StateMsg) tea.Model { return doctor.New(i, st) }},
		{Title: page.TabConfig, New: func(i int, st page.StateMsg) tea.Model { return config.New(i, st) }},
		{Title: page.TabSetup, New: func(i int, st page.StateMsg) tea.Model { return setup.New(i, st) }},
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
// Enabled / Reason は能力（Caps）でタブを無効化する枠も兼ねる。実装済みの 5 枚は
// いずれもタブ自体は能力を必要としない（runner の一覧とジョブの一覧はホスト内の
// 読み取りだけで成立し、systemd が無くても run.sh 直起動の runner を表示できる。
// Disk と Logs も、docker や journal が無ければタブの中で該当する行や操作だけを
// 縮退させる）ため、常に有効である。
//
// **Setup も同じで、タブ自体は塞がない。** 追加・削除・更新は root と GitHub の
// 認証を要するが、screens.md「無効な操作の表示」はこれを**操作単位**で規定して
// いる。タブごと塞ぐと、何が足りないのかを操作の理由として出せなくなる
// （判定は ui/page/action の Allow が持つ）。
func New(caps appconfig.Caps, ex exec.Executor, keys keymap.Set, s token.Styles, dark bool) []Tab {
	init := page.StateMsg{
		Result: runner.Result{},
		Caps:   caps,
		Styles: s,
		Keys:   keys,
		Deps:   page.Deps{Exec: ex},
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

// KeyOf は名前の一致する有効なタブの番号キーを返す。無ければ空文字。
//
// 番号は並び順から決まる（New）ので、親 Model が「Doctor タブは 5」と書き写すと
// 並びを変えたときに案内だけが別のタブを指す。**タブの番号を知っているのは
// このパッケージだけ**という分担をここで保つ。
//
// 用途は起動時の前提チェック（FR-44）が状態行に出す誘導
// （`⚠ ホスト前提 2 件（5 で詳細）`）である。無効なタブには誘導しない
// （押しても開けないタブを案内することになる）。
func KeyOf(tabs []Tab, title string) string {
	for _, t := range tabs {
		if t.Title == title && t.Enabled {
			return t.Key
		}
	}
	return ""
}

// IndexOfKey は番号キーに一致するタブの添字を返す。無ければ ok は偽。
//
// 番号キーと []Tab の添字の対応を知っているのはこのパッケージだけという分担を
// KeyOf と同じ理由で保つ（親 Model に添字の算出を書き写させない）。
func IndexOfKey(tabs []Tab, key string) (int, bool) {
	for i := range tabs {
		if tabs[i].Key == key {
			return i, true
		}
	}
	return 0, false
}

// IndexOfTitle は名前に一致するタブの添字を返す。無ければ ok は偽。
//
// タブをまたぐ移動（page.OpenTabMsg）が名前で行き先を指すため、名前から添字を
// 引く場所をここへ 1 箇所に集める。
func IndexOfTitle(tabs []Tab, title string) (int, bool) {
	for i := range tabs {
		if tabs[i].Title == title {
			return i, true
		}
	}
	return 0, false
}

// Next は step の向きへ次に有効なタブの添字を返す。無効なタブは飛ばし、端では
// 折り返す。有効なタブが 1 枚も無ければ ok は偽。
func Next(tabs []Tab, active, step int) (int, bool) {
	n := len(tabs)
	if n == 0 {
		return 0, false
	}
	for i := 1; i <= n; i++ {
		target := ((active+step*i)%n + n) % n
		if tabs[target].Enabled {
			return target, true
		}
	}
	return 0, false
}

// Live はタブが Msg を受け取れるか（有効で Model を持つか）を返す。
func Live(tabs []Tab, i int) bool {
	return i >= 0 && i < len(tabs) && tabs[i].Enabled && tabs[i].Model != nil
}

// Deliver は Msg を指定したタブへ配る。無効なタブや Model を持たないタブへは
// 配らない（Live）。配る先の Model が無く、捨てても失われるのは自分で始めた
// 処理の結果だけである。
//
// **tabs のスライスは実体を共有する前提で書き換える。** 呼び出し側の親 Model
// （ui.App）の複製がタブのスライスの実体を共有するのと同じ理由で、ここで
// tabs[i].Model を書き換えれば呼び出し側にもそのまま反映される
// （ui.App の doc「コピーはタブのスライスの実体を共有する」）。
func Deliver(tabs []Tab, i int, msg tea.Msg) tea.Cmd {
	if !Live(tabs, i) {
		return nil
	}
	var cmd tea.Cmd
	tabs[i].Model, cmd = tabs[i].Model.Update(msg)
	return cmd
}

// ActivateOnce は起動時に選択されているタブへ page.ActivateMsg を 1 度だけ配る
// （Issue #63）。done が真なら何もしない。配れたときだけ done を真にする。
//
// **配る場所が親の Init ではないのは、Init が Model を書き換えられない（Cmd だけを
// 返す）ためである。** 配ったことを覚えられないと、共有状態が配られるたび
// （端末サイズ・背景色・3 秒ごとの再検出）に同じタブへ前面化が届き、
// page.ActivateMsg で購読を張る page が周期ごとに 1 本ずつ購読を増やす。
//
// タブの切り替えに伴う前面化（ui の activate）と役割を分けてあるのは、切り替えでは
// 離れるタブへの page.DeactivateMsg と対になる必要があるのに対し、起動時の 1 度目には
// 対になる相手が居ないためである。往復して戻ってきたときの前面化は切り替えの側が配る。
//
// **配れないうちは覚えない。** 有効な page が入るのを待って次の機会に配る。
func ActivateOnce(tabs []Tab, active int, done *bool) tea.Cmd {
	if *done || !Live(tabs, active) {
		return nil
	}
	*done = true
	return Deliver(tabs, active, page.ActivateMsg{})
}

// Distribute は共有状態を有効な全タブへ配る。
//
// 選択中のタブだけでなく有効な全タブへ配るのは、タブを切り替えた瞬間に古いサイズや
// 古い検出結果で描かれることを防ぐためである。
func Distribute(tabs []Tab, st page.StateMsg) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(tabs))
	for i := range tabs {
		if !Live(tabs, i) {
			continue
		}
		cmds = append(cmds, Deliver(tabs, i, st))
	}
	return tea.Batch(cmds...)
}

// Broadcast は有効な全タブへ Msg を配り、返った Cmd をまとめる。
//
// 選択中のタブだけに配らないのは、裏のタブも自分で始めた処理を持つためである
// （裏に回ったときに畳み損ねた処理をここで確実に閉じられる）。
func Broadcast(tabs []Tab, msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(tabs))
	for i := range tabs {
		if cmd := Deliver(tabs, i, msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// Views はタブのメタ情報を表示用の値へ落とす。
//
// 選択中かどうかは添字と active の比較でここで解決し、chrome へは真偽値だけを
// 渡す（chrome が tabset を import しないための境界）。
func Views(tabs []Tab, active int) []chromebar.TabView {
	views := make([]chromebar.TabView, 0, len(tabs))
	for i := range tabs {
		views = append(views, chromebar.TabView{
			Key:     tabs[i].Key,
			Title:   tabs[i].Title,
			Active:  i == active,
			Enabled: tabs[i].Enabled,
		})
	}
	return views
}

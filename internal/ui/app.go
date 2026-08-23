// Package ui は bubbletea の親 Model を提供する。
//
// 親 Model（App）は Atomic Design の階層の外に置き、検出結果・Caps・端末サイズ・
// 背景の明暗・現在のタブ・キーの配送を受け持つ。page を切り替える唯一の主体であり、
// 個別のタブの中身は知らない（atomic-design.md の適用方針）。
//
// 内部は token / keymap / atom / molecule / organism / template / page に階層化し、
// 依存の方向をパッケージの import で強制する。
package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/chrome"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/tabset"
	"github.com/ousiassllc/gsr-helper/internal/ui/template"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// Options は cmd から受け取る起動時の決定事項。
type Options struct {
	// Color は色を使うか。NO_COLOR / --no-color / 非 TTY を cmd が 1 つの値に
	// まとめたものであり、UI 側で環境を読み直さない。
	Color bool
	// Refresh は一覧の自動更新間隔。0 以下なら設定ファイルの値を使う。
	Refresh time.Duration
	// Roots は追加の走査ルート（設定ファイルと --root を cmd が合わせたもの）。
	Roots []string
	// Host はヘッダに出すホスト名。既定の runner 名の接頭辞にも使う（FR-11）。
	Host string
	// Secrets は短命トークンの預け先。Setup タブが取得したトークンを載せる。
	Secrets *gh.Secrets
}

// App は親 Model。
//
// **コピーはタブのスライスの実体を共有する。** tabs は写しても同じ配列を指すため、
// 直前の Update が返した 1 つの値だけを持つこと（organism.Table と同じ約束）。
type App struct {
	cfg  appconfig.Config
	caps appconfig.Caps
	ex   exec.Executor
	opts Options

	keys   keymap.Set
	styles token.Styles
	dark   bool
	width  int
	height int

	tabs   []tabset.Tab
	active int
	chrome page.ChromeMsg
	// notice は親が状態行に出す一時的な案内（無効なタブの理由）。次の打鍵で消える。
	notice string

	result runner.Result
	err    error

	// inflight は実行中の検出の本数。0 でない間は新しい検出を始めない（onTick）。
	inflight int
	// seq は発行した検出の通し番号、applied は取り込んだ結果の番号。
	// 古い周期の結果で新しい一覧を上書きしないために持つ（discoveredMsg.seq）。
	seq     int
	applied int
}

// tea.Model を実装していることをコンパイル時に確かめる。
var _ tea.Model = App{}

// New は親 Model を組み立てる。
//
// 背景の明暗の初期値は暗背景（dark = true）とする。端末へ問い合わせた応答が
// 得られない環境ではこの値のままになるため、暗背景の端末が多数である前提で
// 安全側に倒している（atomic-design.md の背景の明暗と NO_COLOR）。
func New(cfg appconfig.Config, caps appconfig.Caps, ex exec.Executor, o Options) App {
	const dark = true

	keys := keymap.New()
	styles := token.NewStyles(dark, o.Color)
	return App{
		cfg:      cfg,
		caps:     caps,
		ex:       ex,
		opts:     o,
		keys:     keys,
		styles:   styles,
		dark:     dark,
		width:    0,
		height:   0,
		tabs:     tabset.New(caps, ex, keys, styles, dark),
		active:   0,
		chrome:   page.ChromeMsg{Tab: 0, Modal: false, Input: "", Status: "", Footer: nil},
		notice:   "",
		result:   runner.Result{},
		err:      nil,
		inflight: 0,
		seq:      0,
		applied:  0,
	}
}

// Init は背景色の問い合わせと最初の自動更新の周期を発行する。
//
// 検出をここで直に始めず即時の tickMsg に任せるのは、Init が Model を書き換えられない
// （Cmd だけを返す）ためである。ここで発行すると「実行中の検出」を親が数えられず、
// 二重起動を防ぐ判定（onTick）が起動直後だけ狂う。検出の入口を tickMsg の分岐 1 つに
// 揃えることで、実行中の本数と通し番号が必ず親の状態に載る。
//
// tea.RequestBackgroundColor は Cmd ではなく Msg を返す関数なので、**呼ばずに**
// 関数値のまま渡す（呼ぶと Msg になり Cmd として渡せない）。tea.Cmd は
// func() tea.Msg なので、この関数値がそのまま Cmd になる。
func (a App) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
		firstTick(),
	)
}

// Update は Msg を種類ごとに振り分ける。
//
// ドメイン層をこの中で直接呼ばない。重い処理はすべて tea.Cmd として UI の外で
// 実行し、結果を Msg で受け取る（architecture/overview.md の非同期処理モデル）。
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return a.handleKey(msg)
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		cmd := a.distribute()
		return a, cmd
	case tea.BackgroundColorMsg:
		a.dark = msg.IsDark()
		a.styles = token.NewStyles(a.dark, a.opts.Color)
		cmd := a.distribute()
		return a, cmd
	case tickMsg:
		cmd := a.onTick()
		return a, cmd
	case discoveredMsg:
		cmd := a.applyDiscovered(msg)
		return a, cmd
	case page.ChromeMsg:
		if msg.Tab == a.active {
			a.chrome = msg
		}
		return a, nil
	case page.GlobalKeyMsg:
		// page が解釈しなかったキーだけがここへ戻る（page.GlobalKeyMsg の doc）。
		return a.handleGlobalKey(msg.Press)
	case page.OpenTabMsg:
		// タブをまたぐ移動は親が担う。タブ同士は互いを知らないため、移動元は
		// 移動先の番号も型も持てない（page.OpenTabMsg の doc）。
		return a.openTab(msg)
	case page.TabMsg:
		// ドメイン層の呼び出し結果は発行元のタブへ戻す（page.TabMsg の doc）。
		return a.forwardTo(msg.Tab, msg.Msg)
	default:
		a, cmd := a.forward(msg)
		return a, cmd
	}
}

// View は共通レイアウトを組み立てて返す。
//
// 代替スクリーンは tea.View の AltScreen で宣言する（v2 に tea.WithAltScreen は無い）。
// 全画面を占有して終了時に元の端末内容へ戻すため、7 タブを切り替える UI でも
// スクロールバックを汚さない。
func (a App) View() tea.View {
	body := ""
	if t, ok := a.current(); ok && t.Model != nil {
		// page は tea.Model なので tea.View を返す。枠に流し込むのは中身の文字列だけ。
		body = t.Model.View().Content
	}

	cv := a.chromeView()
	v := tea.NewView(template.Frame(template.FrameInput{
		Header: chrome.Header(cv),
		Tabs:   chrome.TabBar(cv),
		Body:   body,
		Status: chrome.Status(cv),
		Footer: chrome.Footer(cv),
		Width:  a.width,
		Height: a.height,
	}))
	v.AltScreen = true
	return v
}

// state は page へ配る共有状態のスナップショットを返す。
//
// 端末サイズは template.BodySize で本体領域に換算してから渡す。サイズの真実を
// 1 箇所に集めるため、page より下は自分で領域を計算しない。
func (a App) state() page.StateMsg {
	w, h := template.BodySize(a.width, a.height)
	return page.StateMsg{
		Result: a.result,
		Caps:   a.caps,
		Styles: a.styles,
		Keys:   a.keys,
		Exec:   a.ex,
		Dark:   a.dark,
		Color:  a.opts.Color,
		BodyW:  w,
		BodyH:  h,
		Err:    a.err,
		Setup: page.SetupDeps{
			Host:     a.opts.Host,
			Defaults: a.cfg.Defaults,
			Secrets:  a.opts.Secrets,
		},
	}
}

// distribute は共有状態を有効な page へ配る。
//
// 選択中のタブだけでなく有効な全タブへ配るのは、タブを切り替えた瞬間に古いサイズや
// 古い検出結果で描かれることを防ぐためである。各 page は ChromeMsg を返すが、
// 採用するのは選択中のタブのものだけ（Update の page.ChromeMsg の分岐）。
//
// 無効なタブ（この版で未実装のタブ）には配らない。Model を持たないので配る先が無く、
// 後続 Issue が tabs.go の 1 行を差し替えれば自動的に配られるようになる。
func (a *App) distribute() tea.Cmd {
	st := a.state()
	cmds := make([]tea.Cmd, 0, len(a.tabs))
	for i := range a.tabs {
		if !a.live(i) {
			continue
		}
		var cmd tea.Cmd
		a.tabs[i].Model, cmd = a.tabs[i].Model.Update(st)
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

// live はタブが Msg を受け取れるか（有効で Model を持つか）を返す。
func (a App) live(i int) bool {
	return i >= 0 && i < len(a.tabs) && a.tabs[i].Enabled && a.tabs[i].Model != nil
}

// forward は Msg を有効タブへ転送する。
//
// 戻りを tea.Model ではなく App にするのは、親 Model が値として流れる形を崩さない
// ためである（一部の経路だけがポインタを返すと、どちらが最新の状態か追えなくなる）。
func (a App) forward(msg tea.Msg) (App, tea.Cmd) {
	return a.forwardTo(a.active, msg)
}

// forwardTo は Msg を指定したタブへ転送する。
//
// 選択中でないタブへも配るのは、page が発行した Cmd の結果を発行元へ戻すためである
// （page.TabMsg の doc）。無効になったタブ宛の結果は捨てる。配る先の Model が無く、
// 捨てても失われるのは自分で始めた処理の結果だけである。
func (a App) forwardTo(i int, msg tea.Msg) (App, tea.Cmd) {
	if !a.live(i) {
		return a, nil
	}

	var cmd tea.Cmd
	a.tabs[i].Model, cmd = a.tabs[i].Model.Update(msg)
	return a, cmd
}

// current は有効タブを返す。
func (a App) current() (tabset.Tab, bool) {
	if a.active < 0 || a.active >= len(a.tabs) {
		return tabset.Tab{}, false
	}
	return a.tabs[a.active], true
}

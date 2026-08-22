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
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
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
	// Host はヘッダに出すホスト名。
	Host string
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

	tabs   []tab
	active int
	chrome page.ChromeMsg
	// notice は親が状態行に出す一時的な案内（無効なタブの理由）。次の打鍵で消える。
	notice string

	result runner.Result
	err    error
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
		cfg:    cfg,
		caps:   caps,
		ex:     ex,
		opts:   o,
		keys:   keys,
		styles: styles,
		dark:   dark,
		width:  0,
		height: 0,
		tabs:   newTabs(caps, keys, styles, dark),
		active: 0,
		chrome: page.ChromeMsg{Tab: 0, Modal: false, Input: "", Status: "", Footer: nil},
		notice: "",
		result: runner.Result{},
		err:    nil,
	}
}

// Init は背景色の問い合わせ・初回検出・自動更新の Tick を発行する。
//
// tea.RequestBackgroundColor は Cmd ではなく Msg を返す関数なので、**呼ばずに**
// 関数値のまま渡す（呼ぶと Msg になり Cmd として渡せない）。tea.Cmd は
// func() tea.Msg なので、この関数値がそのまま Cmd になる。
func (a App) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
		a.discover(),
		a.tick(),
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
		return a, tea.Batch(a.discover(), a.tick())
	case discoveredMsg:
		// 期限切れ・失敗した周期の部分結果では上書きしない。runner.ScanUnits は
		// ctx がキャンセルされた時点で残りの systemctl show を発行せず取れた分だけを
		// 返すため、部分結果を採ると systemd 管理の runner が run.sh / - と誤表示され、
		// 孤児ユニットも過少報告される。エラーは状態行の警告として出し、一覧は
		// 直前の成功結果を保つ。
		a.err = msg.err
		if msg.err == nil {
			a.result = msg.result
		}
		cmd := a.distribute()
		return a, cmd
	case page.ChromeMsg:
		if msg.Tab == a.active {
			a.chrome = msg
		}
		return a, nil
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

	v := tea.NewView(template.Frame(template.FrameInput{
		Header: a.header(),
		Tabs:   a.tabBar(),
		Body:   body,
		Status: a.status(),
		Footer: a.footer(),
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
		Dark:   a.dark,
		BodyW:  w,
		BodyH:  h,
		Err:    a.err,
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
	if !a.live(a.active) {
		return a, nil
	}

	var cmd tea.Cmd
	a.tabs[a.active].Model, cmd = a.tabs[a.active].Model.Update(msg)
	return a, cmd
}

// current は有効タブを返す。
func (a App) current() (tab, bool) {
	if a.active < 0 || a.active >= len(a.tabs) {
		return tab{}, false
	}
	return a.tabs[a.active], true
}

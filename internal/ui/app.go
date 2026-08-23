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
	"github.com/ousiassllc/gsr-helper/internal/audit"
	"github.com/ousiassllc/gsr-helper/internal/doctor"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/chrome"
	"github.com/ousiassllc/gsr-helper/internal/ui/discovery"
	"github.com/ousiassllc/gsr-helper/internal/ui/ghscope"
	"github.com/ousiassllc/gsr-helper/internal/ui/hostreq"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/tabset"
	"github.com/ousiassllc/gsr-helper/internal/ui/template"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
	"github.com/ousiassllc/gsr-helper/internal/ui/workscan"
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
	// ConfigPath は設定ファイルの配置先（決定は appconfig/confpath の責務）。
	ConfigPath string
	// FirstRun は設定ファイルが無い状態で起動したか（FR-41）。判定は cmd が
	// appconfig.Exists で行う（Load はファイルが無くても既定値を返すため）。
	FirstRun bool
	// Audit は破壊的操作の記録先。外部コマンドを伴わない削除を記録するために
	// page 階層まで配る（page.StateMsg.Audit）。**開けなかった場合も nil に
	// せず audit.Discard() を渡す**——記録先の nil 判定を page ごとに書かせない
	// ためで、縮退（記録せずに続行）は Discard 自身が担う。
	Audit *audit.Logger
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
	// activated は起動時の前面化を配ったか（tabset.ActivateOnce）。
	activated bool
	chrome    page.ChromeMsg
	// notice は親が状態行に出す一時的な案内（無効なタブの理由）。次の打鍵で消える。
	notice string

	result runner.Result
	err    error

	// inflight は実行中の検出の本数。0 でない間は新しい検出を始めない（onTick）。
	inflight int
	// seq は発行した検出の通し番号、applied は取り込んだ結果の番号。
	// 古い周期の結果で新しい一覧を上書きしないために持つ（discovery.Msg.Seq）。
	seq     int
	applied int

	// hostReq は起動時のジョブ実行の前提チェック（FR-44）で見つかった不備の件数。
	// hostReqDone は 1 度発行したか（hostreq.go）。
	hostReq     int
	hostReqDone bool
	// hostChecks は起動時に走らせる診断項目。空なら走らせない。**テストの
	// 差し替え口でもある**（本物は実ホストの sudo / docker / /etc/group を読む）。
	hostChecks []doctor.Check

	// work は runner ごとの _work 使用量とその集計の進行状況（Issue #73）。
	// scopes はトークンの保有スコープと取得の進行状況（Issue #79）。
	// どちらも駆動の契機だけを親が決め、周期の管理はサブパッケージが持つ
	// （background.go）。
	work   workscan.State
	scopes ghscope.State
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
		tabs:   tabset.New(caps, ex, keys, styles, dark),
		chrome: pageChrome(0),
		// inflight/seq/applied/hostReq/hostReqDone はゼロ値のままでよい（起動直後）。
		hostChecks: doctor.Startup(doctor.Default()),
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
	return tea.Batch(tea.RequestBackgroundColor, firstTick())
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
	case discovery.Msg:
		cmd := a.applyDiscovered(msg)
		return a, cmd
	case hostreq.Msg:
		a.hostReq = msg.Bad
		return a, nil
	case workscan.Msg:
		// _work 使用量が確定した。共有状態として全タブへ配り直す（Issue #73）。
		a.work.Apply(msg)
		cmd := a.distribute()
		return a, cmd
	case ghscope.Msg:
		// 保有スコープが確定した。操作の可否の判定に効く（Issue #79）。
		a.scopes.Apply(msg)
		cmd := a.distribute()
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
		return a, tabset.Deliver(a.tabs, msg.Tab, msg.Msg)
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
		Audit:  a.opts.Audit,
		Disk: page.DiskState{
			Thresholds: a.cfg.DiskThresholds,
			Work:       a.work.Usage(),
		},
		Scopes: a.scopes.Scopes(),
		Setup: page.SetupDeps{
			Host:     a.opts.Host,
			Defaults: a.cfg.Defaults,
			Secrets:  a.opts.Secrets,
			// 本番は差し替えない。nil のまま渡すと job が gh.Token から借りた
			// トークンで本物のクライアントを作る（page.SetupDeps.NewClient）。
			NewClient: nil,
			Fetch:     nil,
		},
		Config: page.ConfigDeps{
			Conf:     a.cfg,
			Path:     a.opts.ConfigPath,
			FirstRun: a.opts.FirstRun,
		},
	}
}

// distribute は共有状態を有効な page へ配る。各 page は ChromeMsg を返すが、採用
// するのは選択中のタブのものだけ（Update の page.ChromeMsg の分岐）。配り方そのもの
// （無効なタブを飛ばす・全有効タブへ配る理由）は tabset.Distribute の doc を参照。
func (a *App) distribute() tea.Cmd {
	// 前面化を共有状態より先に配る（activate と同じ順序。張り直す page が最新の
	// スナップショットを持った状態で張れるようにするため）。
	on := tabset.ActivateOnce(a.tabs, a.active, &a.activated)
	return tea.Batch(on, tabset.Distribute(a.tabs, a.state()))
}

// forward は Msg を選択中のタブへ転送する。
//
// 戻りを tea.Model ではなく App にするのは、親 Model が値として流れる形を崩さない
// ためである（一部の経路だけがポインタを返すと、どちらが最新の状態か追えなくなる）。転送
// そのもの（無効なタブへは配らない）は tabset.Deliver の doc を参照。
func (a App) forward(msg tea.Msg) (App, tea.Cmd) {
	return a, tabset.Deliver(a.tabs, a.active, msg)
}

// current は有効タブを返す。
func (a App) current() (tabset.Tab, bool) {
	if a.active < 0 || a.active >= len(a.tabs) {
		return tabset.Tab{}, false
	}
	return a.tabs[a.active], true
}

// Package runnerdetail は runner 1 台の詳細画面（モーダル）を提供する。
//
// Runners タブと Jobs タブが共用する（FR-46 / FR-47。Jobs タブの操作対象も runner で
// あり、同じ部品をそのまま開ける）。page 本体から分けているのは、共通の土台
// （StateMsg / ChromeMsg / モーダルの重なり / 操作の可否）に runner 固有の画面を
// 混ぜないためである。Disk / Logs / Doctor / Config / Setup も page を import するので、
// 混ぜると runner の詳細画面まで引きずることになる。
//
// 画面はこのパッケージの New を page.Overlay.Register に渡し、Open で開く。重なりの
// 規則（キーは最上位だけ・esc は 1 枚）は page.Overlay が担保する。
package runnerdetail

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/pane"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/action"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

const (
	// labelWidth は情報部のラベル列の幅。最も長いラベル（ディレクトリ）が
	// 収まる幅にして、値の開始桁を揃える。
	labelWidth = 14
	// headingRows は操作リストの上に置く行数（空行 1 + 見出し 1）。
	headingRows = 2
	// heading は操作リストの見出し（screens.md の詳細画面）。
	heading = "操作"
	// managedUnknownText は起動方式が判定できていない（未稼働の）ときの表記。
	managedUnknownText = "未稼働（サービス登録なし・プロセスなし）"
	// managedUnavailableText は systemd の状態そのものが判定できないときの表記。
	//
	// 「ユニットが無い」と「ユニットが有るか分からない」を書き分ける。一覧では
	// どちらも 1 セルの記号（- と ?）だが、詳細では取り違えると利用者が
	// 「サービス登録されていない」と誤って判断して登録し直しに向かってしまう。
	managedUnavailableText = "判定不能（ユニット一覧を取得できませんでした）"
)

// Model は runner の詳細画面。情報部（pane.Detail）と操作リスト
// （organism.ChoiceList）の組み合わせで構成し、詳細画面専用の organism を作らない
// （atomic-design.md の「詳細画面の操作リストは ChoiceList を使う」）。
//
// Runners タブと Jobs タブが共用する（FR-46 / FR-47）。Jobs タブの操作対象も
// runner なので、同じ部品をそのまま開ける。
type Model struct {
	keys   keymap.Set
	styles token.Styles
	// actions はキー定義から 1 度だけ組んだ操作の表。描画のたびに組み直さない
	// （action.Set の doc）。
	actions action.Set

	target runner.Runner
	caps   appconfig.Caps
	// disk は _work 使用量を引くための共有状態（Issue #73）。未集計なら work 行は
	// パスだけになる。
	disk page.DiskState

	info    pane.Detail
	list    organism.ChoiceList
	opRows  int // 操作リストが使う行数（項目 + 区切り線）
	infoLen int // 情報部の行数
	width   int
	height  int
}

// newModel は詳細画面を組み立てる。
func newModel(keys keymap.Set, s token.Styles) Model {
	return Model{
		keys:    keys,
		styles:  s,
		actions: action.NewSet(keys.Runner, page.ScopeState{}),
		target:  runner.Runner{},
		caps:    appconfig.Caps{},
		info:    pane.NewDetail(),
		list:    organism.NewChoiceList(keys.List, s),
		opRows:  0,
		infoLen: 0,
		width:   0,
		height:  0,
	}
}

// Open は対象を差し替えて詳細を開く。
//
// 操作リストのカーソルは organism.ResetCursor で先頭（安全側）へ戻す。
// 一覧の enter → 詳細の enter で破壊的操作に到達しないための規則（FR-46）である。
//
// **情報部のスクロールも先頭へ戻す。** 80x24 では情報部に配れる行数が 1 行まで
// 潰れる（infoHeight）ため、位置を持ち越すと別の runner の詳細が読んでいた場所から
// 始まり、前の runner の続きを今の runner の情報として読むことになる。同じ対象の
// 状態が変わっただけの SetState では戻さない（読んでいた場所を失う）。
func (d *Model) Open(r runner.Runner, caps appconfig.Caps) {
	d.target, d.caps = r, caps
	items := d.actions.Choices(r, caps)
	d.list.SetItems(items, organism.ResetCursor)
	d.opRows = len(items) + 1 // 破壊的操作の前に置く区切り線 1 本
	d.refresh()
	d.info.GotoTop()
}

// SetSize は詳細に割り当てられた領域を設定する。
func (d *Model) SetSize(w, h int) {
	d.width, d.height = w, h
	d.refresh()
}

// SetState は共有状態のスナップショットを反映する。
//
// **開いている詳細を最新の検出結果で組み直す。** 開いた時点の runner と Caps を持ち
// 続けると、3 秒ごとの再検出で状態が変わっても画面は古いまま（サービスが落ちた・
// ジョブが始まった、が見えない）になる。さらに操作の可否は Busy() や Svc から決まる
// ため、**モーダルを開いた後にジョブを取り始めた runner に対して「削除できる」と
// 提示してしまう**（actions.go の page.reasonBusy）。
//
// 対象は同一性（Dir）で引き直す。検出結果に見つからない周期では今の値を保つ。
// 3 秒ごとの再検出は一時的に 1 台を取りこぼすことがあり、そのたびに詳細が空に
// なると読めないためである。
//
// 操作リストのカーソルは保つ（organism.KeepCursor）。同じ対象の状態が変わっただけで
// カーソルが先頭へ戻ると、操作を選んでいる途中で選択がずれる。対象そのものを
// 差し替える Open は逆に先頭へ戻す（FR-46）。
func (d *Model) SetState(st page.StateMsg) {
	d.keys, d.styles = st.Keys, st.Styles
	d.actions = action.NewSet(st.Keys.Runner, st.Scopes)
	d.list.Restyle(st.Keys.List, st.Styles)
	d.caps = st.Caps
	d.disk = st.Disk
	if r, ok := findRunner(st.Result.Runners, d.target.Dir); ok {
		d.target = r
	}

	items := d.actions.Choices(d.target, d.caps)
	d.list.SetItems(items, organism.KeepCursor)
	d.opRows = len(items) + 1
	d.refresh()
}

// findRunner は同じディレクトリの runner を検出結果から探す。
//
// ディレクトリを同一性に使うのは、runner の名前がディレクトリ名から決まり
// （runner.Runner.Name）、同じホスト上で重複しない唯一の値だからである。
func findRunner(runners []runner.Runner, dir string) (runner.Runner, bool) {
	if dir == "" {
		return runner.Runner{}, false
	}
	for _, r := range runners {
		if r.Dir == dir {
			return r, true
		}
	}
	return runner.Runner{}, false
}

// Title は詳細画面の見出しを返す。
func (d Model) Title() string {
	return d.target.Name() + "  詳細"
}

// Target は詳細が今表示している runner を返す。決定に添えるために使う。
func (d Model) Target() runner.Runner { return d.target }

// Cursor は操作リストのカーソル位置を返す。
func (d Model) Cursor() int { return d.list.Cursor() }

// Hints は詳細画面のフッタに出すキーヒントを返す（screens.md の詳細画面）。
//
// 操作キーの可否と理由は操作リストの各行に出るため、フッタには載せない。
// キー文字列は keymap から取り、説明だけを画面に合わせる。
func (d Model) Hints() []atom.Hint {
	l := d.keys.List
	return []atom.Hint{
		{Key: page.BindingKey(l.Accept), Desc: "実行", Enabled: true, Reason: ""},
		{Key: page.BindingKey(l.Down) + "/" + page.BindingKey(l.Up), Desc: "選択", Enabled: true, Reason: ""},
		{Key: page.BindingKey(d.keys.Global.Back), Desc: "戻る", Enabled: true, Reason: ""},
	}
}

// Update は詳細画面のキーを処理する。
//
// ページ送り以外のキーは操作リストへ渡す。情報部（viewport）と操作リストは
// どちらも j / k を使うため、両方へ流すとカーソル移動とスクロールが同時に起きる。
// 詳細画面の j / k は操作の選択（screens.md の詳細画面）なので、スクロールは
// ページ送りのキーだけに割り当てる。
func (d Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		d.info, cmd = d.info.Update(msg)
		return d, cmd
	}
	if key.Matches(press, d.keys.List.PageDown, d.keys.List.PageUp) {
		var cmd tea.Cmd
		d.info, cmd = d.info.Update(press)
		return d, cmd
	}

	var cmd tea.Cmd
	d.list, cmd = d.list.Update(press)
	return d, cmd
}

// View は情報部と操作リストを縦に並べて返す。
func (d Model) View() string {
	return strings.Join([]string{
		d.info.View(),
		"",
		d.styles.Header.Render(heading),
		d.list.View(),
	}, "\n")
}

// refresh は情報部の行を組み直し、領域を情報部と操作リストへ配る。
//
// 幅が変わると値の中略位置が変わるため、行はサイズが決まってから組み立てる。
func (d *Model) refresh() {
	lines := d.infoLines()
	d.infoLen = len(lines)
	d.info.SetContent(lines)
	d.info.SetSize(d.width, d.infoHeight())
	d.list.SetWidth(d.width)
}

// infoHeight は情報部に配る行数を返す。操作リストを先に確保する。
//
// 操作リストを優先するのは、情報部はスクロールできるが操作リストは全項目が
// 見えないと「押せない操作とその理由」を読めないためである。
func (d *Model) infoHeight() int {
	avail := d.height - d.opRows - headingRows
	return max(min(avail, d.infoLen), 1)
}

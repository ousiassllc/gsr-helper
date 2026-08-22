package page

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/template"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// Overlay はモーダルの重なりを管理する。
//
// キーは最上位の 1 枚にのみ渡し、背後の page には届けない。確認中に打った x が
// 背後の一覧で別の停止操作として解釈されることを構造的に防ぐためである
// （screens.md のモーダル表示中）。esc で 1 枚だけ閉じるのもここで担保する。
//
// **種類ごとの分岐を持たない。** 重なりの規則（キーは最上位だけ・esc は 1 枚）を
// 種類の数と無関係にするためである。モーダルを増やす Issue は Register で自分の
// 種類を足すだけで、この共有ファイルを触らない。
//
// **自分が乗っているタブ番号を持ち、登録したモーダルへ配る**（AttachMsg）。モーダルが
// 発行する Cmd の結果と page へ返す決定は Do(tab, ...) で包む必要があり、その tab を
// モーダルへ渡せるのは Overlay だけだからである。
//
// **写しは実体を共有する（値としての独立性はない）。** 変わりうる状態はすべて 1 つの
// overlayState が持ち、Overlay はその参照である。以前はスタックだけがスライスの
// 付け替えで写しごとに分かれ、map だけが共有される半端な状態だった（捨てた写しが
// 中身の変更だけを残してスタックの変更を失う）。組み立ては NewOverlay を通すこと
// （ゼロ値は使えない）。
type Overlay struct {
	tab int // 乗っているタブ番号。組み立て後は変わらない
	s   *overlayState
}

// NewOverlay はモーダルの重なりを組み立てる。tab には page 自身のタブ番号を渡す。
//
// ヘルプ（ModalHelp）だけを登録した状態で返す。ヘルプに出すキーの範囲は一覧 +
// runner 操作（keymap.Set.RunnerListHelp）を既定とし、別の範囲を持つタブは
// SetHelpScope で差し替える。それ以外のモーダルは画面が Register で足す。
func NewOverlay(tab int, keys keymap.Set, s token.Styles, dark bool) Overlay {
	// 最初のリサイズと検出が届く前でも配色とキー定義を持った状態で描けるようにする。
	st := StateMsg{Keys: keys, Styles: s, Dark: dark}
	o := Overlay{tab: tab, s: &overlayState{
		keys:   keys,
		styles: s,
		stack:  nil,
		modals: make(map[ModalKind]Modal),
		last:   st,
		size:   SizeMsg{W: 0, H: 0},
		width:  0,
		height: 0,
	}}
	// ヘルプの登録が返す Cmd は捨てる。helpModal は表示専用でドメイン層を呼ばず、
	// 登録時のどの Msg にも Cmd を返さない。画面が足すモーダルは戻り値を返すこと。
	o.Register(ModalHelp, newHelpModal(st, keymap.Set.RunnerListHelp))
	return o
}

// Register はモーダル 1 種類を登録する。登録時に自分のタブ番号・最新の共有状態・
// 領域が届くので、起動後に遅延登録しても次の周期を待たずに描ける。
//
// **同じ種類を二重に登録すると panic する。** ModalKind は各パッケージが自由に
// 宣言する文字列なので、別々の Issue が同じ綴りを選ぶと片方が到達不能になる。
// 黙って上書きすると症状は「enter を押しても何も起きない」になり、コンパイル
// エラーも実行時エラーもログも残らない。登録は page の組み立て時に決まるため、
// 誤りは最初の起動で必ず表面化する。
//
// **戻り値の Cmd は呼び出し側まで返すこと。** 登録した時点で処理を始めるモーダルが
// あり、捨てるとその処理が動かない。
func (o Overlay) Register(kind ModalKind, m Modal) tea.Cmd {
	if _, dup := o.s.modals[kind]; dup {
		panic("page: モーダルの種類が重複している: " + string(kind))
	}
	o.s.modals[kind] = m
	return tea.Batch(o.send(kind, AttachMsg{Tab: o.tab}), o.replay(kind))
}

// SetHelpScope は ? に出すキーの範囲を差し替える。
//
// Set から範囲を選ぶ関数を渡すのは、配色やキー定義が差し替わったときに Overlay が
// 自分で組み直せるようにするためである（page が declare し直す必要が無い）。
//
// 登録し直さず、開いているヘルプへ指示を送る。登録し直すと中身だけが黙って
// 入れ替わり、読んでいたスクロール位置も失われる。
func (o Overlay) SetHelpScope(scope HelpScope) tea.Cmd {
	return o.send(ModalHelp, scopeMsg{scope: scope})
}

// Open は種類を指定してモーダルを開く。既に開いていれば最上位へ動かす。
//
// 開く直前に最新の共有状態と領域をリプレイする。閉じている間は配らない（毎周期
// 作り直さない）ので、開いた時点で追いつかせる必要があるためである。
//
// 開くときに渡した Msg は中身の Model へそのまま届く。「何を開くか」（対象の runner や
// 確認の文面）を Msg で渡すことで、Overlay は種類ごとの引数を知らずに済む。
//
// **未登録の種類を開くと panic する**（Register の doc と同じ理由）。戻り値の Cmd は
// 呼び出し側まで返すこと（開いた瞬間に購読や計算を始めるモーダルが使う）。
func (o Overlay) Open(kind ModalKind, msg tea.Msg) tea.Cmd {
	if _, ok := o.s.modals[kind]; !ok {
		panic("page: 登録されていないモーダルを開こうとした: " + string(kind))
	}

	var replay tea.Cmd
	if !o.opened(kind) {
		replay = o.replay(kind)
	}
	cmd := o.send(kind, msg)
	o.push(kind)
	return tea.Batch(replay, cmd)
}

// OpenHelp は全キー一覧を開く。
func (o Overlay) OpenHelp() tea.Cmd {
	return o.Open(ModalHelp, nil)
}

// Close は最上位のモーダルを 1 枚だけ閉じる。
//
// 1 枚ずつ閉じるのは、詳細画面からヘルプを開いた後の esc で詳細まで消えると、
// 利用者が「どこへ戻ったか」を追えなくなるためである。
func (o Overlay) Close() {
	if len(o.s.stack) == 0 {
		return
	}
	o.s.stack = o.s.stack[:len(o.s.stack)-1]
}

// Modal は登録済みのモーダルを返す。
//
// 中身を具体型へ戻すのは登録した側の責任である（Overlay は tea.Model としてしか
// 持たない）。開いているかどうかは問わない。
func (o Overlay) Modal(kind ModalKind) (Modal, bool) {
	m, ok := o.s.modals[kind]
	return m, ok
}

// Active はモーダルを 1 枚以上開いているかを返す。ChromeMsg.Modal に載せる値である。
func (o Overlay) Active() bool { return len(o.s.stack) > 0 }

// Handles は page がこの Msg を Overlay へ渡すべきかを返す。
//
// page の「キー以外を配る」判定をこの 1 つに集める。開閉だけで判断すると、宛先を
// 明示した ModalMsg が閉じている間に捨てられ、page 宛の決定（ResultMsg）は
// モーダル自身へ戻って消える。
func (o Overlay) Handles(msg tea.Msg) bool {
	switch msg.(type) {
	case ModalMsg:
		// 宛先が決まっている。開いていなくても届ける。
		return true
	case ResultMsg:
		// 決定は page が解釈する。戻すと発行元のモーダルへ帰って捨てられる。
		return false
	default:
		return o.Active()
	}
}

// Update は宛先付きの Msg をその種類へ、それ以外を最上位のモーダルへ渡す。
//
// esc は最上位のモーダルが自分で解釈する（Modal.HandlesBack が真）ときだけ渡し、
// そうでなければここで 1 枚閉じる。入力や編集の取消を閉じる操作より先に解釈させる
// ためである。判断は真偽値 1 つに委ねるので、Overlay は種類ごとの分岐を持たない。
func (o Overlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if to, ok := msg.(ModalMsg); ok {
		// 宛先が明示されている。閉じていても最上位でなくてもその種類へ届ける。
		return o, o.send(to.Kind, to.Msg)
	}

	top, ok := o.top()
	if !ok {
		return o, nil
	}

	press, isKey := msg.(tea.KeyPressMsg)
	if isKey && key.Matches(press, o.s.keys.Global.Back) && !o.handlesBack(top) {
		o.Close()
		return o, nil
	}
	return o, o.send(top, msg)
}

// View は最上位のモーダルを枠に入れて返す。開いていなければ空文字を返す。
func (o Overlay) View() string {
	m, ok := o.topModal()
	if !ok {
		return ""
	}
	return template.Modal(template.ModalInput{
		Title:  o.s.styles.Header.Render(m.Title(m.Model)),
		Body:   m.Model.View().Content,
		Width:  o.s.width,
		Height: o.s.height,
	})
}

// Hints は最上位のモーダルのキーヒントを返す。開いていなければ nil を返す。
func (o Overlay) Hints() []atom.Hint {
	m, ok := o.topModal()
	if !ok {
		return nil
	}
	return m.Hints(m.Model)
}

// top は最上位のモーダルの種類を返す。
func (o Overlay) top() (ModalKind, bool) {
	if len(o.s.stack) == 0 {
		return "", false
	}
	return o.s.stack[len(o.s.stack)-1], true
}

// topModal は最上位のモーダルを返す。
func (o Overlay) topModal() (Modal, bool) {
	kind, ok := o.top()
	if !ok {
		return Modal{}, false
	}
	m, ok := o.s.modals[kind]
	return m, ok
}

// opened は指定した種類が既に開いているかを返す。
func (o Overlay) opened(kind ModalKind) bool {
	for _, k := range o.s.stack {
		if k == kind {
			return true
		}
	}
	return false
}

// push はモーダルを重ねる。同じ種類が既にあるときは最上位へ動かす。
//
// 種類ごとに部品を 1 つしか持たないため、同じ種類を 2 枚積むと閉じても中身が
// 変わらない「閉じられないモーダル」に見える。
func (o Overlay) push(kind ModalKind) {
	kept := make([]ModalKind, 0, len(o.s.stack)+1)
	for _, k := range o.s.stack {
		if k != kind {
			kept = append(kept, k)
		}
	}
	kept = append(kept, kind)
	o.s.stack = kept
}

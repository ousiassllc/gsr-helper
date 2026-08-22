package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// handleKey はキー入力を配送する。
//
// **ctrl+c 以外のキーはまず有効タブへ渡す。** グローバルキー（タブ切替・再読み込み・
// 終了）を解釈するのは、page がそのキーを自分では使わないと判断して差し戻してきた
// とき（page.GlobalKeyMsg）だけである。
//
// 親が先に解釈しないのは、モーダル表示中と入力中にグローバルキーを閉じ込める判断が
// page にしかできないためである。以前は ChromeMsg で受け取った状態で親が判断して
// いたが、その値は 1 打鍵ぶん古く、確認中に打った q でアプリが終わりえた
// （page.GlobalKeyMsg の doc）。閉じ込めが必要な状態を知っているのはモーダルを
// 持っている page だけなので、判断もそこに置く。
//
// キーが page と親で二重に解釈されないことは、同時に有効なキーが重複しないという
// 規則が担保する（keymap の TestNoDuplicateKeysInSameContext）。1〜7 や q は一覧の
// キーではないので、差し戻されるまでに一覧が反応することはない。
//
// ? と esc も page が処理する。ヘルプの中身が画面ごとのキー集合に依存し、モーダルは
// page が organism として保持する設計だからである（esc も「選択のクリア」か
// 「モーダルを 1 枚閉じる」かを page しか判断できない）。
func (a App) handleKey(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// 前の打鍵で出した案内は次の打鍵で消す（状態行に残り続けないようにする）。
	a.notice = ""

	if key.Matches(press, a.keys.Global.Interrupt) {
		return a.quit()
	}
	if !a.live(a.active) {
		// 差し戻してくれる page が居ないので親が直に解釈する。
		return a.handleGlobalKey(press)
	}

	a, cmd := a.forward(press)
	return a, cmd
}

// handleGlobalKey は page が解釈しなかったキーを処理する。
//
// 対象はタブ切替・再読み込み・終了だけである。page が使うキー（? / esc / 一覧の
// 移動 / 操作キー）はここへ戻ってこない。
func (a App) handleGlobalKey(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	g := a.keys.Global
	switch {
	case key.Matches(press, g.Quit):
		return a.quit()
	case key.Matches(press, g.Refresh):
		// 手動の再読み込みは Tick を待たずに検出の Cmd を発行する。実行中の検出が
		// あるときは重ねない（自動更新と同じ理由。discover.go の onTick）。その検出の
		// 結果は遅くとも discoverBudget 以内に届く。
		if a.inflight > 0 {
			return a, nil
		}
		cmd := a.discover()
		return a, cmd
	case key.Matches(press, g.TabNext):
		return a.moveTab(1)
	case key.Matches(press, g.TabPrev):
		return a.moveTab(-1)
	case key.Matches(press, g.TabSelect):
		return a.selectTab(press.String())
	default:
		return a, nil
	}
}

// selectTab は番号キーで指定されたタブへ移る。
//
// 無効なタブでは移らず、無効である理由を状態行に出す。何も起きないと利用者からは
// 「効かないキー」に見えるためである（screens.md の設計原則 2）。
//
// 番号キーは page を経由して差し戻されたものである（handleKey）。一覧が数字を
// 使わないため二重に解釈されることはない。
func (a App) selectTab(k string) (tea.Model, tea.Cmd) {
	for i := range a.tabs {
		if a.tabs[i].Key != k {
			continue
		}
		if !a.tabs[i].Enabled {
			a.notice = a.tabs[i].notice()
			return a, nil
		}
		return a.activate(i)
	}
	return a, nil
}

// moveTab は step の向きへタブを移る。無効なタブは飛ばし、端では折り返す。
func (a App) moveTab(step int) (tea.Model, tea.Cmd) {
	n := len(a.tabs)
	if n == 0 {
		return a, nil
	}

	for i := 1; i <= n; i++ {
		next := ((a.active+step*i)%n + n) % n
		if a.tabs[next].Enabled {
			return a.activate(next)
		}
	}
	return a, nil
}

// activate は有効タブを切り替え、新しいタブへ共有状態を配って ChromeMsg を促す。
//
// 切り替えた直後に chrome を初期化するのは、前のタブのモーダル・入力中・フッタが
// 1 フレームだけ残ることを防ぐためである。
func (a App) activate(i int) (tea.Model, tea.Cmd) {
	if i < 0 || i >= len(a.tabs) || !a.tabs[i].Enabled || i == a.active {
		return a, nil
	}

	// 離れるタブに終わりを知らせる。知らせないと、裏に回った page はストリームや
	// goroutine を畳む機会が無く、タブを行き来するたびに購読が積み上がる
	// （page.DeactivateMsg の doc）。
	a, off := a.forwardTo(a.active, page.DeactivateMsg{})

	a.active = i
	a.chrome = pageChrome(i)
	// 移動先には前面に戻ったことを知らせてから共有状態を配る。畳んだ処理を張り直す
	// page が、最新のスナップショットを持った状態で張り直せるようにするためである。
	a, on := a.forwardTo(i, page.ActivateMsg{})
	next, st := a.forward(a.state())
	return next, tea.Batch(off, on, st)
}

// quit は全 page に終了を知らせ、後始末の Cmd を流し切ってから終了する。
//
// tea.Quit を直に返すと、page が持つ長寿命の処理（ログの購読、監視の goroutine）は
// 畳まれないままランタイムが止まる。tea.Batch では終了と後始末が並走して同じ競合に
// なるため、tea.Sequence で**後始末を先に**流す（page.ShutdownMsg の doc）。
func (a App) quit() (tea.Model, tea.Cmd) {
	a, cleanup := a.broadcast(page.ShutdownMsg{})
	if cleanup == nil {
		return a, tea.Quit
	}
	return a, tea.Sequence(cleanup, tea.Quit)
}

// broadcast は有効な全タブへ Msg を配り、返った Cmd をまとめる。
//
// 選択中のタブだけに配らないのは、裏のタブも自分で始めた処理を持つためである
// （裏に回ったときに畳み損ねた処理をここで確実に閉じられる）。
func (a App) broadcast(msg tea.Msg) (App, tea.Cmd) {
	cmds := make([]tea.Cmd, 0, len(a.tabs))
	for i := range a.tabs {
		var cmd tea.Cmd
		a, cmd = a.forwardTo(i, msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return a, tea.Batch(cmds...)
}

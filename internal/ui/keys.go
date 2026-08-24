package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/tabset"
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
	if !tabset.Live(a.tabs, a.active) {
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
		// あるときは重ねない（自動更新と同じ理由。discovery.State.Start）。その検出の
		// 結果は遅くとも discovery.Budget 以内に届く。
		//
		// 重ねないときは理由を状態行に出す。黙って何もしないと「効かないキー」に
		// 見えるが（screens.md の設計原則 2）、待たされる時間は最大 15 秒ある。
		// 無効なタブの番号キー（selectTab）と同じ形の案内にそろえる。**Start は
		// 始めなかったことを nil でしか返さないので、案内を出すかは Busy で見る。**
		if a.disc.Busy() {
			a.notice = refreshNotice(g.Refresh)
			return a, nil
		}
		// _work 使用量も引き直す（Issue #73）。再検出とは別周期にしてあるので、
		// 明示的な再読み込みが唯一の更新契機である（background.go）。実行中は
		// 重ねないので、走査中の連打で goroutine は積み上がらない。
		cmd := tea.Batch(a.disc.Start(a.input()), a.work.Start(a.disc.Result().Runners))
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

// refreshNotice は検出中に再読み込みを押したときの案内を返す。
//
// キーと説明は keymap から取り、案内とヘルプで文言が食い違わないようにする。
func refreshNotice(b key.Binding) string {
	return "[" + page.BindingKey(b) + "]" + b.Help().Desc + " 検出中です"
}

// selectTab は番号キーで指定されたタブへ移る。
//
// 無効なタブでは移らず、無効である理由を状態行に出す。何も起きないと利用者からは
// 「効かないキー」に見えるためである（screens.md の設計原則 2）。
//
// 番号キーは page を経由して差し戻されたものである（handleKey）。一覧が数字を
// 使わないため二重に解釈されることはない。
func (a App) selectTab(k string) (tea.Model, tea.Cmd) {
	i, ok := tabset.IndexOfKey(a.tabs, k)
	if !ok {
		return a, nil
	}
	if !a.tabs[i].Enabled {
		a.notice = a.tabs[i].Notice()
		return a, nil
	}
	return a.activate(i)
}

// openTab は page が求めたタブへ移り、用件をそのタブへ配る（page.OpenTabMsg）。
//
// 移動先は名前で指す。タブ同士が互いを知らないため、移動元は番号も型も持てない
// （page.OpenTabMsg の doc）。名前とタブ番号の対応を知っているのは親だけである。
//
// **既に前面に居る場合も用件は配る。** activate は同じタブへの移動を何もせずに
// 返すので、そこで打ち切ると Logs タブを開いたまま `l` を押した場合だけ何も
// 起きない。
//
// 無効なタブへは移らず、理由を状態行に出す（番号キーと同じ扱い。selectTab）。
// 名前が一致するタブが無い場合は何もしない。実装の誤りだが、利用者から見れば
// 「効かないキー」であり、落とすより静かに無視するほうが害が小さい。
func (a App) openTab(msg page.OpenTabMsg) (tea.Model, tea.Cmd) {
	i, ok := tabset.IndexOfTitle(a.tabs, msg.Title)
	if !ok {
		return a, nil
	}
	if !a.tabs[i].Enabled {
		a.notice = a.tabs[i].Notice()
		return a, nil
	}
	next, move := a.activate(i)
	if msg.Msg == nil {
		return next, move
	}
	deliver := tabset.Deliver(next.tabs, i, msg.Msg)
	return next, tea.Batch(move, deliver)
}

// moveTab は step の向きへタブを移る。無効なタブは飛ばし、端では折り返す。
func (a App) moveTab(step int) (tea.Model, tea.Cmd) {
	target, ok := tabset.Next(a.tabs, a.active, step)
	if !ok {
		return a, nil
	}
	return a.activate(target)
}

// activate は有効タブを切り替え、新しいタブへ共有状態を配って ChromeMsg を促す。
//
// 切り替えた直後に chrome を初期化するのは、前のタブのモーダル・入力中・フッタが
// 1 フレームだけ残ることを防ぐためである。
// 戻りを tea.Model ではなく App にするのは、移動のあとに用件を配る呼び出し元
// （openTab）が親の値を受け取る必要があるためである（forward と同じ理由）。
func (a App) activate(i int) (App, tea.Cmd) {
	if i < 0 || i >= len(a.tabs) || !a.tabs[i].Enabled || i == a.active {
		return a, nil
	}

	// 離れるタブに終わりを知らせる。知らせないと、裏に回った page はストリームや
	// goroutine を畳む機会が無く、タブを行き来するたびに購読が積み上がる
	// （page.DeactivateMsg の doc）。
	off := tabset.Deliver(a.tabs, a.active, page.DeactivateMsg{})

	a.active = i
	a.chrome = pageChrome(i)
	// 移動先には前面に戻ったことを知らせてから共有状態を配る。畳んだ処理を張り直す
	// page が、最新のスナップショットを持った状態で張り直せるようにするためである。
	on := tabset.Deliver(a.tabs, i, page.ActivateMsg{})
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

// broadcast は有効な全タブへ Msg を配り、返った Cmd をまとめる。配り方そのもの
// （選択中のタブだけに配らない理由）は tabset.Broadcast の doc を参照。
func (a App) broadcast(msg tea.Msg) (App, tea.Cmd) {
	return a, tabset.Broadcast(a.tabs, msg)
}

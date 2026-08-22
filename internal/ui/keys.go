package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// handleKey はキー入力を配送する。
//
// 段は 4 つあり、上の段で処理したキーは下の段へ渡さない
// （atomic-design.md のキー入力の配送）。
//
//  1. ctrl+c はどの状態でも親が処理して終了する
//  2. モーダル表示中と入力中は**グローバルキーを一切解釈せず**有効タブにのみ渡す
//  3. タブ切替・再読み込み・終了を親が処理する
//  4. 残りは有効タブへ渡す
//
// 2 段目でグローバルキーを解釈しないのは、runner 名が build01-1 のように数字を
// 含み、1〜7 を機能キーとして残すと名前で絞り込めないためである。モーダルへ
// キーを閉じ込めるのは、確認中に打った x が背後の一覧で別の停止操作として
// 解釈されることを防ぐためである。
//
// ? と esc を親で処理せず page へ渡すのは、ヘルプの中身が画面ごとのキー集合に
// 依存し、モーダルは page が organism として保持する設計だからである（esc も
// 「選択のクリア」か「モーダルを 1 枚閉じる」かを page しか判断できない）。
func (a App) handleKey(press tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// 前の打鍵で出した案内は次の打鍵で消す（状態行に残り続けないようにする）。
	a.notice = ""

	g := a.keys.Global
	if key.Matches(press, g.Interrupt) {
		return a, tea.Quit
	}
	if a.chrome.Modal || a.chrome.Input != "" {
		a, cmd := a.forward(press)
		return a, cmd
	}

	switch {
	case key.Matches(press, g.Quit):
		return a, tea.Quit
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
		a, cmd := a.forward(press)
		return a, cmd
	}
}

// selectTab は番号キーで指定されたタブへ移る。
//
// 無効なタブでは移らず、無効である理由を状態行に出す。何も起きないと利用者からは
// 「効かないキー」に見えるためである（screens.md の設計原則 2）。
//
// 有効タブへ転送しないのは、番号キーが親の担当だと決めた以上、押した番号が一覧の
// 操作として解釈されるのを避けるためである。
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

	a.active = i
	a.chrome = pageChrome(i)
	return a.forward(a.state())
}

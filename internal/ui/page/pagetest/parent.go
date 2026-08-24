package pagetest

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/discovery"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/workscan"
)

// 親 Model（internal/ui）を Msg で駆動するための道具。
//
// **App の非公開な状態には触れない。** 触れるもの（タブの差し替え・状態行の組み立て）は
// 内部テストにしか書けないので `ui` 直下に残る。ここに置けるのは tea.Model の口だけで
// 進められるもの——打鍵の往復・Cmd の束の解釈・周期の再現——である
// （atomic-design.md のディレクトリの行数）。
//
// 型引数を取るのは、親 Model（`ui.App`）とタブの具体型のどちらからも同じ手で進めたい
// ためである。呼び出し側は `pagetest.Update[App]` のように束縛して使う。

// Update は Msg を 1 つ渡し、進んだ Model と Cmd を返す。
//
// tea.Model の Update は interface を返すため、具体型で受け直すたびに型アサーションが
// 要る。取り違えは Model が進まないまま緑になるので、panic で止める。
//
// **具体型で具体化すること**（`Update[App]` / `Update[*Spy]`）。M を tea.Model 自身で
// 具体化すると next.(M) が必ず成立し、この panic は働かない。
func Update[M tea.Model](m M, msg tea.Msg) (M, tea.Cmd) {
	next, cmd := m.Update(msg)
	v, ok := next.(M)
	if !ok {
		panic(fmt.Sprintf("Update が %T ではなく %T を返した", m, next))
	}
	return v, cmd
}

// SendKey は打鍵を渡し、page が差し戻したグローバルキーまで解釈させる。
//
// 親はキーを必ず有効タブへ渡し、page が自分では使わないキーだけを page.GlobalKeyMsg
// として差し戻す（ui/keys.go の配送）。実機ではこの往復が bubbletea の Msg ループで
// 起きるため、テストでも同じ順で回す。返す Cmd は差し戻しを処理した結果のもの
// （差し戻しが無ければ打鍵そのものの結果）である。
func SendKey[M tea.Model](m M, k string) (M, tea.Cmd) {
	m, cmd := Update(m, Press(k))
	if _, global, ok := ScanKey(cmd); ok {
		return Update(m, global)
	}
	return m, cmd
}

// Press1 は打鍵を 1 つ送り、page が返した ChromeMsg と、差し戻しを親が解釈した
// 結果の Cmd を返す。**閉じ込められたときの Cmd は nil である**（page 自身の Cmd は
// 返さない。絞り込み中は点滅の Cmd が混じり、IsQuit で実行すると待たされる）。
//
// SendKey との違いは ChromeMsg を**親へ渡さない**ことである。前提は「親の状態は
// 1 打鍵ぶん古い」であり、渡すと閉じ込めの検証したい経路が消える。「親が page より
// 先にキーを解釈する」退行のほうは spy 経由の検証が受け持つ。
//
// 差し戻しを取りこぼして nil になる経路は無い（ScanKey の doc）。取りこぼしが nil に
// 化けると、閉じ込めを見る側の assertion がすべて満たされて静かに緑になる。
func Press1[M tea.Model](m M, k string) (M, page.ChromeMsg, tea.Cmd) {
	next, cmd := Update(m, Press(k))

	c, global, ok := ScanKey(cmd)
	if !ok {
		return next, c, nil
	}
	next, cmd = Update(next, global)
	return next, c, cmd
}

// ApplyChrome は Cmd に含まれる ChromeMsg を親へ渡した Model を返す。
//
// フッタは page が ChromeMsg で報告したものを親が描くため、フッタの表示を検証するには
// page → 親の 1 往復が要る。**入れ子の tea.Batch まで辿る**（1 段だけ展開する Expand
// ではなく Msgs を使うのはこのためである）。親は共有状態の配布と、起動後に 1 度だけ走る
// 取得を 1 つの Batch にまとめて返すので、1 段だけ展開すると配布ぶんが Batch のまま残る。
func ApplyChrome[M tea.Model](m M, cmd tea.Cmd) M {
	for _, msg := range Msgs(cmd) {
		c, ok := msg.(page.ChromeMsg)
		if !ok {
			continue
		}
		m, _ = Update(m, c)
	}
	return m
}

// IsQuit は Cmd が終了を指示しているかを返す。
//
// 終了は page の後始末を流し切ってから行うため tea.Sequence に包まれる
// （ui/keys.go の quit）。包みの中まで見ないと終了を見落とす。
func IsQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); ok {
		return true
	}
	seq, ok := Cmds(msg)
	if !ok {
		return false
	}
	for _, c := range seq {
		if c == nil {
			continue
		}
		if _, quit := c().(tea.QuitMsg); quit {
			return true
		}
	}
	return false
}

// Blocked はモーダル表示中と入力中の 2 つの状態を作る関数を名前で返す。
//
// どちらもグローバルキーを解釈しない状態であり、同じ配送の規則が働く。状態を持つのは
// page 側であり、閉じ込めの判断も page が行う（page.GlobalKeyMsg の doc）ため、
// spy の chrome に立てる。
func Blocked() map[string]func(s *Spy) {
	return map[string]func(s *Spy){
		"モーダル表示中": func(s *Spy) { s.Chrome.Modal = true },
		"入力中":     func(s *Spy) { s.Chrome.Input = "絞り込み" },
	}
}

// OpenTabOf は Cmd の結果から page.OpenTabMsg を取り出す。無ければ ok が偽。
func OpenTabOf(cmd tea.Cmd) (page.OpenTabMsg, bool) {
	for _, msg := range Msgs(cmd) {
		if open, ok := msg.(page.OpenTabMsg); ok {
			return open, true
		}
	}
	return page.OpenTabMsg{Title: "", Msg: nil}, false
}

// Discovered は検出が 1 周期終わった Model と、そのとき返った Cmd を返す。
// err を渡すと期限切れ・失敗した周期になる（結果は取り込まれない）。
//
// **通し番号は 0 のままである。** 既に番号付きの周期を取り込んだ Model へ渡すと
// discovery.State.Apply が追い抜かれた周期（msg.Seq < applied）と見て結果を捨てるため、
// 検出が届かないまま**静かに緑になる**。番号を進めた Model に対して使う場合は、この
// 関数ではなく discovery.Msg を直に組んで Seq を明示すること。
func Discovered[M tea.Model](m M, err error) (M, tea.Cmd) {
	return Update(m, discovery.Msg{
		Result: runner.Result{Runners: []runner.Runner{SampleRunner()}},
		Err:    err,
	})
}

// TakeHostReq は Cmd の束から起動時の前提チェック（FR-44）の結果を取り込む。
// 束に含まれていなければ ErrNotFound、束の展開が待ち時間内に戻らなければ
// ErrCmdTimeout を返す（HostReqOf の返しをそのまま渡す）。
func TakeHostReq[M tea.Model](m M, cmd tea.Cmd) (M, error) {
	msg, err := HostReqOf(cmd)
	if err != nil {
		return m, err
	}
	m, _ = Update(m, msg)
	return m, nil
}

// WorkScanStarts は検出成功を n 周期分流し、その間に発行された _work 集計の回数を返す。
// dir には t.TempDir() を渡す（このパッケージは testing を import しない）。
func WorkScanStarts[M tea.Model](m M, dir string, n int) int {
	res := runner.Result{Runners: []runner.Runner{{Dir: dir}}}
	starts := 0
	for seq := 1; seq <= n; seq++ {
		var cmd tea.Cmd
		m, cmd = Update(m, discovery.Msg{Seq: seq, Result: res, Err: nil})
		for _, msg := range Msgs(cmd) {
			if _, ok := msg.(workscan.Msg); ok {
				starts++
			}
			m, _ = Update(m, msg)
		}
	}
	return starts
}

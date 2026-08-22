package page

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// タブをまたぐ移動（Runners / Jobs の `l` から Logs タブを開く）の Msg を置く。

// OpenTabMsg は別のタブを前面に出し、用件を渡すよう親へ求める Msg。
//
// **タブ同士は互いを import しない**（atomic-design.md のディレクトリ構成。検査は
// page/pagetest/import_test.go の TestOnlyTabsetImportsTabs）ため、移動元は移動先の
// Model も型も参照できない。そこで移動先を名前（tabset.Tab.Title）で指し、用件は
// tea.Msg として運ぶ。名前とタブ番号の対応を知っているのは親だけで、page はどちらも
// 持たない。
//
// **page.Do で包まない。** 包むと発行元のタブへ戻ってしまう（TabMsg の doc）。宛先は
// 親であり、親は自分の Update で受けてから移動先へ配る。
//
// 名前が一致するタブが無い、あるいは無効な場合、親は移動せず理由を状態行に出す。
// 押しても何も起きないキーを作らないためであり、呼び出し側は可否を先に判定して
// フッタへ反映すること（screens.md の「無効な操作の表示」）。
type OpenTabMsg struct {
	// Title は移動先のタブ名。tabset.Tab.Title と一致させる。
	Title string
	// Msg は前面に出したあとその page へ配る用件。nil なら移動だけを行う。
	Msg tea.Msg
}

// TabLogs は Logs タブの名前。OpenTabMsg.Title に渡す。
//
// 文字列の一致で解決する以上、タブ名を変えたときにここが取り残されると移動だけが
// 静かに効かなくなる。tabset のテスト（TestOpenTabTitlesMatchTabs）が、この定数に
// 対応するタブが実在することを検査する。
const TabLogs = "Logs"

// ShowLogMsg は「この runner の直近ジョブの Worker ログを開く」という用件。
//
// OpenTabMsg.Msg に載せて Logs タブへ渡す（screens.md の `l`）。移動元（Runners /
// Jobs）と移動先（Logs）のどちらからも見える場所が共通の page しか無いため、
// タブ固有の用件だがここに置く。
type ShowLogMsg struct {
	Runner runner.Runner
}

// OpenTab は移動先のタブと用件を親へ伝える Cmd を返す。
func OpenTab(title string, msg tea.Msg) tea.Cmd {
	m := OpenTabMsg{Title: title, Msg: msg}
	return func() tea.Msg { return m }
}

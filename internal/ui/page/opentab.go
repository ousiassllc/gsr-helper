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

// TabRunners は Runners タブの名前。OpenTabMsg.Title に渡す。
//
// Setup タブの esc（前の画面へ戻る）が使う。**親は esc をタブの移動に使わない**
// （keys.go の handleGlobalKey）。Logs タブの esc は絞り込みの解除であり、
// 移動に使うと絞り込みを解くつもりの打鍵で画面ごと切り替わってしまうためである
// （screens.md「Logs タブから esc では戻らない」）。戻り先が一意に決まるタブだけが
// 自分で移動を要求する形にしてある。
const TabRunners = "Runners"

// TabSetup は Setup タブの名前。OpenTabMsg.Title に渡す。
//
// TabLogs と同じく、対応するタブが実在することを tabset のテストが検査する。
const TabSetup = "Setup"

// SetupOp は Setup タブへ依頼する操作。
//
// action.ID を使わないのは、依存が action → page の一方向であり page から
// action を参照できないためである（action/allow.go のパッケージコメント）。
type SetupOp int

const (
	// SetupAdd は runner の追加（FR-10〜FR-16）。
	SetupAdd SetupOp = iota
	// SetupRemove は runner の削除（FR-17〜FR-19）。
	SetupRemove
	// SetupUpdate は runner のバージョン更新（FR-20〜FR-22）。
	SetupUpdate
)

// SetupRequestMsg は「この runner に対して追加・削除・更新を始める」という用件。
//
// OpenTabMsg.Msg に載せて Setup タブへ渡す（screens.md の `n` / `D` / `u`）。
// ShowLogMsg と同じ理由でここに置く（移動元と移動先の双方から見える場所が
// page しか無い）。
//
// **確認ダイアログは移動先の Setup タブが出す。** 起点によって確認の強さを
// 変えないという規則（functional.md の操作フロー）は、確認の実装を 1 つに保つ
// ことで守る。移動元にも確認を置くと、同じ操作に 2 つの確認の組み立てができる。
type SetupRequestMsg struct {
	// Op は依頼する操作。
	Op SetupOp
	// Runners は対象。追加では空（Setup タブがフォームで受け取る）。
	Runners []runner.Runner
}

// OpenTab は移動先のタブと用件を親へ伝える Cmd を返す。
func OpenTab(title string, msg tea.Msg) tea.Cmd {
	m := OpenTabMsg{Title: title, Msg: msg}
	return func() tea.Msg { return m }
}

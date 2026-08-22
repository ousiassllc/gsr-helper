// Package page はタブ共通の Msg と、タブ間で共有する部品を提供する。
//
// タブ 1 枚（tea.Model）はサブパッケージ page/<tab> に置き、このパッケージだけを
// 共通の土台として参照する。page/<tab> → page の一方向依存は Go の import で
// 強制されるため、タブ同士が参照し合うことはできない（タブ間で共有する状態は
// 親 Model のみが持つという規則と揃う。atomic-design.md のディレクトリ構成）。
//
// このパッケージが持つのは次の 3 つである。
//   - 親 Model と page の間でやり取りする Msg（StateMsg / ChromeMsg）
//   - 操作の可否と理由の判定（actions.go）
//   - Runners タブと Jobs タブが共用する詳細画面とモーダルの重なり（detail.go / overlay.go）
//
// ドメイン層を tea.Cmd で呼ぶのは page 階層のみである（atomic-design.md の依存の規則）。
package page

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// StateMsg は親が持つ共有状態のスナップショット。全 page の Update に配られる。
//
// bubbletea の View() は引数を取れないため、page が描画時に共有状態を参照するには
// スナップショットを保持する必要がある。仕様書（atomic-design.md の page の責務）の
// 「page は検出結果を保持しない」は「共有状態の真実は親が持ち、page は親から渡された
// スナップショットを描画に使うだけで、自分では取得しない」と読み替える。page が
// Discover を呼ばないという本質（同じ検出が重複実行されず、タブ間でデータが食い違わない）
// は維持される。
//
// 端末サイズは template.BodySize で本体領域に換算した値を配る。サイズの真実を
// 1 箇所に集めるため、page より下は自分でサイズを問い合わせない。
type StateMsg struct {
	Result runner.Result
	Caps   appconfig.Caps
	Styles token.Styles
	Keys   keymap.Set
	// Exec は外部プロセス実行の唯一の経路。page がドメイン層を tea.Cmd で呼ぶときに使う。
	//
	// 共有状態に載せるのは、ドメイン層を呼べるのが page 階層だけ（atomic-design.md の
	// 依存の規則）であり、その page に Executor を渡す道が他に無いためである。載せないと
	// タブを足す Issue ごとにこの構造体と親 Model の 2 箇所を直すことになる。
	//
	// **systemctl が無い環境でも nil にはしない。** 検出（discover.go）は Executor を
	// nil にして systemd の参照を落とす縮退を持つが、それは runner.Discover の契約で
	// あってこの層の約束ではない。page は systemctl を使えるかを Caps.Systemd で判断し、
	// nil 判定を各タブに書かせない。
	//
	// 監査記録の失敗は Executor 自身が通知先（command.WithAuditErrorFunc）へ渡し、
	// cmd 側が TUI の終了後にまとめて出す。**UI から stderr へ書かない**（描画が壊れる）ため、
	// 監査エラーの受け皿を page へ配る必要はない。
	Exec  exec.Executor
	Dark  bool
	BodyW int
	BodyH int
	Err   error // 直近の検出エラー
}

// TabMsg は page が発行した Cmd の結果を、発行元のタブへ差し戻すための包み。
//
// 親 Model はキー・StateMsg・ChromeMsg 以外の Msg を選択中のタブへ配る（app.go の
// forward）。ドメイン層の呼び出しは page が tea.Cmd で行うため、結果が届くのは
// 早くても次のフレームであり、その間に利用者がタブを切り替えられる。包まずに流すと
// 結果が**別のタブへ渡って失われ**、裏のタブは処理を完了できない。Tab を持たせて
// 発行元へ戻すことで、どのタブも自分が始めた処理を必ず終えられる。
type TabMsg struct {
	Tab int     // 発行元のタブ番号（page が持つ tab）
	Msg tea.Msg // ドメイン層の呼び出し結果
}

// Do は page がドメイン層を呼ぶ唯一の入口。
//
// tab には page 自身のタブ番号を渡す。結果は TabMsg として発行元のタブへ戻るため、
// タブを切り替えても処理が迷子にならない。**ドメイン層を呼ぶ Cmd はすべてこの関数を
// 通すこと。** 直に tea.Cmd を返すと結果が選択中のタブへ配られ、非同期の結果が
// 静かに失われる（TabMsg の doc）。
//
// 包むのは自分で発行するドメイン呼び出しだけにする。bubbletea / bubbles が解釈する
// Msg（終了・順次実行など）を包むとランタイムへ届かなくなるためである。organism は
// ドメイン層を呼ばない規約なので、包む場所は page に限られる。
func Do(tab int, fn func() tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return TabMsg{Tab: tab, Msg: fn()}
	}
}

// GlobalKeyMsg は page が自分では解釈しなかったキーを親へ差し戻す Msg。
//
// **モーダル表示中と入力中に page がこれを返さないことが、グローバルキーを閉じ込める
// 仕組みそのものである。** 以前は親が ChromeMsg で受け取ったモーダル・入力の状態を見て
// 配送を止めていたが、ChromeMsg は次のフレームで届くため親の値は 1 打鍵ぶん古く、
// 素早い連続打鍵（キーの押しっぱなし・貼り付け）では確認中に打った q でアプリが終わり、
// 1 でタブが変わりえた。**キーを閉じ込められるのは、モーダルを持っている page だけである。**
//
// 発行元のタブ番号は載せない。タブを切り替えた直後に前のタブから差し戻されたキーも
// 解釈する必要があるためである（載せて突き合わせると、切替の直後に打った q が捨てられる）。
// キーが配られるのは選択中のタブだけなので、差し戻しの出どころは常に「利用者が今
// 見ている画面」である。
type GlobalKeyMsg struct {
	Press tea.KeyPressMsg
}

// BubbleKey は page が解釈しなかったキーを親へ差し戻す Cmd を返す。
//
// page/<tab> は「入力中・モーダル表示中は差し戻さない」「自分のキーを処理し終えた
// 残りは差し戻す」の 2 つだけを守ればよい。タブ切替・再読み込み・終了の解釈は親が持つ。
func BubbleKey(press tea.KeyPressMsg) tea.Cmd {
	return func() tea.Msg { return GlobalKeyMsg{Press: press} }
}

// ChromeMsg は page が親へ返す、本体以外の描画とキー配送に必要な情報。
//
// 親は「モーダルが開いているか」「入力中か」を知る必要があるが、独自 interface は
// 作れない（interface は Executor / doctor.Check / tea.Model の 3 つに限るという規則。
// components/overview.md の主要な interface 一覧）。そこで page 側が状態変化を
// tea.Msg で親へ通知する。この形にすることで page は tea.Program を必要とせず
// 単体テストできる。
type ChromeMsg struct {
	Tab    int         // 発行元のタブ番号。親は有効タブのものだけを採用する
	Modal  bool        // モーダルを 1 枚以上開いているか
	Input  string      // 入力中の名称（"絞り込み"）。空なら入力中でない
	Status string      // 状態行に出す文
	Footer []atom.Hint // フッタのキーヒント（可否と理由込み）
}

// Package page はタブ共通の Msg と、タブ間で共有する部品を提供する。
//
// タブ 1 枚（tea.Model）はサブパッケージ page/<tab> に置き、このパッケージだけを
// 共通の土台として参照する。page → page/<tab> の向きは循環になるため Go が禁じる。
//
// **タブ同士が参照し合わないことは Go では強制されない。** 新しいタブが page/runners を
// 直に import してもコンパイルは通る。タブ間で共有する状態は親 Model のみが持つ、
// という規則（atomic-design.md のディレクトリ構成）を実際に守らせているのは
// page/pagetest/import_test.go の TestOnlyTabsetImportsTabs で、タブを import して
// よいのは ui/tabset だけであることを本番ファイルの import から検査する。
//
// このパッケージが持つのは次の 4 つである。
//   - 親 Model と page の間でやり取りする Msg（StateMsg / ChromeMsg と lifecycle.go）
//   - タブをまたぐ移動（opentab.go）と、親が配る設定の差し戻し（cfgsaved.go）
//   - モーダルの重なりと開閉（modal.go / modalcmd.go / overlay.go / overlaystate.go）
//   - タブ共通のキー束とヘルプ（binding.go / helpmodal.go）
//
// ドメイン層を tea.Cmd で呼ぶのは page 階層のみである（atomic-design.md の依存の規則）。
package page

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/audit"
	"github.com/ousiassllc/gsr-helper/internal/exec"
	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/setup/job"
	"github.com/ousiassllc/gsr-helper/internal/setup/tarball"
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
	Exec exec.Executor
	Dark bool
	// Color は色を使うか。NO_COLOR / --no-color / 非 TTY を cmd が 1 つの値に
	// まとめたもので、UI 側で環境を読み直さない。Styles には畳み込み済みだが、
	// huh のテーマを組み立てるには真偽値そのものが要る（token.HuhTheme）。
	Color bool
	BodyW int
	BodyH int
	Err   error // 直近の検出エラー
	// Setup は Setup タブが要る起動時の決定事項。1 つにまとめてあるのは、
	// タブ 1 枚のためだけに StateMsg のフィールドを 3 つ増やさないためである。
	Setup SetupDeps
	// Config は Config タブが要る起動時の決定事項。Setup と同じ理由で 1 つに
	// まとめてある。
	Config ConfigDeps
	// Audit は破壊的操作の記録先。外部コマンドを伴わない削除（internal/disk の
	// ファイル削除など）を記録するために page 階層まで配る（Issue #71）。
	//
	// nil / audit.Discard() は no-op で、監査ログを開けない場合の縮退はそのまま
	// 働く。UI から直接書き込む経路は無く、渡すだけで済む点は Exec と同じである。
	Audit *audit.Logger
	// Disk はディスク関連の共有状態（閾値と _work 使用量）。Issue #72 / #73。
	Disk DiskState
	// Scopes はトークンの保有スコープ。操作の可否の判定に使う。Issue #79。
	Scopes ScopeState
}

// DiskState はタブをまたいで共有するディスク関連の状態。
//
// 閾値（設定由来の静的な値）と使用量（親が駆動する集計の結果）を 1 つにまとめて
// あるのは、Setup / Config と同じく「タブ 1 枚のために StateMsg のフィールドを
// いくつも増やさない」ためである。どちらも Disk タブ専用ではなく、閾値は doctor の
// リソース診断が、使用量は Runners タブと runner 詳細画面が読む。
type DiskState struct {
	// Thresholds はディスク使用率の警告閾値（設定ファイルの disk_thresholds）。
	//
	// 既定値（80 / 90）を表示側に埋め込まないために配る。埋め込むと、設定を
	// 変えても表示だけが既定のまま残る。
	Thresholds appconfig.DiskThresholds
	// Work は runner ごとの _work 使用量。キーは runner.Runner.Dir。
	//
	// **キーが無いことが「まだ集計していない」を表す。** 0 バイトは空の _work と
	// いう有効値なので、ゼロ値と兼用しない。未集計の runner は `-` に縮退する。
	//
	// runner 名ではなくディレクトリで引くのは、名前が .runner の AgentName 由来で
	// 重複しうるのに対し、ディレクトリは検出結果の中で一意だからである（Runners
	// タブの一覧も行の識別子に Dir を使っている）。
	Work map[string]WorkUsage
}

// WorkUsage は runner 1 台の _work 使用量。
//
// 失敗を戻り値ではなく値として持つのは、1 台の集計失敗で他の runner の表示を
// 落とさないためである（disk.Usage.Err と同じ考え方）。
type WorkUsage struct {
	// Bytes は使用量。Err が非 nil のときの値は意味を持たない。
	Bytes int64
	// Err は集計に失敗した理由。nil なら成功。失敗した runner は `-` に縮退する。
	Err error
}

// WorkText は runner の _work 使用量を表示用の文字列で返す（Issue #73）。
//
// **未集計と集計失敗はどちらも空文字を返す。** 呼び出し側はそれを "-" に縮退させる
// （listrow の dashCell）。書き分けないのは、利用者にとってどちらも「今は分からない」で
// あり、一覧の 1 セルに理由を書く余地が無いためである。集計は起動直後には終わって
// いないので、未集計は異常ではない。
//
// 0 バイトは有効な値としてそのまま出す（空の _work を持つ runner がある）。
func (d DiskState) WorkText(dir string) string {
	u, ok := d.Work[dir]
	if !ok || u.Err != nil {
		return ""
	}
	return atom.Bytes(u.Bytes)
}

// ScopeState は起動時に 1 度だけ引いたトークンの保有スコープ（Issue #79）。
//
// **Caps に載せていない。** 能力判定（appconfig/hostcaps.Detect）は起動シーケンス上で
// 同期的に走り 800ms の予算を持つが、保有スコープの確認は GitHub API への往復を
// 要する。Caps に混ぜると「起動から一覧表示まで 1 秒以内」（non-functional.md）を
// 壊すため、親が非同期に引いて確定した時点でこの値として配る。
type ScopeState struct {
	// Scopes は保有スコープ。Known が偽の間は意味を持たない。
	Scopes gh.Scopes
	// Known は取得を終えたか。
	//
	// **偽の間は操作を塞がない。** 判定が済む前に塞ぐと、権限の足りている
	// トークンで起動直後だけ操作できなくなる。取得に失敗した場合も偽のままにして、
	// 塞がない側へ倒す（screens.md「無効な操作の表示」の 6 段目）。
	Known bool
}

// ConfigDeps は Config タブ（対話型設定編集）が要る値。
//
// 本ツール自身の設定（FR-41〜FR-42）を扱うために要るもので、runner 側の設定
// （.env / .path / drop-in）は検出結果（Result）から引けるためここには無い。
//
// Path と FirstRun は cmd が起動時に決め、UI 側で環境や設定を読み直さない（SetupDeps と
// 同じ方針）。**設定ファイルのパスを UI 側で決め直さない**のは、配置先の決定が
// appconfig/confpath の責務であり、SUDO_USER の扱いを 2 か所に分けないためである。
// **Conf だけは起動時の値に固定されない**——Config タブが書き込めた設定を
// ConfigSavedMsg で親へ返し、親が差し替えて配り直す（Issue #128）。
type ConfigDeps struct {
	// Conf は読み込み済みの自身の設定。ウィザードの初期値に使う。
	Conf appconfig.Config
	// Path は設定ファイルの配置先。書き込み先と差分の見出しに使う。
	Path string
	// FirstRun は設定ファイルが無い状態で起動したか（FR-41）。
	//
	// appconfig.Load はファイルが無くても既定値を返すため、Conf からは初回起動を
	// 判別できない。判定は cmd が appconfig.Exists で行う。
	FirstRun bool
}

// SetupDeps は Setup タブ（runner の追加・削除・バージョン更新）が要る値。
//
// いずれも cmd が起動時に決め、UI 側で環境や設定を読み直さない
// （docs/ui/atomic-design.md「背景の明暗と NO_COLOR」と同じ方針）。
type SetupDeps struct {
	// Host は既定の runner 名の接頭辞に使うホスト名（FR-11）。
	Host string
	// Defaults は設定ファイルの defaults（インストール先・ラベル・ephemeral）。
	Defaults appconfig.Defaults
	// Secrets は取得した短命トークンの預け先。
	//
	// ここへ預けた値は exec の値一致マスク（段 2）に載り、監査ログと
	// エラー文言から平文が消える（docs/architecture/security.md）。
	Secrets *gh.Secrets
	// NewClient は API クライアントの生成を差し替える口（job.Deps.NewClient と同じ形）。
	//
	// **テストが本物の GitHub へ出ないようにするための継ぎ目である。** 本番は nil を
	// 渡し、job 側が gh.Token から借りたトークンで api.github.com 向けの
	// クライアントを作る。nil のままではテストも同じ経路に落ち、周囲の GH_TOKEN を
	// 拾って短命トークンの発行（remove-token）まで実際に叩いてしまう。t.Parallel を
	// 使う以上 t.Setenv で環境を消すこともできないので、差し替えの口を共有状態に
	// 持たせて、テストは httptest のサーバへ向ける。
	NewClient func(ctx context.Context, d job.Deps) (*gh.Client, error)
	// Fetch は tarball の取得を差し替える口（job.Deps.Fetch と同じ形）。
	//
	// NewClient と同じ理由で置く。本番は nil で tarball.Fetch を使い、テストは
	// 外向きのダウンロードが起きない実装を挿す。
	Fetch func(ctx context.Context, in tarball.Info, dir string) (string, error)
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

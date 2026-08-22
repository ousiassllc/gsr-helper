package keymap

import "charm.land/bubbles/v2/key"

// RunnerKeys は runner に対する操作のキー。
//
// 小文字は安全側、大文字は影響が大きい側に割り当てる（screens.md の設計原則 3）。
// Jobs タブの操作対象も runner であり、同じ定義を使う。
type RunnerKeys struct {
	Start   key.Binding // 開始
	Stop    key.Binding // 停止
	Kill    key.Binding // 強制停止
	Drain   key.Binding // ドレイン停止
	Restart key.Binding // 再起動
	Enable  key.Binding // enable / disable の切り替え

	Add    key.Binding // runner を追加
	Delete key.Binding // runner を削除
	Update key.Binding // バージョン更新
	Edit   key.Binding // 設定を編集
	Logs   key.Binding // ログを開く
}

// NewRunnerKeys は runner の操作キーの定義を返す。
func NewRunnerKeys() RunnerKeys {
	return RunnerKeys{
		Start: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "開始"),
		),
		Stop: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "停止"),
		),
		Kill: key.NewBinding(
			key.WithKeys("X"),
			key.WithHelp("X", "強制停止"),
		),
		Drain: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "ドレイン停止"),
		),
		Restart: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "再起動"),
		),
		Enable: key.NewBinding(
			key.WithKeys("E"),
			key.WithHelp("E", "enable/disable の切替"),
		),
		Add: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "追加"),
		),
		Delete: key.NewBinding(
			key.WithKeys("D"),
			key.WithHelp("D", "削除"),
		),
		Update: key.NewBinding(
			key.WithKeys("u"),
			key.WithHelp("u", "バージョン更新"),
		),
		Edit: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "設定を編集"),
		),
		Logs: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "ログを開く"),
		),
	}
}

// Bindings は runner の操作キーを定義順で返す。
func (r RunnerKeys) Bindings() []key.Binding {
	return []key.Binding{
		r.Start, r.Stop, r.Kill, r.Drain, r.Restart, r.Enable,
		r.Add, r.Delete, r.Update, r.Edit, r.Logs,
	}
}

// Order は全キーの一覧（?）に並べる順を返す。
//
// 安全な操作を先、破壊的な操作を後に置く。フッタは Footer、詳細画面の操作リストは
// Detail が並びを決める。3 つを分けているのは、載せる操作の集合が画面ごとに違う
// （フッタは幅 80 に収まる 9 個、詳細は対象 runner に対する操作だけ）ためである。
func (r RunnerKeys) Order() []key.Binding {
	return []key.Binding{
		r.Logs, r.Start, r.Drain, r.Restart, r.Enable, r.Edit, r.Update, r.Add,
		r.Stop, r.Kill, r.Delete,
	}
}

// FooterKey はフッタ 1 行目に出す操作 1 つ分の定義。
//
// 説明文を Binding と別に持つのは、ヘルプ（?）では「ドレイン停止」「enable/disable の
// 切替」のように長い表記を保ちつつ、フッタでは screens.md の共通レイアウトの短い表記
// （「ドレイン」）を使うためである。長い表記のままだと 11 個のうち 4 個しか幅 80 に
// 収まらず、「有効なキーを常に画面に出す」（設計原則 1）を満たせない。
type FooterKey struct {
	Binding key.Binding // 押すキーの定義
	Desc    string      // フッタ用の短い表記
}

// Footer はフッタ 1 行目に並べる操作を screens.md の共通レイアウトの順で返す。
//
// 並びと表記は screens.md のフッタ 1 行目
// （s:開始 x:停止 X:強制 d:ドレイン D:削除 n:追加 u:更新 e:設定 l:ログ）に一致させる。
// R（再起動）と E（enable/disable の切替）を載せないのは、この 9 個 + ?:ヘルプ で
// 幅 80 のうち 77 セルを使い切るためである。2 つは詳細画面の操作リスト（Detail）と
// ? の全キー一覧から辿れる。
func (r RunnerKeys) Footer() []FooterKey {
	return []FooterKey{
		{Binding: r.Start, Desc: "開始"},
		{Binding: r.Stop, Desc: "停止"},
		{Binding: r.Kill, Desc: "強制"},
		{Binding: r.Drain, Desc: "ドレイン"},
		{Binding: r.Delete, Desc: "削除"},
		{Binding: r.Add, Desc: "追加"},
		{Binding: r.Update, Desc: "更新"},
		{Binding: r.Edit, Desc: "設定"},
		{Binding: r.Logs, Desc: "ログ"},
	}
}

// JobsFooter は Jobs タブのフッタに出す操作を screens.md の Jobs タブの順で返す。
//
// Footer の部分集合を別に返すのは、Jobs タブが載せる操作が 4 つ（ドレイン・強制停止・
// 再起動・ログ）だけだからである。削除（D）と設定編集（e）を出さないのは、screens.md の
// Jobs タブでこの 2 つを詳細画面から辿る操作としているためで、フッタの先頭に置く
// `enter:runner の詳細` がその入口になる。再起動（R）は逆に、幅の都合で Footer に
// 入れられなかったものをここで出す（Jobs タブは出すキーが少なく幅に余裕がある）。
//
// 表記は Footer と同じ短いものを使う。page 側で書き直してはならない。キーと説明文の
// 出どころを 1 つにするのが keymap の存在理由であり、page が独自の文言を持つと、
// キーを差し替えたときに Jobs タブのフッタだけが古い表記のまま残る。
func (r RunnerKeys) JobsFooter() []FooterKey {
	return []FooterKey{
		{Binding: r.Drain, Desc: "ドレイン"},
		{Binding: r.Kill, Desc: "強制"},
		{Binding: r.Restart, Desc: "再起動"},
		{Binding: r.Logs, Desc: "ログ"},
	}
}

// Detail は詳細画面の操作リストに並べる操作を返す。
//
// 並びは screens.md の詳細画面（l / s / d / R / E / e / u ─ x / X / D）に一致させる。
// 安全な操作が先、破壊的な操作（x / X / D）が後で、区切り線はその境目に入る。
//
// n（追加）は載せない。詳細画面は「選択中の runner に対する操作」の起点であり、
// 追加は対象となる runner を持たない。screens.md の詳細画面のキー表にも無い。
//
// E（切替）は載せる。フッタには幅の都合で出せないため、ここと ? の全キー一覧が
// 利用者が E を辿れる唯一の経路である（screens.md の Runners タブの操作）。
func (r RunnerKeys) Detail() []key.Binding {
	return []key.Binding{
		r.Logs, r.Start, r.Drain, r.Restart, r.Enable, r.Edit, r.Update,
		r.Stop, r.Kill, r.Delete,
	}
}

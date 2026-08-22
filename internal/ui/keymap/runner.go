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

// Order はフッタと詳細画面の操作リストに並べる順を返す。
//
// 安全な操作を先、破壊的な操作を後に置く。詳細画面では区切り線の下に
// 破壊的な操作をまとめるため、この順がそのまま表示順になる。
func (r RunnerKeys) Order() []key.Binding {
	return []key.Binding{
		r.Logs, r.Start, r.Drain, r.Restart, r.Enable, r.Edit, r.Update, r.Add,
		r.Stop, r.Kill, r.Delete,
	}
}

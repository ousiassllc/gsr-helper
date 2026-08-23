package keymap

import "charm.land/bubbles/v2/key"

// LogKeys は Logs タブに固有のキー（screens.md の Logs タブのキーマップ）。
//
// スクロール（`j` / `k` / `ctrl+f` / `ctrl+b`）・末尾へ移動（`G`）・フィルタ（`/`）は
// List が持つものをそのまま使う。同じ動きに別のキーを割り当てないためであり、
// ここに持つのは Logs タブにしか無い 3 つに限る。
type LogKeys struct {
	// Pane はファイル一覧と本文のどちらを操作するかの切り替え。
	//
	// **Logs タブでは Global.TabNext（次のタブ）が働かない。** screens.md が Logs タブの
	// `tab` をペインの切り替えに割り当てているためで、page が消費して親へ差し戻さない。
	// タブの移動は番号キーと `shift+tab` で行える。
	Pane key.Binding
	// Follow は末尾への追従の ON / OFF。
	Follow key.Binding
	// Journal は journalctl ビューへの切り替え。
	Journal key.Binding
}

// NewLogKeys は Logs タブのキーの定義を返す。
func NewLogKeys() LogKeys {
	return LogKeys{
		Pane: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "ファイル一覧 / 本文の切り替え"),
		),
		Follow: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "追従の ON/OFF"),
		),
		Journal: key.NewBinding(
			key.WithKeys("J"),
			key.WithHelp("J", "journalctl ビューへ切り替え"),
		),
	}
}

// Bindings は Logs タブのキーをヘルプに並べる順で返す。
func (l LogKeys) Bindings() []key.Binding {
	return []key.Binding{l.Pane, l.Follow, l.Journal}
}

// Footer はフッタ 1 行目に並べる操作を返す。
//
// 説明を Binding と別に持つ理由は RunnerKeys.Footer と同じで、ヘルプでは長い表記を
// 保ちつつフッタでは幅 80 に収まる短い表記を使うためである。スクロールと末尾移動は
// 端末 UI の慣習として説明せずとも通じるので、フッタには載せない
// （設計原則 1 の「有効なキーを常に画面に出す」は、この画面に固有の操作を指す）。
func (l LogKeys) Footer() []FooterKey {
	return []FooterKey{
		{Binding: l.Pane, Desc: "ペイン"},
		{Binding: l.Follow, Desc: "追従"},
		{Binding: l.Journal, Desc: "journal"},
	}
}

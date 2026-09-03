// Package dialog は承認・待機・入力のダイアログを提供する。
//
// organism の部品を性質で分ける方針（atomic-design.md の organism の分割方針）に従い、
// カーソルと選択を持つ一覧・選択は organism / organism/table に、スクロールするだけの
// 表示専用の領域は organism/pane に、**判断か待機を求めて画面を占有する部品**をここに
// 置く。持つ状態も検証の観点も違うためである。
//
// このパッケージには破壊的操作の確認（Confirm）とドレイン待機（DrainWaiter）を置く。
// 差分承認（DiffApproval）とフォームのラッパー（Form）も同じ性質だが、それぞれ
// Config / Setup タブを持ち込む別の Issue が実装する（huh も未導入である）。
//
// **確認ダイアログの実装はこのパッケージの Confirm 1 つに統一する。** screens.md に
// 現れる確認（停止 / 強制停止 / 削除 / バージョン更新 / クリーンアップ / 追加の
// プレビュー / 設定の書き込み）はすべて「対象・影響・実行するコマンド・y/N」という
// 同じ構造を持つ。組み立てる経路を 1 つに限ることで、「確認を経ない削除経路は設けない」
// （FR-30）を構造として守る。個別の確認ダイアログ organism を追加しないこと。
//
// organism とこのパッケージ（および organism/table / organism/pane）は**どの向きにも
// import しない**。分割の目的は行数上限の分散であり、部品同士の依存を増やすことでは
// ない。組み合わせるのは page の役割である。import するのは atom / molecule / token /
// keymap と bubbles / bubbletea / lipgloss に限る。
//
// この階層の型は tea.Model を実装せず、bubbles 流の「具体型を返す Update と
// View() string」に揃える（organism パッケージの doc と同じ規約）。見出しは
// template.Modal が描くため、View が返すのはモーダルの**中身だけ**である。
package dialog

import (
	"charm.land/bubbles/v2/key"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
)

// fitHeight は body と tail を合わせて height 行に収める。
//
// **tail は必ず残し、超過分は body の末尾から落とす。** 枠（template.Modal）の
// 切り詰めは末尾から行を落とすため、任せると「実行しますか? [y/N]」や制約の注記
// といった**最も落としてはいけない行**から先に消える。ダイアログ側で先に削る行を
// 選び、可変長の中身（対象の一覧・待機中のジョブ）を犠牲にする。
//
// height が 0 以下のときは大きさが未設定なので何も落とさない。
func fitHeight(body, tail []string, height int) []string {
	if height <= 0 || len(body)+len(tail) <= height {
		return append(body, tail...)
	}
	keep := max(height-len(tail), 0)
	return append(body[:keep:keep], tail...)
}

// hint はキー定義から 1 つのキーヒントを作る。説明はダイアログ側で与える。
//
// キー文字列を定義から取るのは、フッタに出す表記と実際に効くキーがずれないように
// するためである。説明を差し替えられるようにしてあるのは、esc（keymap.Global.Back）の
// 既定の説明が「戻る」であり、待機のキャンセルのような画面ごとの意味を持たせる
// 必要があるからである。ダイアログのキーはどれも押せる状態でのみ描くので、
// 可否と理由は固定する（押せない確認ダイアログは開かない）。
func hint(b key.Binding, desc string) atom.Hint {
	return atom.Hint{Key: b.Help().Key, Desc: desc, Enabled: true, Reason: ""}
}

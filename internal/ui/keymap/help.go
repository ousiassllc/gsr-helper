package keymap

import (
	"slices"

	"charm.land/bubbles/v2/key"
)

// ? に出す全キー一覧（ヘルプ）の組み立てを集める。
//
// 同時に有効なキーの集合（keymap.go の Contexts）と分けているのは、**目的が違う**
// ためである。あちらは重複を機械が検査するためのもので、範囲は「その場面で実際に
// 効くキー」に厳密に絞る。こちらは押せるキーを人に見せるためのもので、範囲はもう
// 少し広い（画面仕様の「キー一覧」）。1 ファイル 300 行の上限
// （docs/ui/atomic-design.md）に収める際の切れ目もここになる（Issue #112）。

// Help は ? の全キー一覧のグループを組み立てる。bubbles/help の FullHelpView に渡す。
//
// **画面が自分で使うキーのグループだけを渡す。** ? の中身は画面ごとのキー集合に依存し
// （? を親が処理せず page へ渡す理由そのもの。ui/keys.go の doc）、全画面で同じ一覧を
// 返すと Disk / Logs / Config のキーが Runners のヘルプにも並ぶ。逆に画面ごとの一覧を
// このパッケージに列挙すると、タブを 1 つ足すたびに共有のこのメソッドを直す必要が出る。
//
// Global はどの画面でも有効なので必ず先頭に付ける。
//
// 全キー一覧は操作の可否を反映しない。可否は状況で変わるため、一覧の役割は
// キーと動作の対応を示すことに限る（screens.md の無効な操作の表示）。
func (s Set) Help(groups ...[]key.Binding) [][]key.Binding {
	return help(s.Global.Bindings(), groups...)
}

// help は先頭に置くグローバルキーのグループを指定して ? のグループを組み立てる。
//
// Help から分けているのは、Logs タブだけ先頭のグループが違う（tab を除く）ためである。
// Help に「先頭を差し替える」引数を足すと全画面の呼び出しが 1 画面の都合を背負うので、
// 組み立てだけをこの関数に置き、既定の先頭を渡す Help と Logs 用の先頭を渡す LogsHelp が
// それぞれ呼ぶ形にした。空のグループを落とすのは bubbles/help が空の列を描かないため。
func help(global []key.Binding, groups ...[]key.Binding) [][]key.Binding {
	out := make([][]key.Binding, 0, len(groups)+1)
	out = append(out, global)
	for _, g := range groups {
		if len(g) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// RunnerListHelp は runner を並べる一覧のタブ（Runners / Jobs）が ? に出すグループを返す。
//
// 一覧のキー・絞り込み中のキー・runner の操作キーを持つ画面のための組み合わせである。
// Disk / Logs / Doctor / Config / Setup は操作キーの集合が違うため、このメソッドを
// 使わず Help に自分のグループを渡す。
func (s Set) RunnerListHelp() [][]key.Binding {
	return s.Help(s.List.Bindings(), s.List.FilterBindings(), s.Runner.Order())
}

// DiskHelp は Disk タブが ? に出すグループを返す。
//
// 一覧のキー・絞り込み中のキー・Disk 固有のキーを持つ画面のための組み合わせである
// （RunnerListHelp と同じ形。載せる操作キーの集合だけが違う）。runner の操作キーは
// この画面では効かないので渡さない。
//
// 一覧のキーからは **enter を外す**。Disk タブに詳細画面は無く、page/disk の
// handleKey は enter に何もしない。List.Bindings をそのまま渡すと `enter 詳細を開く`
// が並び、押しても何も起きないキーをヘルプが案内することになる
// （screens.md の設計原則 2）。
func (s Set) DiskHelp() [][]key.Binding {
	return s.Help(s.listBindingsWithoutEnter(), s.List.FilterBindings(), s.Disk.Bindings())
}

// ConfigHelp は Config タブが ? に出すグループを返す。
//
// 一覧のキー（項目の上下と決定）だけを載せる。Config タブは runner の追加や
// サービス制御を持たず、編集の起点は決定（enter）1 つだけだからである。
// 反映方法の選択とフォームはモーダルであり、キーはそちらのフッタが出す。
func (s Set) ConfigHelp() [][]key.Binding {
	return s.Help(s.List.Bindings())
}

// DoctorHelp は Doctor タブが ? に出すグループを返す。
//
// 一覧のキー（enter を含む）と絞り込み中のキーだけを載せる。**Doctor 固有の
// キー集合は無い。** 詳細を開く enter は List.Enter、全項目の再実行 r は
// Global.Refresh そのものであり、同じキーを別の定義として重ねると同時に有効な
// Binding が 2 つになる（DiskKeys の doc と同じ理由）。
//
// enter を残すのは Disk タブと違って詳細画面があるためである（screens.md の
// Doctor タブのキーマップ）。runner の操作キーはこの画面で効かないので渡さない。
//
// 一覧のキーからは **space（選択のトグル）と ctrl+a（全選択）を外す**。Doctor の
// 区画は page/doctor/rows.go が Selectable:false を宣言しており、行を選ぶという
// 状態がそもそも無い。選んだ行に対する一括操作も無く（再実行の対象は全項目の r か
// カーソル位置の 1 項目だけ）、押しても何も起きないキーをヘルプが案内することに
// なる（screens.md の設計原則 2。DiskHelp が enter を外すのと同じ理由）。
func (s Set) DoctorHelp() [][]key.Binding {
	return s.Help(s.listBindingsWithout(s.List.Toggle, s.List.SelectAll), s.List.FilterBindings())
}

// SetupHelp は Setup タブが ? に出すグループを返す。
//
// 一覧のキー（メニューの上下と決定）と、runner に対する追加・削除・バージョン更新の
// キーを載せる。サービス制御のキーはこの画面で効かないので渡さない。
//
// enter を残すのは、Disk タブと違って Setup のメニューが enter で決まるためである
// （DiskHelp が enter を落とす理由はその画面に決定操作が無いことであって、一覧の
// キー定義そのものの性質ではない）。
func (s Set) SetupHelp() [][]key.Binding {
	return s.Help(s.List.Bindings(), s.Setup())
}

// Setup は Setup タブで有効な runner 操作のキーを返す。
//
// RunnerKeys から追加・削除・バージョン更新の 3 つだけを取り出す。専用のキー群を
// 新設しないのは、同じ操作に同じキーを割り当てるためである（Runners 一覧の n / D / u
// と Setup タブのそれが別の定義に分かれると、片方だけキーを変えても気付けない）。
func (s Set) Setup() []key.Binding {
	return []key.Binding{s.Runner.Add, s.Runner.Delete, s.Runner.Update}
}

// listBindingsWithoutEnter は enter を除いた通常モードの一覧のキーを返す。
//
// 入力中にのみ有効な Accept も enter だが、List.Bindings には含まれないので落ちない
// （FilterBindings が返す）。
func (s Set) listBindingsWithoutEnter() []key.Binding {
	return s.listBindingsWithout(s.List.Enter)
}

// listBindingsWithout は drop と同じキーを持つものを除いた通常モードの一覧のキーを返す。
//
// **List 側ではなくここに置く。** キーを外す理由は「このタブに詳細画面が無い」
// 「この一覧は行を選べない」といった画面側の事情であり、一覧のキー定義そのものの
// 性質ではない。List に専用のメソッドを生やすと、一覧のキーが画面の数だけ系統が
// あるように読める。
//
// 落とす相手は **同じキーを持つ Binding** として選ぶ。添字や説明文で選ぶと、
// List.Bindings の並びや文言を変えたときに黙って別のキーが落ちる。
func (s Set) listBindingsWithout(drop ...key.Binding) []key.Binding {
	dropped := func(b key.Binding) bool {
		return slices.ContainsFunc(drop, func(d key.Binding) bool {
			return slices.Equal(b.Keys(), d.Keys())
		})
	}

	all := s.List.Bindings()
	out := make([]key.Binding, 0, len(all))
	for _, b := range all {
		if dropped(b) {
			continue
		}
		out = append(out, b)
	}
	return out
}

// LogsHelp は Logs タブが ? に出すグループを返す。
//
// runner の操作キーは出さない。Logs タブに runner への操作は無く、出すと押しても
// 何も起きないキーをヘルプに並べることになる。一覧のキーを出すのはファイル一覧の
// ペインで有効だからで、Logs タブ固有の 3 つを別のグループにするのは、有効になる
// 状況が違うキーはグループを分けるという Help の方針に従っている。
//
// **Help を経由せず、先頭のグローバルは logsGlobal（tab を除いたもの）を渡す。**
// Help が付ける既定の先頭には Global.TabNext が入っており、この画面では効かない
// 「次のタブ」が Log.Pane の「ファイル一覧 / 本文の切り替え」と並んで tab の行を
// 2 つ作ってしまう。押しても効かないキーを出さない方針は runner の操作キーを外すのと
// 同じであり、tab だけ例外にする理由は無い。
func (s Set) LogsHelp() [][]key.Binding {
	return help(s.logsGlobal(), s.Log.Bindings(), s.List.Bindings(), s.List.FilterBindings())
}

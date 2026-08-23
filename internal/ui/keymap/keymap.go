package keymap

import (
	"slices"

	"charm.land/bubbles/v2/key"
)

// Set は 1 つの画面で参照するキー定義の集約。
type Set struct {
	Global  Global
	List    List
	Runner  RunnerKeys
	Disk    DiskKeys
	Log     LogKeys
	Confirm ConfirmKeys
}

// New はキー定義の集約を返す。
func New() Set {
	return Set{
		Global:  NewGlobal(),
		List:    NewList(),
		Runner:  NewRunnerKeys(),
		Disk:    NewDiskKeys(),
		Log:     NewLogKeys(),
		Confirm: NewConfirmKeys(),
	}
}

// Context は同時に有効になるキーの集合。キー重複検査の単位である。
//
// キーが page と親 Model で二重に解釈されないことは、「同時に有効なキーが重複しない」
// という規則だけが担保している（ui/keys.go のキー配送）。親はキーをまず page へ渡し、
// page は自分が使わないキーだけを差し戻す。page と親の両方に割り当てられたキーは
// 差し戻される前に page が消費したうえで親も動くため、1 打鍵で 2 つの操作が起きる。
// r / q / 1〜7 / tab は 1 文字の一般的なキーで衝突しやすい。
type Context struct {
	// Name は検査の失敗メッセージに出す画面・モードの名前。
	Name string
	// Fields はこのコンテキストのキーが由来する Set のフィールド名。
	//
	// 検査の網羅性はここで担保する。Set のすべてのフィールドがいずれかの Context に
	// 現れることを keymap_test.go が reflect で突き合わせるため、Set にキー集合を
	// 足して Contexts への登録を忘れるとテストが落ちる。
	Fields []string
	// Keys はこのコンテキストで同時に有効なキー。
	Keys []key.Binding
}

// Contexts は同時に有効になりうるキー集合をすべて返す。
//
// **Set にキー集合（フィールド）を足したら、ここにも足すこと。** 足さなければ、その
// キーは重複検査を一度も通らないまま親のグローバルキーと衝突しうる。二重解釈が実際に
// 起きるのは page が organism へキーを渡すと同時に親へ差し戻す経路
// （page/runners・page/jobs の handleKey の既定分岐）であり、新しいタブが自前の
// organism キー（区画の絞り込み、フォーム）を足したときに漏れる。
//
// 登録漏れを人の注意に頼らないよう、Set の全フィールドがいずれかの Context の Fields に
// 現れることをテストが強制する（TestContextsCoverEverySetField）。
func (s Set) Contexts() []Context {
	// 一覧画面の通常モード。グローバルキー・一覧のキー・runner の操作キーが
	// すべて同時に有効になる（Runners タブは孤児ユニットの区画を持つが、区画の
	// 移動は j / k の端越えなので専用のキーは無い）。
	normal := s.Global.Bindings()
	normal = append(normal, s.List.Bindings()...)
	normal = append(normal, s.Runner.Bindings()...)

	// Disk タブの通常モード。runner の操作キーは効かず、代わりに Disk 固有の
	// キーが有効になる（選択と再集計は List.Toggle / Global.Refresh を使い回す）。
	disk := s.Global.Bindings()
	disk = append(disk, s.List.Bindings()...)
	disk = append(disk, s.Disk.Bindings()...)

	// Logs タブの通常モード。runner の操作キーは効かず、代わりに Logs タブ固有の
	// 3 つが有効になる。**Global.TabNext（tab）を含めない**のは、この画面では tab が
	// ペインの切り替えだからである（LogKeys.Pane の doc）。含めると重複検査が落ちるが、
	// それは「同時に有効なキー」という前提が崩れることを正しく示している。
	//
	// 除外そのものは logsGlobal が持つ。? の一覧（LogsHelp）も同じ除外を必要とするため、
	// ここに書き並べると同じ事実が 2 箇所に散る。
	logs := s.logsGlobal()
	logs = append(logs, s.Log.Bindings()...)
	logs = append(logs, s.List.Up, s.List.Down, s.List.Top, s.List.Bottom,
		s.List.PageDown, s.List.PageUp, s.List.Filter, s.List.Enter)

	return []Context{
		{
			Name:   "一覧画面（通常モード）",
			Fields: []string{"Global", "List", "Runner"},
			Keys:   normal,
		},
		{
			Name:   "Disk タブ（通常モード）",
			Fields: []string{"Global", "List", "Disk"},
			Keys:   disk,
		},
		{
			Name:   "Logs タブ（通常モード）",
			Fields: []string{"Global", "List", "Log"},
			Keys:   logs,
		},
		{
			// 入力中はグローバルキーを解釈せず、確定・取消・中断のみが有効。
			Name:   "入力中",
			Fields: []string{"Global", "List"},
			Keys:   []key.Binding{s.List.Accept, s.List.Cancel, s.Global.Interrupt},
		},
		{
			// 確認ダイアログ表示中はモーダルの背後へキーが流れないため、
			// y / n と、キャンセルを兼ねる esc / enter、中断の ctrl+c だけが効く
			// （page/overlay.go のキー配送）。List と Global から由来するキーを
			// 混ぜているのは、esc と enter を ConfirmKeys で再定義せず
			// Global.Back / List.Accept を使い回すためである（ConfirmKeys の doc）。
			Name:   "確認ダイアログ",
			Fields: []string{"Global", "List", "Confirm"},
			Keys: []key.Binding{
				s.Confirm.Yes, s.Confirm.No,
				s.Global.Back, s.List.Accept, s.Global.Interrupt,
			},
		},
	}
}

// logsGlobal は Logs タブで実際に効くグローバルキーを返す（tab を除いたもの）。
//
// **Logs タブで tab が「次のタブ」ではない、という事実の出どころをここ 1 つにする。**
// 同時に有効なキーの集合（Contexts）と ? の全キー一覧（LogsHelp）は、どちらもこの除外を
// 必要とする。片方だけに書くと、重複検査は緑のまま ? だけが嘘をつく形が作れてしまう。
// 実際、LogsHelp が Help 経由でグローバルキーをそのまま先頭に並べていたころは、Logs タブの
// ? に tab の行が「次のタブ」と「ファイル一覧 / 本文の切り替え」の 2 つ、互いに矛盾する
// 説明で並んでいた（screens.md の「この画面では tab が次のタブではない」に反する表示）。
//
// 除外を tab というキーで判定し、Global.TabNext を名指しで飛ばす列挙にしないのは、
// Global にキーが増えたときに Logs タブだけ取りこぼさないためである。列挙で書くと
// グローバルキーを 1 つ足すたびにここも直さなければならず、直し忘れると Logs タブの
// ? からだけそのキーが消える。
func (s Set) logsGlobal() []key.Binding {
	all := s.Global.Bindings()
	out := make([]key.Binding, 0, len(all))
	for _, b := range all {
		if slices.Equal(b.Keys(), s.Global.TabNext.Keys()) {
			continue
		}
		out = append(out, b)
	}
	return out
}

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

// listBindingsWithoutEnter は enter を除いた通常モードの一覧のキーを返す。
//
// **List 側ではなくここに置く。** enter を外す理由は「このタブに詳細画面が無い」と
// いう画面側の事情であり、一覧のキー定義そのものの性質ではない。List に専用の
// メソッドを生やすと、一覧のキーが「enter 付き」と「enter 無し」の 2 系統あるように
// 読める。
//
// 落とす相手は Enter と同じキーを持つ Binding として選ぶ。添字や説明文で選ぶと、
// List.Bindings の並びや文言を変えたときに黙って別のキーが落ちる。入力中にのみ
// 有効な Accept も enter だが、List.Bindings には含まれない（FilterBindings が返す）。
func (s Set) listBindingsWithoutEnter() []key.Binding {
	all := s.List.Bindings()
	out := make([]key.Binding, 0, len(all))
	for _, b := range all {
		if slices.Equal(b.Keys(), s.List.Enter.Keys()) {
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

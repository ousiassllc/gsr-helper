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

package keymap

import "charm.land/bubbles/v2/key"

// Set は 1 つの画面で参照するキー定義の集約。
type Set struct {
	Global Global
	List   List
	Runner RunnerKeys
}

// New はキー定義の集約を返す。
func New() Set {
	return Set{
		Global: NewGlobal(),
		List:   NewList(),
		Runner: NewRunnerKeys(),
	}
}

// FullHelp は ? の全キー一覧のグループを返す。bubbles/help の FullHelpView に渡す。
//
// 全キー一覧は操作の可否を反映しない。可否は状況で変わるため、一覧の役割は
// キーと動作の対応を示すことに限る（screens.md の無効な操作の表示）。
func (s Set) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		s.Global.Bindings(),
		s.List.Bindings(),
		s.Runner.Order(),
	}
}

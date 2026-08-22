package atom

import "github.com/ousiassllc/gsr-helper/internal/ui/token"

// plainStyles は色を使わないスタイル。期待値を素の文字列で書けるようにする。
//
// パッケージ内のテストで共有するため helper_test.go に置く
// （internal/exec/helper_test.go と同じ置き方）。
func plainStyles() token.Styles {
	return token.NewStyles(true, false)
}

// colorStyles は色を使うスタイル。色があっても記号や括弧が残ることの検証に使う。
func colorStyles() token.Styles {
	return token.NewStyles(true, true)
}

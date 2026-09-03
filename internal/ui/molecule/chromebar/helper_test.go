package chromebar

import "github.com/ousiassllc/gsr-helper/internal/ui/token"

// plainStyles は色を使わないスタイル。期待値を素の文字列で書けるようにする。
func plainStyles() token.Styles {
	return token.NewStyles(true, false)
}

package molecule

import "github.com/ousiassllc/gsr-helper/internal/ui/token"

// plainStyles は色を使わないスタイル。期待値を素の文字列で書けるようにする。
// 共有するテストヘルパーは internal/exec/helper_test.go と同じくここに集める。
func plainStyles() token.Styles {
	return token.NewStyles(true, false)
}

// columnIDs は列の識別子を並びのまま返す。
func columnIDs(cols []token.Column) []string {
	ids := make([]string, 0, len(cols))
	for _, c := range cols {
		ids = append(ids, c.ID)
	}
	return ids
}

// containsID は識別子が一覧に含まれるかを返す。
func containsID(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

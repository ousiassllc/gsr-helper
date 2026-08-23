package disk

import (
	"strings"
	"testing"
)

// 実行中の件数は状態行に出さない（進捗表示と二重に出さない）。
func TestStatusLineDoesNotDuplicateProgress(t *testing.T) {
	m, _ := selected(t)
	m, _ = send(t, m, press("c"))
	m, _ = send(t, m, press("y"))

	if s := m.status(); strings.Contains(s, "クリーンアップ中") {
		t.Errorf("状態行に進捗が二重に出ている（status = %q）", s)
	}
}

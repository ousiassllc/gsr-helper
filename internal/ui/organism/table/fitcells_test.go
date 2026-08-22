package table

import (
	"slices"
	"testing"
)

// fitCells はセルの数を列数ちょうどに揃える。
//
// **内部テストにしてあるのは、詰めが View() から観測できないためである。** 足りない
// セルを空文字で埋めても、埋めずに短い行を渡しても、bubbles/table は行を幅いっぱいに
// 伸ばすので出力はバイト単位で同一になる。外側（render_test.go）から見えるのは切り
// 捨てだけで、詰めの分岐を消しても全緑だった（Issue #31）。
func TestFitCells(t *testing.T) {
	tests := map[string]struct {
		cells []string
		n     int
		want  []string
	}{
		"ちょうど":   {[]string{"a", "b"}, 2, []string{"a", "b"}},
		"多い":     {[]string{"a", "b", "c"}, 2, []string{"a", "b"}},
		"少ない":    {[]string{"a"}, 2, []string{"a", ""}},
		"1 つも無い": {nil, 2, []string{"", ""}},
		"列が無い":   {[]string{"a"}, 0, nil},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := fitCells(tt.cells, tt.n); !slices.Equal(got, tt.want) {
				t.Errorf("fitCells(%q, %d) = %q, want %q", tt.cells, tt.n, got, tt.want)
			}
		})
	}
}

package molecule

import (
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// KeyBar はフッタの 2 行を返す。1 行目はキーヒントの並び、2 行目は無効なキーの理由。
//
// 理由が無いときも 2 行目を空行として返す。理由の有無で本体の行数が変わり、
// 表示が上下に跳ねることを防ぐためである。
//
// ?:ヘルプ はこの関数が末尾に付ける。**呼び出し側の hints に含めないこと。**
// フッタは常に ?:ヘルプ を含む（screens.md の共通レイアウト）ため、全キーの
// 一覧へ辿れる入口を画面ごとの hints の作り方に委ねない。幅に収まらない場合は
// 手前のヒントから落ち、?:ヘルプ だけが残る。
func KeyBar(hints []atom.Hint, width int, s token.Styles) string {
	// ヘルプは常に有効なので理由を持たない。
	help := atom.KeyHint(atom.Hint{Key: "?", Desc: "ヘルプ", Enabled: true, Reason: ""}, s)

	parts := make([]string, 0, len(hints)+1)
	for _, h := range hints {
		parts = append(parts, atom.KeyHint(h, s))
	}
	parts = append(parts, help)

	// 収まらない分は ?:ヘルプ に集約する（有効なキーを画面から消さないため、
	// 全キーの一覧へ辿れる入口だけは必ず残す）。atom.Join は parts の末尾から
	// 落とすので、ヘルプは最後の要素として並べても overflow として再度付く。
	line := atom.Join(parts, " ", width, help)
	return line + "\n" + reasonLine(hints, width, s)
}

// reasonLine は無効なキーの理由を 1 行にまとめる。
//
// 同じ理由のキーは "s/x/X: root 権限が必要です" のようにまとめる。理由ごとに
// 1 行ずつ出すとフッタの高さが状況で変わるため、1 行に収める。
func reasonLine(hints []atom.Hint, width int, s token.Styles) string {
	order := make([]string, 0, len(hints))
	keys := make(map[string][]string, len(hints))
	for _, h := range hints {
		if h.Enabled || h.Reason == "" {
			continue
		}
		if _, ok := keys[h.Reason]; !ok {
			order = append(order, h.Reason)
		}
		keys[h.Reason] = append(keys[h.Reason], h.Key)
	}
	if len(order) == 0 {
		return ""
	}

	groups := make([]string, 0, len(order))
	for _, reason := range order {
		groups = append(groups, strings.Join(keys[reason], "/")+": "+reason)
	}
	return s.Muted.Render(atom.Truncate(strings.Join(groups, "  "), width))
}

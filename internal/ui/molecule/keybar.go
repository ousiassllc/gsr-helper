package molecule

import (
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// KeyBar はフッタの 2 行を返す。1 行目はキーヒントの並び、2 行目は無効なキーの理由。
//
// **返す行数は常に 2 行である。** 理由が無いときも 2 行目を空行として返し、理由が
// 幾つあっても 2 行目に収める（reasonLine）。行数が変わると本体の高さが動いて
// 表示が上下に跳ねる（template.ChromeHeight はフッタを 2 行で固定している）。
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
// 同じ理由のキーは 1 つの組にまとめ、"(s)(x): 理由" の形で理由の数だけ並べて
// 空白 2 つで区切る（screens.md の共通レイアウトのフッタ 2 行目
// "(s)(x)(X)(R)(D)(n)(u): root 権限が必要です…"）。キーを丸括弧で囲むのは、
// 1 行目がグレーアウトだけで無効を示す（幅を増やせない）ため、色を使えない端末で
// 有効・無効を読み分ける手がかりをこの行が担うからである。
//
// **理由ごとに行を増やさず、必ず 1 行にする。** フッタは 2 行に固定されており
// （template.ChromeHeight）、増やすと本体の高さが理由の数で動いて表示が跳ねる。
// 収まらない分は末尾を中略する。件数（"他 N 件"）にまとめて捨てないのは、
// まとめた理由がフッタからは一切辿れなくなるためである。
//
// 並べる順は、最も多くのキーを塞いでいる理由を先頭にし、残りは出現順にする。
// 中略で削られるのは末尾からなので、最も広く当てはまる理由（root / systemd /
// 認証のような能力不足は複数のキーに一斉に効く）が最後まで残る。
func reasonLine(hints []atom.Hint, width int, s token.Styles) string {
	order, keys := groupReasons(hints)
	if len(order) == 0 {
		return ""
	}

	groups := make([]string, 0, len(order))
	for _, reason := range widestReasonFirst(order, keys) {
		groups = append(groups, parenKeys(keys[reason])+": "+reason)
	}
	return s.Muted.Render(atom.Truncate(strings.Join(groups, "  "), width))
}

// groupReasons は無効なキーを理由ごとにまとめ、理由の出現順とキーの一覧を返す。
func groupReasons(hints []atom.Hint) (order []string, keys map[string][]string) {
	order = make([]string, 0, len(hints))
	keys = make(map[string][]string, len(hints))
	for _, h := range hints {
		if h.Enabled || h.Reason == "" {
			continue
		}
		if _, ok := keys[h.Reason]; !ok {
			order = append(order, h.Reason)
		}
		keys[h.Reason] = append(keys[h.Reason], h.Key)
	}
	return order, keys
}

// widestReasonFirst は最も多くのキーを塞いでいる理由を先頭に、残りを出現順に並べる。
// 同数なら先に現れたものを先頭に採る。
func widestReasonFirst(order []string, keys map[string][]string) []string {
	top := order[0]
	for _, reason := range order[1:] {
		if len(keys[reason]) > len(keys[top]) {
			top = reason
		}
	}

	sorted := make([]string, 0, len(order))
	sorted = append(sorted, top)
	for _, reason := range order {
		if reason != top {
			sorted = append(sorted, reason)
		}
	}
	return sorted
}

// parenKeys はキーを丸括弧で囲んで並べる（"(s)(x)(X)"）。
func parenKeys(ks []string) string {
	var b strings.Builder
	for _, k := range ks {
		b.WriteString("(" + k + ")")
	}
	return b.String()
}

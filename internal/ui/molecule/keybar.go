package molecule

import (
	"strconv"
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
// キーは丸括弧で囲む。1 行目はグレーアウトだけで無効を示す（幅を増やせない）ため、
// 色を使えない端末で有効・無効を読み分ける手がかりはこの行が担う
// （screens.md の共通レイアウトのフッタ 2 行目 "(s)(x)(X)(R)(D)(n)(u): root 権限が…"）。
//
// 理由が複数あるときは 1 つだけを出し、残りは件数にまとめる。2 つ並べると幅 80 で
// 後ろの理由が中略され、対処の書かれた部分が読めなくなるためである。残す理由は
// 最も多くのキーを塞いでいるものにする。能力不足（root / systemd / 認証）は複数の
// キーに一斉に効くため、この規則では未対応のような個別の理由より優先される。
func reasonLine(hints []atom.Hint, width int, s token.Styles) string {
	order, keys := groupReasons(hints)
	if len(order) == 0 {
		return ""
	}

	top := widestReason(order, keys)
	line := parenKeys(keys[top]) + ": " + top
	if rest := len(order) - 1; rest > 0 {
		line += "  " + token.IconWarn + " 他 " + strconv.Itoa(rest) + " 件"
	}
	return s.Muted.Render(atom.Truncate(line, width))
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

// widestReason は最も多くのキーを塞いでいる理由を返す。同数なら先に現れたものを採る。
func widestReason(order []string, keys map[string][]string) string {
	top := order[0]
	for _, reason := range order[1:] {
		if len(keys[reason]) > len(keys[top]) {
			top = reason
		}
	}
	return top
}

// parenKeys はキーを丸括弧で囲んで並べる（"(s)(x)(X)"）。
func parenKeys(ks []string) string {
	var b strings.Builder
	for _, k := range ks {
		b.WriteString("(" + k + ")")
	}
	return b.String()
}

// Package report は Setup タブの結果報告（FR-15）の行を組み立てる。
//
// page/setup から分けたのは、**結果（setup.Result）とエラーだけを見て文字列の並びを
// 返す純粋関数**であり、Model もキー入力も bubbletea も要らないためである。外部
// テストから直に確かめられる形になり、page/setup の内部テストへ置く必要も無くなる。
// 1 ディレクトリ 2000 行の上限に対して page/setup が残り 34 行になったのを機に分けた
// （docs/ui/atomic-design.md の行数の予算）。
package report

import (
	"errors"
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/gh"
	"github.com/ousiassllc/gsr-helper/internal/setup"
)

// Lines は結果報告の行を組み立てる。
//
// 何台目までが成功し、どこで何が失敗したかを示す（FR-15）。成功分は残っている
// ことも明記する——失敗を見た利用者が、途中まで作った runner を手で消すべきか
// 判断できるようにするためである。
func Lines(res setup.Result, err error) []string {
	if err == nil {
		return []string{"完了: " + strconv.Itoa(len(res.Succeeded)) + " 台"}
	}

	out := make([]string, 0, 4)
	if res.Failed != "" {
		out = append(out, "✗ "+res.Failed+" の"+res.Phase+"で失敗しました")
	}
	out = append(out, errorLines(err)...)

	// 並びは screens.md の進捗のモックに合わせる（完了 → 未実行 → 残る旨）。
	if n := len(res.Succeeded); n > 0 {
		out = append(out, "完了: "+strconv.Itoa(n)+" 台（"+strings.Join(res.Succeeded, ", ")+"）")
	}
	if n := len(res.Remaining); n > 0 {
		out = append(out, "未実行: "+strconv.Itoa(n)+" 台（"+strings.Join(res.Remaining, ", ")+"）")
	}
	if len(res.Succeeded) > 0 {
		out = append(out, strings.Join(res.Succeeded, ", ")+" はそのまま残っています。")
	}
	return out
}

// errorLines は失敗した理由の行を組み立てる。
//
// **API の失敗は Hint を独立した行にする。** 結果報告は 1 行ずつ幅で切り詰めて描く
// （page/setup の reportView）ため、Hint を末尾へ繋いだ 1 行を渡すと、不足している
// スコープと `gh auth refresh` のコマンド例だけがちょうど落ちる。403 で利用者が
// 最も知りたいのはそこである（org の tarball 取得が
// `[HTTP 403]: GET https://api.gith…` で切れ、理由が読めない実例があった）。
//
// 1 行目は Hint を外した文言にする。gh.APIError.Error() が Hint を「。」で末尾へ
// 繋ぐ形（同型の doc）に依拠しており、繋ぎ方が変われば
// TestLinesPutAPIHintOnItsOwnLine が落ちる。
func errorLines(err error) []string {
	line := firstLine(err.Error())
	var apiErr *gh.APIError
	if errors.As(err, &apiErr) && apiErr.Hint != "" {
		return []string{"  " + strings.TrimSuffix(line, "。"+apiErr.Hint), "  → " + apiErr.Hint}
	}
	return []string{"  " + line}
}

// firstLine は文字列の 1 行目を返す。
//
// 報告の 1 項目は 1 行に収める。複数行のエラーをそのまま入れると、行の並びが
// 崩れて「どこで止まったか」が読めなくなる。
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

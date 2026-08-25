package linebudget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/buildconfig/buildconfigtest"
)

// budgetProseDoc は「散文が行数の実測値を数値で持たない」検査の対象文書である。
//
// **除外の一覧を文書ごとに持つのは、除外が文書の構造に結びついているからである。**
// `atomic-design.md` は行数表を 2 つ持ち、過去の値を記録する節を 20 以上抱えるが、
// `overview.md` は行数表を持たず、除外も 1 つも持たない。一覧を 1 つに混ぜると、
// **片方にしか無い見出しが消えたことを検査が知らせられなくなる**——
// skipExcludedSections は一覧に挙げた見出しが本文に無いと `t.Fatal` するので、
// 混ぜた一覧では「もう一方の文書にある見出し」を毎回取り逃がすことになり、
// 見出しの改名で除外が黙って外れる（あるいは効かなくなる）のを止められない。
//
// tables を数で持つのも同じ理由である。**表の数は文書ごとに違うのに、
// 「見つからなければ落ちる」保証は残したい**——`atomic-design.md` は 2 つ無ければ
// 除外が壊れており、`overview.md` は 0 なので表を探さない（見つかったら落ちる）。
type budgetProseDoc struct {
	rel      []string // リポジトリルートからのパス要素
	name     string   // エラーメッセージに出す名前
	tables   int      // 本文が持つ行数表の数（0 は表を持たない文書）
	excluded []string // 過去の値を記録する節の見出し（明示の一覧）
}

// budgetProseDocs は検査の対象文書である。
//
// **corpus が 1 ファイルだと、同じ形の写しが他の文書で無検査のまま残る。**
// `overview.md` の `internal/runner` 系の使用量は改訂 1.19 / 1.39 で 2 度陳腐化して
// 人手で追随した記録があり、PR #168 が入れた検査は `atomic-design.md` しか
// 読んでいなかったためそこを見ていなかった（Issue #169）。
var budgetProseDocs = []budgetProseDoc{
	{
		rel:      []string{"docs", "ui", "atomic-design.md"},
		name:     "atomic-design.md",
		tables:   2,
		excluded: budgetProseExcluded,
	},
	{
		// **`overview.md` は除外を 1 つも持たない。** 唯一あった「### `internal/disk`」は
		// 節まるごと（API 責務表と全段落）を外しながら「載せてよいのは過去の値だけを持つ
		// 節」という規約を満たしていなかった。当時の値は `atomic-design.md` の
		// 「`internal/disk` から `pathguard` を切り出した判断（Issue #101）」が持つので、
		// `overview.md` 側は数値を落として同節への参照へ寄せた。
		rel:    []string{"docs", "components", "overview.md"},
		name:   "components/overview.md",
		tables: 0,
	},
}

// read は対象文書の本文を返す。
func (d budgetProseDoc) read(t *testing.T) string {
	t.Helper()

	path := filepath.Join(append([]string{buildconfigtest.RepoRoot(t)}, d.rel...)...)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s を読めない: %v", d.name, err)
	}
	return string(body)
}

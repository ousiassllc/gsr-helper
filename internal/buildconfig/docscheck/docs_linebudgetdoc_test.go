package docscheck

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
// `overview.md` は行数表を持たず、記録の節は 1 つだけである。一覧を 1 つに混ぜると、
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
		rel:      []string{"docs", "components", "overview.md"},
		name:     "components/overview.md",
		tables:   0,
		excluded: overviewProseExcluded,
	},
}

// overviewProseExcluded は `docs/components/overview.md` で検査から外す節の見出しである
// （見出し行そのままの綴り）。
//
// この 1 件だけなのは、`internal/disk` の節が **Issue #101 当時の値**——警告帯
// （2000 行超）へ入り、`disk/pathguard` を切り出して戻した経緯——を記録しているからである。
// その Issue に紐づく過去の事実なので、現在の実測へ寄せると議論の根拠そのものが消える。
// **同じ節が持っていた現在形の実測値のほうは `atomic-design.md` の行数表への参照へ寄せた**
// ので、除外の中に残るのは当時の値だけである（Issue #169）。
var overviewProseExcluded = []string{
	"### `internal/disk`",
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

package cleanview

import (
	"strconv"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule"
)

// クリーンアップの逐次表示と結果報告を organism/pane.ProgressList へ流す（Issue #75）。
//
// **バーを描くのは全体件数が事前に確定しているからである**（atomic-design.md の
// 「bubbles/progress を使う範囲」）。件数は確認を通した計画（disk.CleanPlan）の
// 時点で決まっており、Total に入れれば ProgressList がバーを出す。
//
// 状態行に件数のテキストを重ねて出さない。同じ進捗が 2 か所に出ると、片方だけが
// 古い値になったときにどちらが正しいのか読み手に判断できない。

// ProgressTitle は進捗表示の見出し。件数は ProgressList が Done/Total から自分で
// 添えるので、ここには入れない（page/setup の bareTitle と同じ約束）。
const ProgressTitle = "クリーンアップ中…"

// DockerLabel は docker の削除の進捗に出る表示名。
//
// internal/disk が進捗に載せる表示名と同じ文字列にしてある。突き合わせを名前で
// 行うため、別の文言を作ると docker の行だけ完了しても状態が変わらない。
const DockerLabel = "docker / 未使用リソース"

// Rows は計画の対象を未着手の行として並べる。
//
// 並びは disk.Apply が処理する順（パス → docker）に合わせる。実行順と表示順が
// 食い違うと、上の行が終わる前に下の行が完了する見た目になる。
//
// 名前に使うのは進捗（disk.Progress.Label）と同じ文字列である。突き合わせを
// 名前で行うため、ここで別の文言を作ると完了しても行の状態が変わらない。
func Rows(plan disk.CleanPlan) []molecule.ProgressView {
	rows := make([]molecule.ProgressView, 0, len(plan.Paths)+1)
	for _, t := range plan.Paths {
		rows = append(rows, molecule.ProgressView{
			Name: t.Label, State: molecule.ProgressWaiting, Detail: atom.Bytes(t.Bytes),
		})
	}
	if plan.Docker {
		// disk.Apply が docker の進捗に載せる表示名は dockerPruneLabel と同じ
		// 文字列である（scan.go の doc）。名前で突き合わせるので、ここで別の
		// 文言を作ると docker の行だけ完了しても状態が変わらない。
		rows = append(rows, molecule.ProgressView{
			Name: DockerLabel, State: molecule.ProgressWaiting, Detail: "",
		})
	}
	return rows
}

// Mark は 1 対象の結果を行へ反映する。
//
// 見つからない名前は黙って捨てる。進捗の名前は internal/disk が付けるので、
// 表示側が知らない対象が届くのは実装の食い違いだが、落とすより静かに無視する
// ほうが害が小さい（削除そのものは進んでいる）。
func Mark(rows []molecule.ProgressView, p disk.Progress) {
	for i := range rows {
		if rows[i].Name != p.Label {
			continue
		}
		if p.Err != nil {
			rows[i].State, rows[i].Detail = molecule.ProgressFailed, FirstLine(p.Err.Error())
			return
		}
		rows[i].State = molecule.ProgressDone
		return
	}
}

// Report は完了後の結果報告を組み立てる（FR-15 の結果報告）。
//
// 成功・失敗・未実行を書き分けるのは、1 件の失敗で残りを止めない disk.Apply の
// 契約（apply.go）のもとでは「何件消えて何件残ったか」が利用者の次の判断を決める
// ためである。未実行が出るのは打ち切られた場合（ctx のキャンセル）だけである。
//
// **件数は行の状態から数える。** 実行側が返す失敗件数を別に受け取らないのは、
// 進捗の到着順と終了通知の到着順が決まっていないためで、2 つの数え方を持つと
// 「最後の進捗より先に終了が届いた回だけ 1 件ずれる」形の食い違いが起きる。
func Report(rows []molecule.ProgressView, bytes int64, err error) []string {
	done, failedRows, pending := 0, 0, 0
	for _, r := range rows {
		switch r.State {
		case molecule.ProgressDone:
			done++
		case molecule.ProgressFailed:
			failedRows++
		case molecule.ProgressWaiting, molecule.ProgressRunning:
			pending++
		}
	}

	out := []string{"成功: " + strconv.Itoa(done) + " 件"}
	if failedRows > 0 {
		out = append(out, "失敗: "+strconv.Itoa(failedRows)+" 件")
	}
	if pending > 0 {
		out = append(out, "未実行: "+strconv.Itoa(pending)+" 件")
	}
	// 解放量は全件成功したときだけ出す。失敗があると、消せなかった対象のぶんを
	// 含んだ見込み値になる（cleanNotice と同じ理由）。
	if failedRows == 0 && pending == 0 && err == nil {
		out = append(out, "解放: "+atom.Bytes(bytes))
	}
	if err != nil && failedRows == 0 {
		// 対象ごとの失敗として現れなかったエラー（打ち切りなど）。
		out = append(out, FirstLine(err.Error()))
	}
	return out
}

// FirstLine は 1 行目だけを返す。続きがあることは中略記号で示す。
//
// disk.Apply は errors.Join で失敗を束ねる（改行区切り）ため、そのまま 1 行の領域へ
// 流すと枠が崩れる。
func FirstLine(s string) string {
	head, rest, found := strings.Cut(s, "\n")
	if found && rest != "" {
		return head + " …"
	}
	return head
}

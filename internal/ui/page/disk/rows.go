package disk

import (
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/disk"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule/listrow"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/table"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// sectionTargets は削除候補の区画の添字。Disk タブは 1 区画だけを持つ。
//
// runner ごとに区画を分けないのは、クリーンアップが runner をまたいで選ぶ操作だから
// である（screens.md の Disk タブは 1 枚の表に runner 名込みの TARGET を並べる）。
// 区画を分けると、区画をまたぐ選択が「全選択」で何を選ぶのかを毎回説明する羽目になる。
const sectionTargets = 0

// dockerIDPrefix は docker の行の識別子に付ける接頭辞。
//
// docker の対象はパスを持たないため、識別子をパス以外から作る必要がある
// （identifier の重複は選択集合のキーの衝突になる）。
const dockerIDPrefix = "docker:"

// row は Disk タブの 1 行。集計結果 1 件をそのまま持つ。
//
// 表示用に落とした値ではなく disk.Usage を持つのは、選択された行をそのまま
// disk.Target へ変換して PlanClean に渡すためである（clean.go の cleanTargets）。
// 表示用の構造体まで落としてから戻すと、削除の基準ディレクトリ（Base）のような
// 画面に出ない値を持ち回れず、確認を経ない別経路で組み直すことになる。
type row struct {
	usage disk.Usage
}

// newTable は Disk タブの一覧を組み立てる。
func newTable(keys keymap.Set, s token.Styles) table.Model[row] {
	return table.New(keys.List, s, table.SectionInput[row]{
		Title:   "",
		Columns: token.DiskColumns(),
		Rules:   token.DiskColumnRules(),
		Render:  renderTarget,
		ID:      rowID,
		Match:   matchTarget,
		// 行ごとに選択の可否が分かれる唯一の一覧である（FR-31）。区画ごとの
		// Selectable では表せないため Disabled を持つ。
		Disabled:   rowDisabled,
		Selectable: true,
	})
}

// rowID は行の識別子を返す。
//
// **識別子が安定していないと SetItems のたびに選択が落ちる**（table.SetItems の doc）。
// 集計は判明順に届き、そのたびに区画の行を丸ごと差し替えるため、この一覧は 1 回の
// 集計の中で何度も SetItems を通る。
//
// ファイルの対象はパスを使う。パスは集計から削除まで一貫して同じ値であり、runner 名や
// 表示名と違って重複しない。docker の対象はパスを持たないので、表示名から作る
// （DockerUsage の表示名は docker の Type から機械的に決まるため、再集計しても
// 同じ文字列になる）。
func rowID(r row) string {
	if r.usage.Path != "" {
		return r.usage.Path
	}
	return dockerIDPrefix + r.usage.Label
}

// rowDisabled は行を選択できないかと、その理由を返す（FR-31）。
//
// 判定そのものは持たない。ジョブ実行中の _work を選ばせないのは internal/disk の
// 責務（Scan が Removable と Reason を埋める）であり、docker の行の可否は集計の時点で
// page が決めている（scan.go）。ここで条件を書き直すと、判定が 2 か所に分かれて
// 片方だけが直る。
func rowDisabled(r row) (reason string, disabled bool) {
	if r.usage.Removable {
		return "", false
	}
	return r.usage.Reason, true
}

// renderTarget は削除候補の行をセル列に変換する。
func renderTarget(in table.RowInput[row]) []string {
	return listrow.DiskTargetRow(targetView(in.Item.usage), in.Cols, in.Styles)
}

// matchTarget は絞り込みの一致判定。表示名を対象にする。
//
// 大文字小文字を無視するのは、runner 名が build01-1 のように小文字で、リポジトリ名が
// 大文字を含みうるためである（jobs/rows.go と同じ扱い）。パスを対象に含めないのは、
// パスが表示名を必ず含む（runner ディレクトリ + _work/<名前>）ため、含めても一致する
// 集合が変わらないからである。
func matchTarget(r row, q string) bool {
	return strings.Contains(strings.ToLower(r.usage.Label), strings.ToLower(q))
}

// targetView は集計結果を表示用の構造体に落とす。
//
// Scanning を常に偽にするのは、行が表に載るのは集計が終わった対象だけだからである
// （判明した対象から順に 1 件ずつ SetItems する。scan.go の waitUsage）。まだ判明して
// いない対象は行そのものが無く、「集計中の行」という状態は存在しない。表が空の間に
// 集計中であることを伝えるのは View の空文言の役割である。
//
// 失敗した対象は容量とファイル数を負にして「値なし」に落とす。集計に失敗した対象の
// Bytes は 0 のままだが、0 は「空ディレクトリ」という有効値であり（disk.Usage の doc）、
// そのまま出すと消しても何も空かない対象が 0B の候補として並ぶ。
func targetView(u disk.Usage) listrow.DiskTargetView {
	v := listrow.DiskTargetView{
		Target:   u.Label,
		Bytes:    u.Bytes,
		Files:    u.Files,
		Path:     u.Path,
		Scanning: false,
		Reason:   u.Reason,
		Failed:   u.Err != nil,
	}
	if u.Err != nil {
		v.Bytes, v.Files = -1, -1
	}
	return v
}

// usageRows は集計結果を行に変換する。並びは判明順（FR-28）。
//
// 容量順に並べ替えない。並べ替えると、判明するたびに行が入れ替わって選びかけの
// 対象を目で追えなくなる（集計は非同期で、選択と並行して進む）。
func usageRows(seen []disk.Usage) []row {
	out := make([]row, 0, len(seen))
	for _, u := range seen {
		out = append(out, row{usage: u})
	}
	return out
}

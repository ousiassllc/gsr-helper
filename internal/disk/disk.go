// Package disk は runner のディスク使用量の集計とクリーンアップを担う。
//
// 集計と削除を 1 つのパッケージに置いているのは、削除できる対象が集計で見えた対象と
// 同じものに限られるという不変条件を 1 か所で守るためである。削除パスの検証
// （ValidatePath）はこのパッケージの外に出さず、PlanClean と Apply の両方から必ず
// 通す構造にしてある。検証を通らないパスは計画にも載らず、計画に載っていても実行
// 直前にもう一度検証される（docs/architecture/security.md「削除パスの検証を必須にする」）。
//
// 外部プロセスの実行は internal/exec の Executor 経由に限る。docker の使用量取得と
// 未使用リソースの削除だけが外部コマンドで、ディレクトリの走査と削除は外部の du /
// rm を使わず自前で行う（進捗を出せるようにするため。
// docs/api/external-interfaces.md「その他のシステムコマンド」）。
package disk

// Kind は集計対象の種類。UI が行の並べ替えとアイコンの出し分けに使うため、
// 表示名（Label）とは別に機械可読な種別を持たせている。
type Kind int

// Kind の取り得る値。
const (
	KindWork   Kind = iota // _work/<リポジトリ>
	KindTool               // _work/_tool
	KindTemp               // _work/_temp
	KindDiag               // _diag
	KindDocker             // docker の使用量
)

// Usage は集計対象 1 件の使用量。Scan が判明順に送出する。
//
// 失敗を戻り値ではなく Err として 1 行に載せるのは、1 つの対象の走査失敗で
// runner 全体の集計を落とさないためである（権限不足のディレクトリが 1 つあっても
// 残りの内訳は表示できる）。
type Usage struct {
	Kind Kind
	// Runner は runner 名。docker の行では空。
	Runner string
	// Base は削除パス検証の基準（runner ディレクトリ）。docker の行では空。
	Base string
	// Path は集計・削除の対象パス。docker の行では空。
	Path string
	// Label は表示名（"build01-1 / _work/bar"、"docker / ビルドキャッシュ"）。
	Label string
	Bytes int64
	// Files はファイル数。docker では数えられないため -1（不明）。
	// 0 は「空ディレクトリ」という有効値なので、不明と兼用しない。
	Files int64
	// Removable はクリーンアップ対象として選べるか。
	Removable bool
	// Reason は選べない理由（ジョブ実行中など。FR-31）。選べる場合は空。
	Reason string
	// Err は集計に失敗した理由。nil なら成功。
	Err error
}

// Stats は 1 つのファイルシステムの容量と inode の残量（FR-29）。
//
// inode を容量と並べて持つのは、小さいファイルを大量に作る runner では容量より
// 先に inode が枯渇し、容量だけを見ていると原因が分からなくなるためである。
type Stats struct {
	Path        string
	TotalBytes  int64
	UsedBytes   int64
	AvailBytes  int64
	TotalInodes int64
	UsedInodes  int64
	FreeInodes  int64
}

// UsedPercent は容量の使用率を返す。分母が 0 なら 0。
//
// 分母を TotalBytes ではなく「使用 + 利用可能」にしているのは df に合わせるためである。
// ext4 などは総容量の数 % を root 用に予約しており、予約分を分母に含めると df の
// 表示と数 % ずれる。利用者は df と突き合わせて見るため、ずれる側を選ばない。
func (s Stats) UsedPercent() int { return percent(s.UsedBytes, s.UsedBytes+s.AvailBytes) }

// InodePercent は inode の使用率を返す。分母が 0 なら 0。
// inode には予約の概念が無いため、使用 + 空き（= 総数）をそのまま分母にする。
func (s Stats) InodePercent() int { return percent(s.UsedInodes, s.UsedInodes+s.FreeInodes) }

// percent は used/total の百分率を返す。0 除算とマイナスを避けるための共通処理。
func percent(used, total int64) int {
	if total <= 0 || used <= 0 {
		return 0
	}
	return int(used * 100 / total)
}

// DockerItem は docker system df の 1 行。
type DockerItem struct {
	// Type は docker が返す種別（"Images" / "Containers" / "Local Volumes" / "Build Cache"）。
	Type string
	// Label は表示名（"docker / イメージ" など）。
	Label string
	Size  int64
	// Reclaimable は prune で解放できる見込みの容量。
	Reclaimable int64
}

// Target はクリーンアップ対象 1 件。UI が Usage から組み立てて PlanClean に渡す。
//
// Usage をそのまま渡さないのは、選択された対象だけを表す型を分けることで
// 「集計しただけの行」が削除計画に紛れ込まないようにするためである。
type Target struct {
	Label string
	// Runner は runner 名。監査ログの runner に載せる。Docker が真なら空。
	//
	// Base（runner ディレクトリ）から filepath.Base で導出しない。runner 名は
	// Runner.Name()（.runner の AgentName 優先）であり、ディレクトリ名と一致する
	// 保証が無いためである。値として別に運ぶことで、ディレクトリ命名規則が
	// 変わっても監査ログの runner が黙って崩れないようにする。
	Runner string
	// Base は runner ディレクトリ。Docker が真なら空。
	Base string
	// Path は削除するパス。Docker が真なら空。
	Path  string
	Bytes int64
	Files int64
	// Docker は docker の未使用リソース（docker system prune -f）か。
	Docker bool
	// Protected は削除してはならない理由。空なら削除してよい。
	//
	// Usage.Removable / Reason をそのまま引き継ぐために持つ。ここで落とすと、
	// ジョブ実行中の保護（FR-31）が表示層だけの約束になり、Target を直接組む
	// 呼び出しが 1 つ増えた時点で黙って外れる。security.md「ジョブ実行中の
	// 操作をガードする」は保護を構造として強制すると宣言しているため、可否は
	// 境界の型で運び、PlanClean と Apply の両方で見る。
	Protected string
}

// CleanPlan は削除計画（ドライラン）。PlanClean だけが組み立てる。
//
// 確認画面に出す内容をこの型に固定することで、確認を経ない破壊的経路を作らない
// （docs/architecture/security.md「確認を経ない破壊的経路を作らない」）。
type CleanPlan struct {
	// Paths は検証済みの削除パス。
	Paths []Target
	// Docker は docker system prune -f を実行するか。
	Docker bool
	// Bytes は解放見込み容量。
	Bytes int64
	// Commands は実行する外部コマンドの全文（確認画面に出す）。
	// ファイル削除は外部コマンドを使わないためここには載らず、Paths として別に示す。
	Commands [][]string
}

// Progress は Apply の進捗 1 件。対象 1 件につき 1 回通知する。
type Progress struct {
	Label string
	// Done は完了した対象の数（この通知を含む）。
	Done int
	// Total は対象の総数。
	Total int
	// Err はこの対象の削除に失敗した理由。nil なら成功。
	Err error
}

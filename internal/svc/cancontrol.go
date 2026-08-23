// Package svc は systemd 経由のサービス制御とドレイン停止を担う。
//
// 操作の可否（CanControl）を UI ではなくここに置くのは、同じ判定がフッタ・詳細画面の
// 操作リスト・確認ダイアログの 3 箇所に分かれて食い違うのを防ぐためである
// （docs/components/overview.md の `internal/svc`）。UI は受け取った可否と理由を
// 描くだけにする。
//
// 外部プロセスは internal/exec の Executor 経由でのみ起動し、os/exec は直接使わない
// （docs/architecture/overview.md の「依存の規則」）。タイムアウト・監査ログ・
// トークンマスクの適用漏れを構造的に防ぐためである。
package svc

import (
	"slices"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
)

// Op は systemd 経由のサービス制御操作。
//
// 可否の判定を「押したキー」ではなく操作そのもので引くために置く。キーは keymap で
// 差し替えられるため、キー文字列で表を引くと差し替えたときに判定がコンパイルエラーも
// 無く別の操作へ移る（または消える）。UI 側の識別子（ui/page/action.ID）との対応付けは
// UI 側が持ち、svc は keymap も action.ID も知らない。
type Op int

// Op の取り得る値。docs/ui/screens.md の「Runners タブの操作」のうち、systemd 経由で
// サービスに作用する 6 つに対応する。
const (
	OpStart   Op = iota // 開始（s）
	OpStop              // 停止（x）
	OpKill              // 強制停止（X）
	OpDrain             // ドレイン停止（d）
	OpRestart           // 再起動（R）
	OpEnable            // enable / disable の切替（E）
)

// 操作できない理由の文言。docs/ui/screens.md の「無効な操作の表示」の表と一致させる。
//
// エクスポートしているのは、フッタ（atom.KeyHint）と詳細画面の操作リスト
// （molecule.ActionRow）が同じ文字列を出すためである。表示側が文言を持つと、同じ理由が
// 複数箇所に分かれて片方だけが直る。理由の出どころはこの 1 箇所に限る。
const (
	// ReasonRoot は root 権限が要る操作を非 root で試みた場合の理由。
	ReasonRoot = "root 権限が必要です（sudo で起動してください）"
	// ReasonSystemd は systemctl が無い環境の理由。
	ReasonSystemd = "サービス制御は利用できません（systemctl が見つかりません）"
	// ReasonStandalone は run.sh 直起動の runner に systemd 操作を求めた場合の理由。
	ReasonStandalone = "systemd 管理外のため操作できません"
	// ReasonManagedUnknown は systemd の管理状態そのものが判定できない場合の理由。
	// 「管理外」と言い切れないことと、その原因（ユニット一覧が取れなかった）を示す。
	ReasonManagedUnknown = "systemd の管理状態が判定できないため操作できません（ユニット一覧を取得できませんでした）"
	// ReasonNoCommand は発行するコマンドを 1 本も組めない場合の理由。
	//
	// CanControl の 4 段は「起動方式と能力」だけを見るため、それを通っても対象が
	// 空（PID もユニット名も無い）の runner が残りうる。未稼働かつサービス未
	// インストールの runner がこれに当たる（CommandLine が空の並びを返す）。
	ReasonNoCommand = "操作の対象となるプロセスもユニットもありません"
)

// CanControl は操作の可否と不可の理由を返す。可能なら理由は空文字である。
//
// 判定は docs/ui/screens.md「無効な操作の表示」の表の順で行い、**最初に一致した理由を
// 返す**。順を固定するのは、能力の問題（root / systemd）を後段の理由で隠さないため
// である。「systemd 管理外です」とだけ出ると、sudo で起動し直せば使えるのかどうかが
// 読み取れない。
//
// 各段で塞ぐ操作が異なる点に注意すること。
//
//   - root（1 段目）が要るのは systemd のサービスに直接作用する 4 つだけである。
//     ドレイン停止は待機の開始にすぎず、enable / disable の切替はユニットファイルの
//     状態を読み替えるだけなので、この段では塞がない（表の 1 行目に d と E が無い）。
//   - systemd の不在（2 段目）は 6 つすべてを塞ぐ。systemctl が無ければ起動方式に
//     かかわらずサービス制御ができないため、run.sh 直起動（3 段目）より先に見る。
//   - run.sh 直起動（3 段目）と管理状態が判定できない（4 段目）は、**同じ 5 つ**
//     （開始・停止・ドレイン停止・再起動・enable の切替）を塞ぐ。どちらも
//     「systemctl を安全に駆動できない」点で同じ状況であり、違うのは理由が
//     「systemd 管理外だと判明している」のか「そもそも判定できない」のかだけである。
//     文言が 2 つに分かれているのはその差を利用者に伝えるためで、塞ぐ範囲を分ける
//     根拠にはならない。UI が「systemd 管理外」「管理状態が不明」と分類した runner に
//     対し、systemctl でユニットの状態を書き換える操作は 1 つも通さない（FR-09）。
//   - どちらの段でも通すのは強制停止（X）だけである。worker のプロセスへ直接シグナル
//     を送る操作で、systemctl の可否に依存しないためである。ここを塞ぐと run.sh
//     直起動・判定不能の runner を止める手段が UI から無くなる。
//   - **ドレイン停止（d）を通さない。** 待機そのものは /proc の走査だけだが、Worker
//     が消えた後に発行するのは systemctl stop（Drainer.Drain）であり、同じ最終動作の
//     停止（x）が塞がれているのに d だけが待たせた末に同じ systemctl stop を発行する
//     のは筋が通らない。とくに Worker が 0 件なら初回走査で即 stop へ抜けるため、
//     アイドルな runner では確認も猶予も無いまま停止が走る。
//
// 4 段目について「ユニット一覧を取得できなかっただけで、ユニット名は `<dir>/.service`
// から読めるため停止は成立しうる」という理由付けは採らない。同じ理屈は停止（x）にも
// そのまま当てはまり、x を塞ぐ以上 d を通す根拠にならないためである。
//
// スコープ不足（表の 6 行目）はここに無い。保有スコープの判定には GitHub API が必要で、
// appconfig.Caps はその情報を持たないためである。GitHub API を持ち込む Issue が足す。
func CanControl(op Op, r runner.Runner, caps appconfig.Caps) (bool, string) {
	switch {
	case !caps.Root && isOp(op, OpStart, OpStop, OpKill, OpRestart):
		return false, ReasonRoot
	case !caps.Systemd && isOp(op, OpStart, OpStop, OpKill, OpDrain, OpRestart, OpEnable):
		return false, ReasonSystemd
	case r.Managed == runner.ManagedStandalone && isOp(op, OpStart, OpStop, OpDrain, OpRestart, OpEnable):
		return false, ReasonStandalone
	case r.Managed == runner.ManagedUnavailable && isOp(op, OpStart, OpStop, OpDrain, OpRestart, OpEnable):
		return false, ReasonManagedUnknown
	default:
		return true, ""
	}
}

// isOp は操作が候補のいずれかかを返す。
//
// 判定表の 1 行を「どの操作に効くか」の列挙としてそのまま読めるようにするための小道具。
func isOp(op Op, ops ...Op) bool { return slices.Contains(ops, op) }

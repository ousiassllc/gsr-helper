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
//   - 管理状態が判定できない（4 段目）ときに塞ぐのは、実行経路が「systemd 管理か
//     どうか」に依存する 4 つである。強制停止とドレイン停止は worker のプロセスに
//     作用するので使える。
//
// スコープ不足（表の 6 行目）はここに無い。保有スコープの判定には GitHub API が必要で、
// appconfig.Caps はその情報を持たないためである。GitHub API を持ち込む Issue が足す。
func CanControl(op Op, r runner.Runner, caps appconfig.Caps) (bool, string) {
	switch {
	case !caps.Root && isOp(op, OpStart, OpStop, OpKill, OpRestart):
		return false, ReasonRoot
	case !caps.Systemd && isOp(op, OpStart, OpStop, OpKill, OpDrain, OpRestart, OpEnable):
		return false, ReasonSystemd
	case r.Managed == runner.ManagedStandalone && isOp(op, OpStart, OpStop, OpRestart):
		return false, ReasonStandalone
	case r.Managed == runner.ManagedUnavailable && isOp(op, OpStart, OpStop, OpRestart, OpEnable):
		return false, ReasonManagedUnknown
	default:
		return true, ""
	}
}

// isOp は操作が候補のいずれかかを返す。
//
// 判定表の 1 行を「どの操作に効くか」の列挙としてそのまま読めるようにするための小道具。
func isOp(op Op, ops ...Op) bool { return slices.Contains(ops, op) }

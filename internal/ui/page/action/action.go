package action

import (
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 操作そのものの定義（識別子・表示に載せる不透明な ID・影響と実装状況）を集める。
// 可否の判定と一覧の組み立ては actions.go にある。

// ID は runner に対する操作の識別子。
//
// 可否の判定（Allow）と影響・破壊性（meta）を、押すキーではなく操作そのもので
// 引くために置く。キーストロークのリテラルで表を引くと、keymap でキーを差し替えた
// ときに判定がコンパイルエラーも無く別の操作へ移る（または消える）。判定を
// svc.CanControl へ寄せるときも、渡すのはキーではなく操作である。
type ID int

// ID の取り得る値。keymap.RunnerKeys のフィールドと 1 対 1 に対応する。
//
// Unknown（ゼロ値）は keymap のどのキーにも対応しないキーストロークを表す。
// 判定表のどの行にも当たらないため、最後の未対応の判定で塞がれる。
const (
	Unknown ID = iota
	Start
	Stop
	Kill
	Drain
	Restart
	Enable
	Add
	Delete
	Update
	Edit
	Logs
)

// names は ID の不透明な識別子。ID の並びと 1 対 1 に対応する。
//
// キーストロークではなくこの文字列を表示層（organism.Choice.ID）へ通す。キーを通すと
// 決定が「ID → キー文字列 → 再マップ」で往復し、キーを差し替えたときに黙って
// 別の操作へ移りうる（Issue #34）。
var names = [...]string{
	Unknown: "unknown",
	Start:   "start",
	Stop:    "stop",
	Kill:    "kill",
	Drain:   "drain",
	Restart: "restart",
	Enable:  "enable",
	Add:     "add",
	Delete:  "delete",
	Update:  "update",
	Edit:    "edit",
	Logs:    "logs",
}

// String は操作の不透明な識別子を返す。
func (id ID) String() string {
	if id < 0 || int(id) >= len(names) {
		return names[Unknown]
	}
	return names[id]
}

// byName は識別子から ID を引く表。表示層から戻ってきた ID を解く。
var byName = func() map[string]ID {
	out := make(map[string]ID, len(names))
	for id, name := range names {
		out[name] = ID(id)
	}
	return out
}()

// Of は表示層から戻ってきた識別子を ID に解く。
//
// 解けない識別子（表示層が勝手に付けた値）は Unknown と偽を返す。
func Of(id string) (ID, bool) {
	a, ok := byName[id]
	return a, ok && a != Unknown
}

// Def は詳細画面の操作リスト 1 項目の定義。
//
// Supported は「この版で実装済みか」を表す。真なのはサービス制御の 6 つ
// （開始・停止・強制停止・ドレイン停止・再起動・enable の切替）で、追加・削除・更新・
// 設定編集・ログは後続の Issue が担うため偽である。Def の定義にこのフィールドを
// 置くことで、後続 Issue は meta の 1 箇所を真にするだけで操作を有効化でき、
// 可否の判定（Allow）とフッタ・操作リストの描画には手を入れずに済む。
type Def struct {
	ID          ID // どの操作か（判定はキーではなくこれで引く）
	Key         string
	Desc        string
	Impact      string // 破壊的操作の影響
	Destructive bool   // 区切り線の下に置くか
	Supported   bool   // この版で実装済みか
}

// newDef は操作の識別子・キー・説明から操作の定義を組む。
func newDef(id ID, k, desc string) Def {
	impact, destructive, supported := meta(id)
	return Def{
		ID:          id,
		Key:         k,
		Desc:        desc,
		Impact:      impact,
		Destructive: destructive,
		Supported:   supported,
	}
}

// 影響の文言。screens.md の**詳細画面（`enter`）のモック**の括弧内（`x` / `X` /
// `R` / `D` の行）と一致させる。
//
// 出どころをモックに採るのは、モックだけが表示文字列そのものを書いているためである。
// 同じ節の「Runners タブの操作」の表にも影響の列があるが、そこは「安全」「—」
// 「ドレイン停止を伴う」「反映方法を選択」まで含む**記述的な要約**で、`X` の
// `⚠ 実行中ジョブを中断` のようにモックとも実装とも字面が違う。要約を典拠にすると、
// 表の言い回しを整えただけで実装が食い違ったと読めてしまう。
//
// **停止・再起動に ⚠ を付けないのは、モックが強さを書き分けているためである。**
// `X`（強制停止）と `D`（削除）は ⚠ 付きで「中断」「登録解除」と断定するが、
// `x`（停止）と `R`（再起動）は記号を付けずに「影響する可能性」と書く。詳細画面の
// 操作リストでも同じ差が出て、⚠ の行だけが確実に起こる被害だと読める。
const (
	impactAffectsJob = "実行中ジョブに影響する可能性"
	impactKill       = token.IconWarn + " 実行中のジョブは中断されます"
	impactDelete     = token.IconWarn + " 登録解除 + サービス削除"
)

// meta は操作ごとの影響・破壊性・実装状況を返す。
//
// 影響の文言は screens.md の詳細画面のモックに従う（上記 impactAffectsJob ほか）。
// 影響を持つのは停止・強制停止・再起動・削除の 4 つである。
//
// **再起動は破壊的でない（区切り線の上に置く）が影響の文言を持つ。** Destructive は
// 詳細画面の操作リストで区切り線の下に置くかどうかだけを決める値であり、確認
// ダイアログを経るかどうかとは別である。停止・強制停止・再起動の 3 つは
// functional.md の確認フロー図が必須段とする「対象と影響の提示」を経るため
// （Issue #5）、影響が空だと dialog.Confirm の「空のブロックは見出しごと落とす」
// 規則で影響のブロックがまるごと消える。破壊性ではなく確認を経るかどうかで
// 文言の有無が決まる。
//
// Supported が真なのはサービス制御の 6 つ（internal/svc が実装した開始・停止・強制
// 停止・ドレイン停止・再起動・enable の切替）に限る。追加・削除・更新・ログ・設定編集は
// 後続の Issue が担うため偽のままで、「押せるが何も起きない」経路を作らない。
func meta(id ID) (impact string, destructive, supported bool) {
	switch id {
	case Start, Drain, Enable:
		return "", false, true
	case Restart:
		return impactAffectsJob, false, true
	case Stop:
		return impactAffectsJob, true, true
	case Kill:
		return impactKill, true, true
	case Delete:
		return impactDelete, true, false
	default:
		return "", false, false
	}
}

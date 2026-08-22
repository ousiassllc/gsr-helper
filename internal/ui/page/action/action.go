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

// meta は操作ごとの影響・破壊性・実装状況を返す。
//
// 影響の文言は screens.md の詳細画面のモックに従う。破壊的な操作（区切り線の下に
// 置くもの）は停止・強制停止・削除の 3 つである。
//
// Supported が真なのはサービス制御の 6 つ（internal/svc が実装した開始・停止・強制
// 停止・ドレイン停止・再起動・enable の切替）に限る。追加・削除・更新・ログ・設定編集は
// 後続の Issue が担うため偽のままで、「押せるが何も起きない」経路を作らない。
func meta(id ID) (impact string, destructive, supported bool) {
	switch id {
	case Start, Drain, Restart, Enable:
		return "", false, true
	case Stop:
		return "", true, true
	case Kill:
		return token.IconWarn + " 実行中のジョブは中断されます", true, true
	case Delete:
		return token.IconWarn + " 登録解除 + サービス削除", true, false
	default:
		return "", false, false
	}
}

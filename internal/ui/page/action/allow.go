// Package action は runner に対する操作の識別・可否の判定・一覧の組み立てを提供する。
//
// page から分けているのは、モーダルの重なりと共有状態の受け渡しを持つ page に
// 操作の判定表まで同居させるとディレクトリが上限を超えるためである
// （atomic-design.md の「ディレクトリの行数」）。依存は action → page の一方向で、
// page はこのパッケージを import しない。
package action

import (
	"slices"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/svc"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
)

// サービス制御に依らない操作（追加・削除・更新）が使う理由の文言。screens.md の
// 「無効な操作の表示」の表と一致させる。
//
// 定数にするのは、フッタ（atom.KeyHint）と詳細画面の操作リスト（molecule.ActionRow）が
// 同じ文字列を出すためである。同じ理由を 2 箇所に書くと片方だけが直る。
//
// **root / systemd / 管理外 / 判定不能の 4 つはここに無い。** サービス制御の可否と
// 同じ理由であり、出どころを svc（ReasonRoot 他）の 1 箇所に寄せてある。認証と
// ジョブ実行中は svc の関心事ではないのでここに残す。
const (
	//nolint:gosec // G101 誤検知。認証を促す画面上の説明文であり、資格情報を含まない。
	reasonToken = "GitHub の認証が必要です（gh auth login）"
	reasonBusy  = "ジョブ実行中です。先に d でドレイン停止してください"
)

// Set はキー定義から 1 度だけ組んだ操作の表。
//
// **描画のたびに組み直さない。** 以前は可否を 1 件求めるたびにキーから操作を引く
// 11 要素の map を確保し、フッタ 1 行の描画で 9 回作っていた（Issue #34）。page は
// 共有状態を受けた時点で 1 つ組み、以後の描画はそれを使う。
type Set struct {
	list  []Def
	byKey map[string]ID
	// scopes は保有スコープ。可否の判定に使う（Issue #79）。
	//
	// **Set が持つのは、判定が 1 フレームに何度も走るためである。** 取得は GitHub API
	// への往復を要するのでそこからは引けず、共有状態として届いた値を組み立て時に
	// 写し取る（page が StateMsg を受けるたびに Set を組み直す）。
	scopes page.ScopeState
}

// NewSet はキー定義から操作の表を組む。
//
// 並び順は keymap.RunnerKeys.Detail（screens.md の詳細画面のキー表と同じ並び。
// 安全な操作が先、破壊的な操作が後）に従う。順序と集合の定義を keymap に一本化
// することで、フッタ・詳細画面の操作リスト・ヘルプで並びが食い違わない。表を
// 各フィールドから作るのは、キーの差し替えに判定が自動で追従するためである。
//
// **2 つの操作が同じ先頭キーを持つと panic する。** 黙って上書きすると片方の操作が
// 判定表のどの行にも当たらなくなり、理由が page.ReasonUnsupported にすり替わる。キー定義は
// 起動時に決まるので、誤りは最初の起動で必ず表面化する。
func NewSet(keys keymap.RunnerKeys, scopes page.ScopeState) Set {
	byKey := make(map[string]ID, len(names))
	for _, a := range keyIDs(keys) {
		if prev, dup := byKey[a.key]; dup {
			panic("page: 操作キー " + a.key + " が " + prev.String() + " と " + a.id.String() + " で重複している")
		}
		byKey[a.key] = a.id
	}

	order := keys.Detail()
	list := make([]Def, 0, len(order))
	for _, b := range order {
		k := page.BindingKey(b)
		list = append(list, newDef(byKey[k], k, b.Help().Desc))
	}
	return Set{list: list, byKey: byKey, scopes: scopes}
}

// withScopes は保有スコープだけを差し替えた写しを返す。
//
// キー定義から表を組み直さずに済ませるための小道具である。検証で状態を並べるときに
// 使うほか、共有状態のうちスコープだけが後から確定する経路（親が起動後に 1 度だけ
// 引く。Issue #79）にも合う。
func (s Set) withScopes(scopes page.ScopeState) Set {
	s.scopes = scopes
	return s
}

// keyID はキーストロークと操作の識別子の対。
type keyID struct {
	key string
	id  ID
}

// keyIDs はキーストロークと操作の識別子の対を keymap から並べる。
//
// map ではなく並びを返すのは、同じ先頭キーが 2 つ現れたことを呼び出し側が検出できる
// ようにするためである（map に詰めた時点で片方が黙って消える）。
func keyIDs(keys keymap.RunnerKeys) []keyID {
	return []keyID{
		{page.BindingKey(keys.Start), Start},
		{page.BindingKey(keys.Stop), Stop},
		{page.BindingKey(keys.Kill), Kill},
		{page.BindingKey(keys.Drain), Drain},
		{page.BindingKey(keys.Restart), Restart},
		{page.BindingKey(keys.Enable), Enable},
		{page.BindingKey(keys.Add), Add},
		{page.BindingKey(keys.Delete), Delete},
		{page.BindingKey(keys.Update), Update},
		{page.BindingKey(keys.Edit), Edit},
		{page.BindingKey(keys.Logs), Logs},
	}
}

// List は詳細画面に並べる操作を安全な順に返す。
func (s Set) List() []Def { return s.list }

// Allow は操作の可否と不可の理由を返す。判定表は screens.md「無効な操作の表示」に従う。
//
// 可否の判断は page が行い、atom.KeyHint / molecule.ActionRow / organism.ChoiceList は
// 受け取った値を描くだけにする。判断を表示部品に持たせると、同じ判定がフッタ・操作
// リスト・確認ダイアログの 3 箇所に分かれて食い違う。
//
// **サービス制御の可否は svc.CanControl へ委譲済みである**（components/overview.md の
// internal/svc）。表示層が持つのは「どの操作をドメイン層のどの操作として問うか」
// （SvcOp）だけで、判定表そのものは持たない。残っているのは svc の関心事ではない
// 追加・削除・更新（認証とジョブ実行中）と、実装状況の判定である。
//
// 判定は下の順で行い、最初に一致した理由を返す。能力の問題（root / systemd / 認証）を
// 実装状況（Supported）で隠さないため、未対応の判定を最後に置く。
func Allow(a Def, r runner.Runner, caps appconfig.Caps, scopes page.ScopeState) (bool, string) {
	if op, ok := SvcOp(a.ID); ok {
		if allowed, reason := svc.CanControl(op, r, caps); !allowed {
			return false, reason
		}
	}
	// スコープ不足の理由は 1 度だけ求める。判定は 1 フレームに何度も走る（フッタは
	// キーぶん、操作リストは項目ぶん）ので、条件と戻り値で 2 度評価すると塞ぐ側で
	// 文字列の組み立てが二重に走る。
	scopeReason := ""
	if is(a, Add, Delete) {
		scopeReason = missingScope(r, scopes)
	}

	switch {
	case !caps.Root && is(a, Delete, Add, Update):
		return false, svc.ReasonRoot
	case !caps.GitHubToken && is(a, Add, Delete, Update):
		return false, reasonToken
	case scopeReason != "":
		// 6 段目「スコープ不足」。塞ぐのは n / D の 2 つで、u（更新）は含めない
		// （screens.md「無効な操作の表示」の表）。
		return false, scopeReason
	case r.Busy() && a.ID == Delete:
		return false, reasonBusy
	case !a.Supported:
		return false, page.ReasonUnsupported
	default:
		return true, ""
	}
}

// SvcOp は操作の識別子をサービス制御の操作へ対応付ける。対応が無ければ偽を返す。
//
// 対応表を UI 側に置くのは、svc がキー定義も action.ID も知らないためである
// （依存は ui/page/action → svc の一方向）。追加・削除・更新・設定編集・ログは
// systemd 経由のサービス制御ではないので、対応を持たない。
//
// 公開しているのは、可否の判定（Allow）だけでなく**実行と確認ダイアログの
// コマンド表示**（ui/page/runnerop）も同じ対応を必要とするためである。対応表が
// 2 箇所に分かれると、片方だけが新しい操作を知っている状態を作れてしまい、
// 「フッタでは押せるのに実行だけ何も起きない」形の欠陥になる。
func SvcOp(id ID) (svc.Op, bool) {
	switch id {
	case Start:
		return svc.OpStart, true
	case Stop:
		return svc.OpStop, true
	case Kill:
		return svc.OpKill, true
	case Drain:
		return svc.OpDrain, true
	case Restart:
		return svc.OpRestart, true
	case Enable:
		return svc.OpEnable, true
	default:
		return 0, false
	}
}

// is は操作が候補のいずれかかを返す。
//
// 判定表の 1 行を「どの操作に効くか」の列挙として読めるようにするための小道具。
func is(a Def, ids ...ID) bool {
	return slices.Contains(ids, a.ID)
}

// Hints は操作のキーヒントをフッタ 1 行目に並べる順で返す。
//
// 集合・並び・説明文は keymap.RunnerKeys.Footer（screens.md の共通レイアウトのフッタ）に
// 従う。詳細画面の操作リスト（Actions）と別に持つのは、フッタが幅 80 に収まる 9 個に
// 絞った短い表記を使うためである。幅に収まらない分を落とすのは molecule.KeyBar の役割。
func (s Set) Hints(r runner.Runner, caps appconfig.Caps, keys keymap.RunnerKeys) []atom.Hint {
	footer := keys.Footer()
	out := make([]atom.Hint, 0, len(footer))
	for _, f := range footer {
		k := page.BindingKey(f.Binding)
		enabled, reason := s.Allowed(k, r, caps)
		out = append(out, atom.Hint{Key: k, Desc: f.Desc, Enabled: enabled, Reason: reason})
	}
	return out
}

// Allowed はキー 1 つの可否と理由を返す。
//
// Jobs タブのようにフッタの並びを独自に持つ画面が、可否の判定だけを共通化するために
// 使う。判定を画面ごとに作り直すと、同じ操作の理由が画面によって食い違う。
//
// 判定はキーではなく操作で行うので、キーだけしか持たない呼び出し側は組み済みの
// 対応表（Set）を通す。
func (s Set) Allowed(k string, r runner.Runner, caps appconfig.Caps) (bool, string) {
	return Allow(newDef(s.byKey[k], k, ""), r, caps, s.scopes)
}

// Choices は詳細画面の操作リストの項目を返す。
//
// 区切り線は最初の破壊的な操作の前に 1 本だけ置く。organism.ChoiceList は
// 区切り線より下を破壊的な操作の区画として描く。
func (s Set) Choices(r runner.Runner, caps appconfig.Caps) []organism.Choice {
	return choices(s.list, r, caps, s.scopes)
}

// choices は操作の定義から選択肢を組み立てる。
//
// Actions と分けているのは、操作の一覧（何を並べるか）と可否の判定（押せるか）を
// 別々に検証できるようにするためである。
func choices(acts []Def, r runner.Runner, caps appconfig.Caps, scopes page.ScopeState) []organism.Choice {
	out := make([]organism.Choice, 0, len(acts))
	divided := false
	for _, a := range acts {
		enabled, reason := Allow(a, r, caps, scopes)
		divider := a.Destructive && !divided
		if divider {
			divided = true
		}
		out = append(out, organism.Choice{
			ID:            a.ID.String(),
			Key:           a.Key,
			Desc:          a.Desc,
			Impact:        a.Impact,
			Reason:        reason,
			Enabled:       enabled,
			DividerBefore: divider,
		})
	}
	return out
}

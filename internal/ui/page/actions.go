package page

import (
	"slices"

	"charm.land/bubbles/v2/key"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/keymap"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// 操作できない理由の文言。screens.md の「無効な操作の表示」の表と一致させる。
//
// 定数にするのは、フッタ（atom.KeyHint）と詳細画面の操作リスト（molecule.ActionRow）が
// 同じ文字列を出すためである。同じ理由を 2 箇所に書くと片方だけが直る。
const (
	reasonRoot       = "root 権限が必要です（sudo で起動してください）"
	reasonSystemd    = "サービス制御は利用できません（systemctl が見つかりません）"
	reasonStandalone = "systemd 管理外のため操作できません"
	//nolint:gosec // G101 誤検知。認証を促す画面上の説明文であり、資格情報を含まない。
	reasonToken       = "GitHub の認証が必要です（gh auth login）"
	reasonBusy        = "ジョブ実行中です。先に d でドレイン停止してください"
	reasonUnsupported = "この版では未対応です"
)

// Action は詳細画面の操作リスト 1 項目の定義。
//
// Supported は「この版で実装済みか」を表す。操作の実装は後続の Issue が担うため、
// 現時点ではすべて false である。Action の定義にこのフィールドを置くことで、
// 後続 Issue は actionMeta の 1 箇所を true にするだけで操作を有効化でき、
// 可否の判定（Allow）とフッタ・操作リストの描画には手を入れずに済む。
type Action struct {
	Key         string
	Desc        string
	Impact      string // 破壊的操作の影響
	Destructive bool   // 区切り線の下に置くか
	Supported   bool   // この版で実装済みか
}

// Actions は詳細画面に並べる操作を安全な順に返す。
//
// 並び順は keymap.RunnerKeys.Detail（screens.md の詳細画面のキー表と同じ並び。
// 安全な操作が先、破壊的な操作が後）に従う。順序と集合の定義を keymap に一本化
// することで、フッタ・詳細画面の操作リスト・ヘルプで並びが食い違わない。
func Actions(keys keymap.RunnerKeys) []Action {
	order := keys.Detail()
	out := make([]Action, 0, len(order))
	for _, b := range order {
		out = append(out, newAction(BindingKey(b), b.Help().Desc))
	}
	return out
}

// newAction はキーと説明から操作の定義を組む。
func newAction(k, desc string) Action {
	impact, destructive, supported := actionMeta(k)
	return Action{
		Key:         k,
		Desc:        desc,
		Impact:      impact,
		Destructive: destructive,
		Supported:   supported,
	}
}

// BindingKey は Binding が受け付ける実際のキー文字列を返す。
//
// ヘルプの表記（Help().Key）ではなく Keys() の先頭を使うのは、organism.ChoiceList が
// 押されたキー（tea.KeyPressMsg.String()）と Choice.Key を突き合わせるためである。
// page/<tab> もフッタのキー表記をこの関数で作り、詳細画面と表記が食い違わないように
// する（keymap の Help().Key は "j/↓" のように複数キーをまとめた表記になる）。
func BindingKey(b key.Binding) string {
	if ks := b.Keys(); len(ks) > 0 {
		return ks[0]
	}
	return b.Help().Key
}

// actionMeta はキーごとの影響・破壊性・実装状況を返す。
//
// 影響の文言は screens.md の詳細画面のモックに従う。破壊的な操作（区切り線の下に
// 置くもの）は停止・強制停止・削除の 3 つである。
//
// Supported はすべて false を返す。操作の実装は後続の Issue（サービス制御・追加削除
// 更新・ログ・設定編集）が担うため、この版では「押せるが何も起きない」経路を作らない。
func actionMeta(k string) (impact string, destructive, supported bool) {
	switch k {
	case "x":
		return "", true, false
	case "X":
		return token.IconWarn + " 実行中のジョブは中断されます", true, false
	case "D":
		return token.IconWarn + " 登録解除 + サービス削除", true, false
	default:
		return "", false, false
	}
}

// Allow は操作の可否と不可の理由を返す。判定表は screens.md「無効な操作の表示」に従う。
//
// 可否の判断は page が行い、atom.KeyHint / molecule.ActionRow / organism.ChoiceList は
// 受け取った値を描くだけにする。判断を表示部品に持たせると、同じ判定がフッタ・操作
// リスト・確認ダイアログの 3 箇所に分かれて食い違う。
//
// 本来この判定は svc.CanControl に集約する規約（components/overview.md の internal/svc）
// だが、svc パッケージはサービス制御の Issue で作る。そこで本関数の中身を
// svc.CanControl の呼び出しに差し替える（シグネチャは変えない）。
//
// 判定は下の順で行い、最初に一致した理由を返す。能力の問題（root / systemd / 認証）を
// 実装状況（Supported）で隠さないため、未対応の判定を最後に置く。
func Allow(a Action, r runner.Runner, caps appconfig.Caps) (bool, string) {
	switch {
	case !caps.Root && slices.Contains([]string{"s", "x", "X", "R", "D", "n", "u"}, a.Key):
		return false, reasonRoot
	case !caps.Systemd && slices.Contains([]string{"s", "x", "X", "R", "E", "d"}, a.Key):
		return false, reasonSystemd
	case r.Managed == runner.ManagedStandalone && slices.Contains([]string{"s", "x", "R"}, a.Key):
		return false, reasonStandalone
	case !caps.GitHubToken && slices.Contains([]string{"n", "D", "u"}, a.Key):
		return false, reasonToken
	case r.Busy() && a.Key == "D":
		return false, reasonBusy
	case !a.Supported:
		return false, reasonUnsupported
	default:
		return true, ""
	}
}

// Hints は操作のキーヒントをフッタ 1 行目に並べる順で返す。
//
// 集合・並び・説明文は keymap.RunnerKeys.Footer（screens.md の共通レイアウトのフッタ）に
// 従う。詳細画面の操作リスト（Actions）と別に持つのは、フッタが幅 80 に収まる 9 個に
// 絞った短い表記を使うためである。幅に収まらない分を落とすのは molecule.KeyBar の役割。
func Hints(r runner.Runner, caps appconfig.Caps, keys keymap.RunnerKeys) []atom.Hint {
	footer := keys.Footer()
	out := make([]atom.Hint, 0, len(footer))
	for _, f := range footer {
		k := BindingKey(f.Binding)
		enabled, reason := Allowed(k, r, caps)
		out = append(out, atom.Hint{Key: k, Desc: f.Desc, Enabled: enabled, Reason: reason})
	}
	return out
}

// Allowed はキー 1 つの可否と理由を返す。
//
// Jobs タブのようにフッタの文言を独自に持つ画面が、可否の判定だけを共通化するために
// 使う。判定を画面ごとに作り直すと、同じ操作の理由が画面によって食い違う。
func Allowed(k string, r runner.Runner, caps appconfig.Caps) (bool, string) {
	return Allow(newAction(k, ""), r, caps)
}

// Choices は詳細画面の操作リストの項目を返す。
//
// 区切り線は最初の破壊的な操作の前に 1 本だけ置く。organism.ChoiceList は
// 区切り線より下を破壊的な操作の区画として描く。
func Choices(r runner.Runner, caps appconfig.Caps, keys keymap.RunnerKeys) []organism.Choice {
	return choices(Actions(keys), r, caps)
}

// choices は操作の定義から選択肢を組み立てる。
//
// Actions と分けているのは、操作の一覧（何を並べるか）と可否の判定（押せるか）を
// 別々に検証できるようにするためである。
func choices(acts []Action, r runner.Runner, caps appconfig.Caps) []organism.Choice {
	out := make([]organism.Choice, 0, len(acts))
	divided := false
	for _, a := range acts {
		enabled, reason := Allow(a, r, caps)
		divider := a.Destructive && !divided
		if divider {
			divided = true
		}
		out = append(out, organism.Choice{
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

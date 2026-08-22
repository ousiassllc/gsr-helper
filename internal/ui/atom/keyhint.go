package atom

import "github.com/ousiassllc/gsr-helper/internal/ui/token"

// Hint は 1 つのキーヒントの表示内容。
//
// 可否（Enabled）と不可の理由（Reason）の判断は page がドメイン層に問い合わせて
// 行い、atom は受け取った値を描くだけにする。判断を 2 箇所に置くと、
// フッタと詳細画面の操作リストで理由が食い違う。
//
// Enabled が false のときは Reason を必ず埋めること。理由の無いグレーアウトは
// 「なぜ押せないのか」が分からない状態を残す（screens.md の無効な操作の表示）。
type Hint struct {
	Key     string // 押すキー（"x" / "ctrl+f" など）
	Desc    string // 動作の説明（日本語）
	Enabled bool   // 押せるか
	Reason  string // 押せない理由。まとめて 1 行に出すのは molecule.KeyBar の役割
}

// KeyHint は 1 つのキーヒントを返す。
//
// 無効なときもキーを消さずグレーアウトする（screens.md の無効な操作の表示）。
// キーを消すと「押せない操作」と「存在しない操作」を区別できなくなる。
//
// **無効なときも幅を増やさない。** フッタ 1 行目は screens.md が定める 9 個のキーと
// ?:ヘルプ で 75 セルを使う。丸括弧で囲むと 1 つあたり 2 セル増えて 95 セルになり、
// 幅 80 に収まらなくなる。
// 「色に依存しない」（screens.md の設計原則 4）は別の手がかりで満たす。フッタでは
// 2 行目が無効なキーを "(s)(x)(X): 理由" と丸括弧付きで並べ（molecule.KeyBar）、
// 詳細画面の操作リストでは同じ行の右端に理由が出る（molecule.ActionRow）。
func KeyHint(h Hint, s token.Styles) string {
	sep := ":"
	if h.Key == "" || h.Desc == "" {
		sep = ""
	}
	if !h.Enabled {
		label := h.Key + sep + h.Desc
		if label == "" {
			return ""
		}
		return s.Muted.Render(label)
	}
	return s.Accent.Render(h.Key) + sep + h.Desc
}

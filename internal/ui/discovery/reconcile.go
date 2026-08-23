package discovery

import "github.com/ousiassllc/gsr-helper/internal/runner"

// Outcome は 1 周期分の検出結果（Msg）をどう扱うかを表す。親 Model（ui.App）は
// この値をそのままフィールドへ書き写して配るだけで済み、周期の追い抜き・部分結果の
// 扱い・起動時の前提チェックを許可する条件の判断はここに閉じ込める（Reconcile）。
type Outcome struct {
	// Applied は次に保持すべき「取り込み済みの周期番号」。
	Applied int
	// Result は次に保持すべき一覧。
	Result runner.Result
	// Err は次に保持すべき検出のエラー（状態行に出す）。
	Err error
	// Stale が真なら、この周期は追い抜かれているため丸ごと捨てる
	// （他のフィールドは Applied 以外意味を持たない）。
	Stale bool
	// StartHostReq が真なら、この周期は起動時の前提チェック（FR-44）を
	// 始めてよい成功周期である。
	StartHostReq bool
}

// Reconcile は 1 周期分の検出結果（msg）を、直前まで保持していた状態
// （applied / cur / curErr）と突き合わせて Outcome を決める。
//
// **追い抜かれた周期は丸ごと捨てる。** 検出には最大 discovery.Budget（15 秒）
// かかるので、遅い周期が新しい周期より後に返ることがある（古い周期の結果で
// 新しい結果を上書きしないため必要。Msg.Seq の doc）。
//
// **期限切れ・失敗した周期の部分結果では上書きしない。** runner.Discover は
// ctx がキャンセルされた時点で残りの systemctl show を発行せず取れた分だけを
// 返すため、部分結果を採ると systemd 管理の runner が run.sh / - と誤表示され、
// 孤児ユニットも過少報告される。エラーは状態行の警告として返し、一覧は直前の
// 成功結果を保つ。
//
// **起動時の前提チェックは成功周期でだけ許可する。** 失敗した周期で許可すると、
// 1 度きりの実行を空の Runners で使い切り、runner ごとに判定する 2 項目
// （NOPASSWD sudo / docker グループ所属）がセッション中一度も走らず警告も出ない。
func Reconcile(applied int, cur runner.Result, curErr error, msg Msg) Outcome {
	if msg.Seq < applied {
		return Outcome{Applied: applied, Result: cur, Err: curErr, Stale: true}
	}

	result, err := cur, msg.Err
	if msg.Err == nil {
		result = msg.Result
	}
	return Outcome{Applied: msg.Seq, Result: result, Err: err, StartHostReq: msg.Err == nil}
}

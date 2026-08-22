package token

// 表示に使う記号。screens.md の記号表と一対一で対応させる。
//
// 表示に使う記号をここ以外に書かない。色を使えない端末でも状態を判別できる
// ようにするため、記号は配色と同じ「表示の定義」として 1 箇所に集める。
const (
	IconActive    = "●"   // サービス稼働中
	IconInactive  = "○"   // サービス停止中
	IconFailed    = "✗"   // 異常終了・FAIL
	IconNoUnit    = "-"   // systemd ユニットなし
	IconJob       = "▶"   // ジョブ実行中
	IconWarn      = "⚠"   // 注意事項あり・WARN
	IconOK        = "✓"   // OK・完了
	IconSkip      = "⊘"   // SKIP
	IconCursor    = "▸"   // カーソル位置
	IconChecked   = "[x]" // 複数選択：選択済み
	IconUnchecked = "[ ]" // 複数選択：未選択
	IconDivider   = "─"   // 区画の区切り
	IconEllipsis  = "…"   // 幅に収まらない文字列の中略
)

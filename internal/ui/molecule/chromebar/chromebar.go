// Package chromebar は共通レイアウトの枠に出る 3 本の帯——ヘッダ（CapsBar）・
// タブ行（TabBar）・フッタ（KeyBar）——を提供する（docs/ui/screens.md の共通レイアウト）。
//
// molecule 直下から分けているのは、**増え方が違う**ためである（1.18 で行ビルダを
// molecule/listrow へ分けたのと同じ軸）。この 3 本は 1 画面につき必ず 1 本ずつで、
// **タブやダイアログが何枚増えても本数は変わらない。** 一方 molecule 直下に残る
// 部品（ActionRow / FSSummaryLine / CommandBlock / LogLine / SummaryCounts /
// ProgressRow）は、タブとダイアログが増えるたびに種類が増える。
//
// 依存は atom / token / lipgloss だけで、**molecule も molecule/listrow も参照
// しない。** 組み合わせるのは ui/chrome と ui/template である。同階層参照の禁止に
// 例外を増やさないため、逆向き（molecule → chromebar）も作らない。
package chromebar

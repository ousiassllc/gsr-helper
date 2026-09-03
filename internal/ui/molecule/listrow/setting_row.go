package listrow

import (
	"github.com/ousiassllc/gsr-helper/internal/ui/atom"
	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// ColValue は現在値の列の識別子。
//
// 他の列 ID（token.ColName など）と違って token に置いていないのは、Config タブの
// 列定義（幅と落とし方。token.ConfigColumns / ConfigColumnRules に当たるもの）を
// 持ち込むのがこの行ビルダとは別の Issue だからである。**識別子だけを先に置く。**
// 表記は他の列 ID と同じ大文字の文字列にしてあり、列定義が入る際は値を変えずに
// token へ移せる。項目名と注意書きに新しい ID を作らず token.ColName /
// token.ColNote を使い回すのは、意味が同じ列の識別子を 2 つに割らないためである。
const ColValue = "VALUE"

// SettingView は Config タブの設定項目 1 行の表示用の構造体。
//
// ドメインの型（.env や systemd drop-in の設定）は受け取らない。molecule 以下は
// 表示に必要な値だけを落とした構造体とプリミティブを受け取る（atomic-design.md の
// 依存の規則）。現在値を文字列で持つのは、値の種類（件数・パス・ラベルの並び）が
// 項目ごとに違い、整形の判断が Config タブ側にしか無いためである。
type SettingView struct {
	// Item は項目名（「.env（環境変数・プロキシ・job hooks）」「runner group」）。
	Item string
	// Value は現在値（「12 項目」「Default」）。空なら「値なし」の記号を出す。
	Value string
	// Note は注意書き（「変更には再登録が必要」）。空なら注記なし。
	Note string
	// Warn は注意書きを警告として描くか。立てると注記の頭に ⚠ が付く。
	Warn bool
}

// SettingRow は cols と同じ順・同じ数のセルを返す。
//
// セル数を列数に必ず揃える理由は RunnerRow と同じで、bubbles/table が行のセルを
// 走査しながら同じ添字の列定義を引くためである。数が足りなければ桁がずれ、
// 超えると添字範囲外で panic する。
func SettingRow(v SettingView, cols []token.Column, s token.Styles) []string {
	cells := make([]string, 0, len(cols))
	for _, c := range cols {
		cells = append(cells, settingCell(v, c, s))
	}
	return cells
}

// settingCell は列 1 つ分のセルを返す。
func settingCell(v SettingView, c token.Column, s token.Styles) string {
	switch c.ID {
	case token.ColName:
		return dashCell(v.Item, c, s)
	case ColValue:
		return dashCell(v.Value, c, s)
	case token.ColNote:
		return settingNoteCell(v, c, s)
	default:
		// 知らない列でもセルを欠かさない（列数とセル数を必ず一致させる）。
		return atom.Cell("", c.Width, columnAlign(c))
	}
}

// settingNoteCell は注意書きのセルを返す。
//
// 警告の注記に記号（⚠）を添えるのは、色を使わない端末でも「ただの補足」と
// 「変更に代償が伴う項目」を読み分けられるようにするためである（設計原則 4）。
// 記号を行末ではなく注記の先頭に置くのは、bubbles/table が列幅を超えた分を
// 切り落とすため、末尾に付けると記号から先に消えるからである。
//
// 警告でない注記は孤児ユニットの NOTE と同じく薄く描く。補足情報が本文と同じ
// 濃さで並ぶと、読む順序が項目名・現在値からそれる。
func settingNoteCell(v SettingView, c token.Column, s token.Styles) string {
	switch {
	case v.Note == "":
		return dashCell("", c, s)
	case v.Warn:
		return styledCell(token.Icon(token.StateWarn)+" "+v.Note, token.StateWarn.Role(), c, s)
	default:
		return styledCell(v.Note, token.RoleMuted, c, s)
	}
}

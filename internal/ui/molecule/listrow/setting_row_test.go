package listrow

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/ui/token"
)

// settingColumns は Config タブの設定一覧の列（項目名 / 現在値 / 注意書き）を返す。
//
// token に列定義が入るまでのテスト用の並びである（ColValue の doc）。幅は
// screens.md の Config タブのモックに合わせてある。
func settingColumns() []token.Column {
	return []token.Column{
		{ID: token.ColName, Title: "項目", Width: 40, Right: false},
		{ID: ColValue, Title: "現在値", Width: 30, Right: false},
		{ID: token.ColNote, Title: "注意", Width: 24, Right: false},
	}
}

// sampleSetting は表示に必要な値が全て埋まった設定項目を返す。
func sampleSetting() SettingView {
	return SettingView{
		Item:  ".env（環境変数・プロキシ・job hooks）",
		Value: "12 項目",
		Note:  "",
		Warn:  false,
	}
}

// setting は sampleSetting に変更を加えた設定項目を返す。
func setting(f func(v *SettingView)) SettingView {
	v := sampleSetting()
	f(&v)
	return v
}

// セル数が列数と一致し、各セル幅が列幅と一致する。
//
// 知らない列 ID でもセルを欠かさないことを併せて確かめる。行のセル数が列数より
// 少ないと bubbles/table の桁がずれ、多いと添字範囲外で panic する。
func TestSettingRowCells(t *testing.T) {
	views := map[string]SettingView{
		"標準":     sampleSetting(),
		"空の値":    {},
		"長い現在値":  setting(func(v *SettingView) { v.Value = "/usr/local/bin:/usr/bin:/bin:/opt/hostedtoolcache" }),
		"注意書きあり": setting(func(v *SettingView) { v.Note, v.Warn = "変更には再登録が必要", true }),
		"補足のみ":   setting(func(v *SettingView) { v.Note = "既定値のまま" }),
		"長い項目名":  setting(func(v *SettingView) { v.Item = "名前 / work dir / ephemeral / runner group" }),
	}
	for name, v := range views {
		cols := settingColumns()
		assertRowCells(t, name, cols, SettingRow(v, cols, plainStyles()))
	}

	unknown := []token.Column{{ID: "UNKNOWN", Title: "UNKNOWN", Width: 8, Right: false}}
	assertRowCells(t, "知らない列", unknown, SettingRow(sampleSetting(), unknown, plainStyles()))

	if got := SettingRow(sampleSetting(), nil, plainStyles()); len(got) != 0 {
		t.Errorf("列が無いときのセル数 = %d, want 0", len(got))
	}
}

// セルは cols と同じ順で返る（列を並べ替えても中身が付いてくる）。
func TestSettingRowFollowsColumnOrder(t *testing.T) {
	cols := settingColumns()
	reversed := []token.Column{cols[2], cols[1], cols[0]}
	v := setting(func(v *SettingView) { v.Note, v.Warn = "変更には再登録が必要", true })

	cells := SettingRow(v, reversed, plainStyles())
	if len(cells) != len(reversed) {
		t.Fatalf("セル数 = %d, want %d", len(cells), len(reversed))
	}
	want := []string{"変更には再登録が必要", v.Value, v.Item}
	for i, w := range want {
		if !strings.Contains(cells[i], w) {
			t.Errorf("%d 番目のセル = %q, want %q を含む", i, cells[i], w)
		}
	}
}

// 項目名・現在値・注意書きが行に出る。値が無い列は「値なし」の記号になる。
func TestSettingRowContents(t *testing.T) {
	tests := map[string]struct {
		view SettingView
		want []string
	}{
		"項目名と現在値": {sampleSetting(), []string{".env（環境変数・プロキシ・job hooks）", "12 項目"}},
		"注意書き": {
			setting(func(v *SettingView) { v.Note, v.Warn = "変更には再登録が必要", true }),
			[]string{token.IconWarn + " 変更には再登録が必要"},
		},
		"警告でない注記": {
			setting(func(v *SettingView) { v.Note = "既定値のまま" }),
			[]string{"既定値のまま"},
		},
		"値なし": {SettingView{Item: "ラベル", Value: "", Note: "", Warn: false}, []string{token.IconNoUnit}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cols := settingColumns()
			row := strings.Join(SettingRow(tt.view, cols, plainStyles()), " ")
			for _, w := range tt.want {
				if !strings.Contains(row, w) {
					t.Errorf("行に %q が無い: %q", w, row)
				}
			}
		})
	}
}

package config

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/runner"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism"
	"github.com/ousiassllc/gsr-helper/internal/ui/organism/dialog"
	"github.com/ousiassllc/gsr-helper/internal/ui/page"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/configmodal"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest/cmdtest"
)

// 見出しの回帰テスト。**どの見出しをいつ出すか**だけを持つ。切れ目は行数では
// なく責務で引いてある——selfconf_test.go は「保存の経路が何を書くか」、こちらは
// 「画面が何と名乗るか」である（docs/ui/atomic-design.md の「ファイルの行数」）。

// 書き込んだ後に開き直したフォームは、もう初回設定と名乗らないこと（Issue #141）。
//
// 見出しが生の FirstRun で決まっていたころは、同じセッションで書き込んだ後も
// `初回設定（gsr-helper 自身の設定）` のまま残った。設定ファイルはもう存在して
// おり、差分の基準（saveSelf）は保存済みの値へ切り替わっているので、見出しだけが
// 事実と食い違う。
func TestSelfConfigTitleStopsSayingFirstRunAfterSave(t *testing.T) {
	t.Parallel()

	st := pagetest.State(80, 24)
	m := New(tabIndex, st)
	st.Config = page.ConfigDeps{Conf: appconfig.Default(), Path: t.TempDir() + "/config.yaml", FirstRun: true}
	next, cmd := m.Update(st)
	m = cmdtest.Advance(next, cmd, 5).(Model)

	if !contains(m, selfTitleFirst) {
		t.Fatalf("初回設定ウィザードの見出しが %q でない: %s", selfTitleFirst, view(m))
	}

	// 値を触らずに確定して書き込む（初回は差分なしでも書く。FR-41）。
	m, _ = send(t, m, page.ResultMsg{Kind: configmodal.FormKind, Msg: dialog.FormDoneMsg{Form: nil}})
	m, cmd = send(t, m, page.ResultMsg{Kind: configmodal.DiffKind, Msg: dialog.DecidedMsg{Confirmed: true}})
	done, err := doneOf(cmd)
	if err != nil {
		t.Fatalf("書き込みの結果を取れない: %v", err)
	}
	if done.err != nil {
		t.Fatalf("設定の書き込みに失敗: %v", done.err)
	}
	m, _ = send(t, m, done)

	// 同じセッションで対象の一覧から開き直す。
	m, _ = send(t, m, organism.ChosenMsg{ID: selfID, Key: ""})
	if !contains(m, selfTitleEdit) {
		t.Fatalf("自身の設定のフォームが開いていない: %s", view(m))
	}
	if contains(m, selfTitleFirst) {
		t.Errorf("書き込んだ後の見出しが %q のまま: %s", selfTitleFirst, view(m))
	}
}

// 対象選択の見出しが runner に限定していないこと（Issue #142）。
//
// 一覧の最後の 1 件は runner ではなく gsr-helper 自身である。「runner を選べ」と
// 読ませると、自身の設定を編集し直しに来た利用者（FR-42）がこの一覧を通り過ぎる
// ——区切り線の下は視覚的にも本体から切り離されており、なおさら見落としやすい。
func TestPickerTitleCoversSelfAsWellAsRunners(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		rs   []runner.Runner
	}{
		{name: "runner がいる", rs: []runner.Runner{withTempDir(t, "build01-1", "")}},
		// 0 台なら一覧は自身の 1 件だけになる（区切り線も出ない）。
		{name: "runner が 0 台", rs: nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := newPage(t, tt.rs...)
			if !contains(m, titlePicker) {
				t.Fatalf("対象選択の見出しが出ていない: %s", view(m))
			}
			if strings.Contains(titlePicker, "runner") {
				t.Errorf("見出し = %q, want runner に限定しない文言", titlePicker)
			}
			if !contains(m, "gsr-helper 自身の設定") {
				t.Errorf("一覧に自身が並んでいない: %s", view(m))
			}
		})
	}
}

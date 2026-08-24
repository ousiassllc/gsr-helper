package itemview_test

import (
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/config/edit"
	"github.com/ousiassllc/gsr-helper/internal/ui/molecule/listrow"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/config/itemview"
)

// tea.Model を組み立てずに検証できるのがこのパッケージを分けた理由なので、
// ここでは edit.Summary を入力に、表示用の値だけを見る。

// find は Kind で行を探す。
func find(t *testing.T, items []itemview.Item, k edit.Kind) itemview.Item {
	t.Helper()

	for _, it := range items {
		if it.Kind == k {
			return it
		}
	}
	t.Fatalf("Kind = %v の行が無い", k)
	return itemview.Item{}
}

// drop-in は systemd ユニットが無ければ選べず、理由を備考へ出す（screens.md の
// 無効な操作の表示）。
//
// **行を消さずに残すことが要件である。** 消すと「変えられない」ことは分かっても
// 「なぜ変えられないか」を出す場所が無くなる。
func TestDropInDisabledWithoutUnitAndSaysWhy(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		hasUnit     bool
		wantEnabled bool
		wantNote    string
	}{
		"ユニット無し": {hasUnit: false, wantEnabled: false, wantNote: "systemd ユニット無し"},
		"ユニット有り": {hasUnit: true, wantEnabled: true, wantNote: "daemon-reload が要る"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			it := find(t, itemview.MainItems(edit.Summary{Editable: true, HasUnit: tt.hasUnit}), edit.KindDropIn)
			if it.Enabled != tt.wantEnabled {
				t.Errorf("選べるか = %v, want %v", it.Enabled, tt.wantEnabled)
			}
			if it.View.Note != tt.wantNote {
				t.Errorf("備考 = %q, want %q", it.View.Note, tt.wantNote)
			}
		})
	}
}

// 編集できない runner では主要な項目がすべて選べない。
func TestNotEditableDisablesEveryMainItem(t *testing.T) {
	t.Parallel()

	items := itemview.MainItems(edit.Summary{Editable: false, HasUnit: true})
	if len(items) == 0 {
		t.Fatal("主要な項目が 1 行も無い")
	}
	for _, it := range items {
		if it.Enabled {
			t.Errorf("Kind = %v が選べる状態になっている", it.Kind)
		}
	}
}

// .env の現在値は項目数の要約で、0 件なら空にする（数字の 0 を出さない）。
func TestEnvValueSummarizesCount(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		count int
		want  string
	}{
		"0 件は空":   {count: 0, want: ""},
		"1 件以上は数": {count: 3, want: "3 項目"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			it := find(t, itemview.MainItems(edit.Summary{Editable: true, EnvCount: tt.count}), edit.KindEnv)
			if it.View.Value != tt.want {
				t.Errorf("現在値 = %q, want %q", it.View.Value, tt.want)
			}
		})
	}
}

// 絞り込みは項目名と現在値の両方を、大文字小文字を無視して見る。
func TestMatchesLooksAtNameAndValueFolded(t *testing.T) {
	t.Parallel()

	it := itemview.Item{View: listrow.SettingView{Item: ".env", Value: "PATH"}}
	for name, tt := range map[string]struct {
		q    string
		want bool
	}{
		"項目名に一致":       {q: "env", want: true},
		"現在値に一致":       {q: "path", want: true},
		"どちらにも無ければ落ちる": {q: "labels", want: false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := itemview.Matches(it, tt.q); got != tt.want {
				t.Errorf("Matches(%q) = %v, want %v", tt.q, got, tt.want)
			}
		})
	}
}

// 選べない理由は空文字で返す（備考欄が既に持っているので二重に出さない）。
func TestDisabledReasonReturnsOnlyWhether(t *testing.T) {
	t.Parallel()

	reason, disabled := itemview.DisabledReason(itemview.Item{Enabled: false})
	if !disabled {
		t.Error("選べない行が選べる扱いになっている")
	}
	if reason != "" {
		t.Errorf("理由 = %q, want 空文字（備考欄と二重に出さない）", reason)
	}
	if _, disabled := itemview.DisabledReason(itemview.Item{Enabled: true}); disabled {
		t.Error("選べる行が選べない扱いになっている")
	}
}

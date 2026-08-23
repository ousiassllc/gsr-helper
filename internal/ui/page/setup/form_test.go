package setup_test

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// 入力済みのフォームで esc を押したら破棄の確認を挟む。
//
// 確認を出すのは page である（モーダルを重ねられるのは Overlay を持つ page だけ）。
// **確認を出さないと esc が何も起こさず、フォームから出られなくなる。**
func TestDirtyFormAsksBeforeDiscarding(t *testing.T) {
	t.Parallel()

	m := newModel(t, state(t, pagetest.SampleRunner()))
	m = send(t, m, pagetest.Press("n"))

	if !strings.Contains(view(m), "登録先の URL") {
		t.Fatalf("追加フォームが開いていない:\n%s", view(m))
	}

	// 1 文字入れてから esc を押す。
	m = send(t, m, pagetest.Press("h"), pagetest.Press("esc"))

	got := view(m)
	if !strings.Contains(got, "入力の破棄") {
		t.Fatalf("破棄の確認が出ていない:\n%s", got)
	}

	// やめるとフォームへ戻る。
	m = send(t, m, pagetest.Press("n"))
	if body := view(m); !strings.Contains(body, "登録先の URL") {
		t.Errorf("キャンセルしてもフォームへ戻っていない:\n%s", body)
	}

	// 破棄するとメニューへ戻る。
	m = send(t, m, pagetest.Press("esc"), pagetest.Press("y"))
	if body := view(m); !strings.Contains(body, "台数を指定して一括追加") {
		t.Errorf("破棄してもメニューへ戻っていない:\n%s", body)
	}
}

// 未入力のフォームは確認を挟まずに閉じる。
func TestCleanFormClosesImmediately(t *testing.T) {
	t.Parallel()

	m := newModel(t, state(t, pagetest.SampleRunner()))
	m = send(t, m, pagetest.Press("n"), pagetest.Press("esc"))

	got := view(m)
	if strings.Contains(got, "入力の破棄") {
		t.Errorf("未入力なのに破棄の確認を出している:\n%s", got)
	}
	if !strings.Contains(got, "台数を指定して一括追加") {
		t.Errorf("メニューへ戻っていない:\n%s", got)
	}
}

func TestMenuListsThreeEntries(t *testing.T) {
	t.Parallel()

	m := newModel(t, state(t, pagetest.SampleRunner()))
	got := view(m)

	for _, want := range []string{
		"台数を指定して一括追加", "1 台ずつ個別に設定して追加", "バージョンを一括更新",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("メニューに %q が無い:\n%s", want, got)
		}
	}
}

func TestMenuIsGatedByCaps(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mutate func(*appconfig.Caps)
		want   string
	}{
		"非 root": {func(c *appconfig.Caps) { c.Root = false }, "root 権限が必要です"},
		"gh 未認証": {func(c *appconfig.Caps) { c.GitHubToken = false }, "GitHub の認証が必要です"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			st := state(t, pagetest.SampleRunner())
			tt.mutate(&st.Caps)

			got := view(newModel(t, st))
			if !strings.Contains(got, tt.want) {
				t.Errorf("理由 %q が出ていない:\n%s", tt.want, got)
			}
		})
	}
}

// フッタは実行・追加・更新のキーを可否つきで出す。
//
// 可否の理由は action.Allow から引くので、Runners タブのフッタと同じ文言になる。
func TestFooterCarriesKeysAndReasons(t *testing.T) {
	t.Parallel()

	st := state(t, pagetest.SampleRunner())
	st.Caps.GitHubToken = false
	m := newModel(t, st)

	hints := chromeOf(t, m).Footer
	if len(hints) == 0 {
		t.Fatal("フッタが空である")
	}

	seen := make(map[string]string, len(hints))
	for _, h := range hints {
		if h.Enabled == (h.Reason != "") {
			t.Errorf("キー %q = %v/%q, 可否と理由の有無が食い違う", h.Key, h.Enabled, h.Reason)
		}
		seen[h.Key] = h.Reason
	}

	for _, k := range []string{"n", "u"} {
		why, ok := seen[k]
		if !ok {
			t.Errorf("フッタに %q が無い: %v", k, seen)
			continue
		}
		if !strings.Contains(why, "GitHub の認証が必要です") {
			t.Errorf("キー %q の理由 = %q, want 認証を促す文言", k, why)
		}
	}
}

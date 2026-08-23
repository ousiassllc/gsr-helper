package setup_test

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/appconfig"
	"github.com/ousiassllc/gsr-helper/internal/ui/page/pagetest"
)

// フォームの esc は、入力済みのときだけ破棄の確認を挟む。
//
// 確認を出すのは page である（モーダルを重ねられるのは Overlay を持つ page だけ）。
// **確認を出さないと esc が何も起こさず、フォームから出られなくなる。** 逆に未入力
// でも確認を出すと、開いただけのフォームを閉じるのに 2 打鍵かかる。1 本で辿るのは、
// Overlay の状態が写しの間で共有され、途中の Model から枝分かれできないためである。
func TestEscOnFormAsksOnlyWhenDirty(t *testing.T) {
	t.Parallel()

	m := pagetest.Quick(newModel(t, state(t, pagetest.SampleRunner())), pagetest.Press("n"))
	if !strings.Contains(view(m), "登録先の URL") {
		t.Fatalf("追加フォームが開いていない:\n%s", view(m))
	}

	// 未入力なら確認を挟まずメニューへ戻る。
	m = pagetest.Quick(m, pagetest.Press("esc"))
	if got := view(m); strings.Contains(got, "入力の破棄") {
		t.Errorf("未入力なのに破棄の確認を出している:\n%s", got)
	} else if !strings.Contains(got, "台数を指定して一括追加") {
		t.Errorf("メニューへ戻っていない:\n%s", got)
	}

	// 開き直して 1 文字入れてから esc を押すと確認が出る。
	m = pagetest.Quick(m, pagetest.Press("n"), pagetest.Press("h"), pagetest.Press("esc"))
	if got := view(m); !strings.Contains(got, "入力の破棄") {
		t.Fatalf("破棄の確認が出ていない:\n%s", got)
	}

	// やめるとフォームへ戻り、破棄するとメニューへ戻る。
	m = pagetest.Quick(m, pagetest.Press("n"))
	if body := view(m); !strings.Contains(body, "登録先の URL") {
		t.Errorf("キャンセルしてもフォームへ戻っていない:\n%s", body)
	}
	m = pagetest.Quick(m, pagetest.Press("esc"), pagetest.Press("y"))
	if body := view(m); !strings.Contains(body, "台数を指定して一括追加") {
		t.Errorf("破棄してもメニューへ戻っていない:\n%s", body)
	}
}

// メニューは 3 つの入口を出し、能力が欠けていれば入口の代わりに理由を出す。
//
// 可否の理由は action.Allow から引くので、Runners タブと同じ文言になる。
func TestMenuListsEntriesGatedByCaps(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mutate func(*appconfig.Caps)
		want   []string
	}{
		"全能力あり": {func(*appconfig.Caps) {}, []string{
			"台数を指定して一括追加", "1 台ずつ個別に設定して追加", "バージョンを一括更新",
		}},
		"非 root": {func(c *appconfig.Caps) { c.Root = false }, []string{"root 権限が必要です"}},
		"gh 未認証": {
			func(c *appconfig.Caps) { c.GitHubToken = false },
			[]string{"GitHub の認証が必要です"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			st := state(t, pagetest.SampleRunner())
			tt.mutate(&st.Caps)

			got := view(newModel(t, st))
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("メニューに %q が無い:\n%s", w, got)
				}
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

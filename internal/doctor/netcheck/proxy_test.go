package netcheck_test

import (
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/doctor/check"
)

// プロキシの資格情報は Detail / Summary / Impact / Remedy のどこにも出さない。
//
// パスワードに % が入ると url.Parse が失敗し、*url.Error は生の URL を丸ごと
// 埋め込む。エラー文言や生の値を文言へ連結すると、伏せられないまま画面へ写り、
// 診断のスクリーンショット共有で漏れる。
func TestProxyNeverLeaksCredentialsInAnyField(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		env    map[string]string
		hidden []string // どの文言にも現れてはならない断片
		shown  []string // 判断材料として残っていてほしい断片
	}{
		"URL として読めない値（% を含むパスワード）": {
			env:    map[string]string{"http_proxy": "http://user:pa%ss@proxy:3128"},
			hidden: []string{"pa%ss", "%ss"},
			shown:  []string{"http_proxy"},
		},
		"小文字と大文字で違う不正値": {
			env: map[string]string{
				"http_proxy": "http://user:pa%ss@a:3128",
				"HTTP_PROXY": "http://user:zz%tt@b:3128",
			},
			hidden: []string{"pa%ss", "zz%tt"},
		},
		"小文字と大文字で違う正しい値": {
			env: map[string]string{
				"http_proxy": "http://user:s3cret@a:3128",
				"HTTP_PROXY": "http://user:0ther@b:3128",
			},
			hidden: []string{"s3cret", "0ther"},
			shown:  []string{"a:3128", "b:3128"},
		},
		"スキームが無く資格情報を含む値": {
			env:    map[string]string{"http_proxy": "user:s3cret@proxy:3128"},
			hidden: []string{"s3cret"},
			shown:  []string{"http_proxy"},
		},
		"設定値の一覧（describe 経路）": {
			env: map[string]string{
				"https_proxy": "http://user:s3cret@proxy:3128",
				"HTTPS_PROXY": "http://user:s3cret@proxy:3128",
				"no_proxy":    "localhost,127.0.0.1",
				"NO_PROXY":    "localhost,127.0.0.1",
			},
			hidden: []string{"s3cret"},
			// no_proxy はホスト名の並びで資格情報を含まない。伏せると宛先が
			// 分からなくなるので、そのまま出ていてほしい。
			shown: []string{"proxy:3128", "localhost,127.0.0.1"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := run(t, "net.proxy", check.Input{Getenv: env(tt.env)})
			if len(got) == 0 {
				t.Fatal("結果が空")
			}

			var all string
			for _, r := range got {
				for _, field := range []string{r.Summary, r.Detail, r.Impact, r.Remedy} {
					for _, secret := range tt.hidden {
						if strings.Contains(field, secret) {
							t.Errorf("資格情報 %q が画面の文言へ写っている: %s", secret, field)
						}
					}
				}
				all += r.Summary + " / " + r.Detail + " / " + r.Impact + " / " + r.Remedy + "\n"
			}
			for _, want := range tt.shown {
				if !strings.Contains(all, want) {
					t.Errorf("判断材料 %q が文言に無い:\n%s", want, all)
				}
			}
		})
	}
}

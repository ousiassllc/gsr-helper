package envfile_test

import (
	"reflect"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/config/envfile"
)

// 実際の .env に近い、コメント・空行・字下げ・重複キーを含む入力。
const sample = `# ジョブ環境
PATH=/usr/local/bin:/usr/bin:/bin
LANG=ja_JP.UTF-8

# プロキシ
https_proxy=http://old-proxy:3128
no_proxy=localhost

  ImageOS=ubuntu22
`

// 変更しなければ 1 バイトも変わらないこと。これが「変更行だけを置き換える」という
// 約束の土台であり、崩れるとコメントや空行が差分に紛れ込む。
func TestParseRoundTripIsByteIdentical(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"末尾に改行あり":   sample,
		"末尾に改行なし":   "A=1\nB=2",
		"CRLF":      "A=1\r\n# コメント\r\n\r\nB=2\r\n",
		"空":         "",
		"コメントと空行だけ": "# だけ\n\n\n",
		"= を含まない行":  "これは設定ではない\nA=1\n",
		"= で始まる行":   "=値だけ\nA=1\n",
	}

	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := envfile.Parse([]byte(in)).String()
			if got != in {
				t.Errorf("往復後 = %q, want %q", got, in)
			}
		})
	}
}

// 変更したキーの行だけが置き換わり、コメント・空行・他の行は順序どおり残ること
// （受け入れ条件「.env の書き戻しでコメントと空行の順序が保持され、変更行のみが
// 差し替わる」）。
func TestSetReplacesOnlyTargetLine(t *testing.T) {
	t.Parallel()

	f := envfile.Parse([]byte(sample))
	f.Set("https_proxy", "http://new-proxy:3128")

	want := `# ジョブ環境
PATH=/usr/local/bin:/usr/bin:/bin
LANG=ja_JP.UTF-8

# プロキシ
https_proxy=http://new-proxy:3128
no_proxy=localhost

  ImageOS=ubuntu22
`
	if got := f.String(); got != want {
		t.Errorf("Set() 後 =\n%q\nwant\n%q", got, want)
	}
}

// 字下げされた行は字下げを保ったまま置き換わること。
func TestSetKeepsIndent(t *testing.T) {
	t.Parallel()

	f := envfile.Parse([]byte("  ImageOS=ubuntu22\n"))
	f.Set("ImageOS", "ubuntu24")

	if got, want := f.String(), "  ImageOS=ubuntu24\n"; got != want {
		t.Errorf("Set() 後 = %q, want %q", got, want)
	}
}

// 無いキーは末尾に足すこと。末尾に改行が無いファイルにも足せること。
func TestSetAppendsNewKey(t *testing.T) {
	t.Parallel()

	tests := map[string]struct{ in, want string }{
		"末尾に改行あり": {"A=1\n", "A=1\nB=2\n"},
		"末尾に改行なし": {"A=1", "A=1\nB=2\n"},
		"空":       {"", "B=2\n"},
		"CRLF":    {"A=1\r\n", "A=1\r\nB=2\r\n"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := envfile.Parse([]byte(tt.in))
			f.Set("B", "2")
			if got := f.String(); got != tt.want {
				t.Errorf("Set() 後 = %q, want %q", got, tt.want)
			}
		})
	}
}

// Get は = の後ろをそのまま返すこと（引用符も空白もジョブ環境へそのまま渡る）。
func TestGet(t *testing.T) {
	t.Parallel()

	f := envfile.Parse([]byte("A=  値に空白  \nB=\"引用符付き\"\nC=\n# D=コメント\n"))

	tests := map[string]struct {
		key   string
		want  string
		found bool
	}{
		"空白を落とさない":  {"A", "  値に空白  ", true},
		"引用符を外さない":  {"B", "\"引用符付き\"", true},
		"空の値":       {"C", "", true},
		"コメントは拾わない": {"D", "", false},
		"無いキー":      {"E", "", false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, ok := f.Get(tt.key)
			if got != tt.want || ok != tt.found {
				t.Errorf("Get(%q) = %q, %v, want %q, %v", tt.key, got, ok, tt.want, tt.found)
			}
		})
	}
}

// Keys は現れる順に、重複を除いて返すこと。
func TestKeys(t *testing.T) {
	t.Parallel()

	f := envfile.Parse([]byte("B=1\n# コメント\nA=2\n\nB=3\n  C=4\n"))
	want := []string{"B", "A", "C"}

	if got := f.Keys(); !reflect.DeepEqual(got, want) {
		t.Errorf("Keys() = %v, want %v", got, want)
	}
}

// Unset は該当行をすべて取り除き、他の行に触れないこと。
func TestUnset(t *testing.T) {
	t.Parallel()

	f := envfile.Parse([]byte("# 見出し\nA=1\nB=2\nA=3\n"))
	f.Unset("A")

	if got, want := f.String(), "# 見出し\nB=2\n"; got != want {
		t.Errorf("Unset() 後 = %q, want %q", got, want)
	}

	// 無いキーを消しても何も変わらない。
	f.Unset("Z")
	if got, want := f.String(), "# 見出し\nB=2\n"; got != want {
		t.Errorf("無いキーの Unset() 後 = %q, want %q", got, want)
	}
}

// 末尾に改行が無いファイルは、その性質を保ったまま取り除くこと。
func TestUnsetKeepsMissingTrailingNewline(t *testing.T) {
	t.Parallel()

	f := envfile.Parse([]byte("A=1\nB=2"))
	f.Unset("B")

	if got, want := f.String(), "A=1"; got != want {
		t.Errorf("Unset() 後 = %q, want %q", got, want)
	}
}

// 同じキーが複数ある場合、Set は最初の行だけを書き換え、Get と一致すること。
func TestSetAndGetAgreeOnDuplicateKey(t *testing.T) {
	t.Parallel()

	f := envfile.Parse([]byte("A=1\nA=2\n"))
	f.Set("A", "9")

	if got, want := f.String(), "A=9\nA=2\n"; got != want {
		t.Errorf("Set() 後 = %q, want %q", got, want)
	}
	if got, _ := f.Get("A"); got != "9" {
		t.Errorf("Get() = %q, want %q", got, "9")
	}
}

// ゼロ値が空の .env として使えること。
func TestZeroValueIsUsable(t *testing.T) {
	t.Parallel()

	var f envfile.File
	if got := f.String(); got != "" {
		t.Errorf("ゼロ値の String() = %q, want 空", got)
	}
	f.Set("A", "1")
	if got, want := f.String(), "A=1\n"; got != want {
		t.Errorf("Set() 後 = %q, want %q", got, want)
	}
}

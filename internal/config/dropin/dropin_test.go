package dropin_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/config/dropin"
)

// Render は [Service] を必ず書くこと。セクションの無いファイルは systemd が
// 構文エラーとして扱い、ユニット全体の読み込みに失敗する。
func TestRenderAlwaysWritesSection(t *testing.T) {
	t.Parallel()

	got := dropin.DropIn{Directives: nil}.Render()
	if !strings.Contains(got, "[Service]") {
		t.Errorf("Render() に [Service] が無い: %q", got)
	}
	if !strings.HasPrefix(got, "#") {
		t.Errorf("Render() が生成物である旨のコメントで始まっていない: %q", got)
	}
}

// Render と Parse が往復すること。
func TestRenderParseRoundTrip(t *testing.T) {
	t.Parallel()

	want := dropin.DropIn{Directives: []dropin.Directive{
		{Key: "Restart", Value: "always"},
		{Key: "MemoryMax", Value: "4G"},
		{Key: "Environment", Value: "A=1"},
		{Key: "Environment", Value: "B=2"},
	}}

	got := dropin.Parse(want.Render())
	if !reflect.DeepEqual(got, want) {
		t.Errorf("往復後 = %+v, want %+v", got, want)
	}
}

// Parse は [Service] 以外のセクションとコメント・空行を捨てること。
func TestParseKeepsOnlyServiceSection(t *testing.T) {
	t.Parallel()

	in := `# コメント
; 別のコメント

[Unit]
Description=無視される

[Service]
Restart=always
  MemoryMax = 4G

[Install]
WantedBy=無視される
`
	want := []dropin.Directive{
		{Key: "Restart", Value: "always"},
		{Key: "MemoryMax", Value: "4G"},
	}

	if got := dropin.Parse(in).Directives; !reflect.DeepEqual(got, want) {
		t.Errorf("Parse() = %+v, want %+v", got, want)
	}
}

// Set / Get / Unset の基本。空文字の値は「打ち消し」として残すこと。
func TestSetGetUnset(t *testing.T) {
	t.Parallel()

	var d dropin.DropIn
	d.Set("Restart", "always")
	d.Set("MemoryMax", "4G")
	d.Set("Restart", "on-failure")

	if got, ok := d.Get("Restart"); got != "on-failure" || !ok {
		t.Errorf("Get(Restart) = %q, %v, want on-failure, true", got, ok)
	}
	if len(d.Directives) != 2 {
		t.Errorf("Set() で行が増えている: %+v", d.Directives)
	}

	// 空文字は本体の設定を打ち消す意味を持つので、行として残る。
	d.Set("MemoryMax", "")
	if got, ok := d.Get("MemoryMax"); got != "" || !ok {
		t.Errorf("Get(MemoryMax) = %q, %v, want 空, true", got, ok)
	}

	d.Unset("MemoryMax")
	if _, ok := d.Get("MemoryMax"); ok {
		t.Error("Unset() 後も Get() が見つけている")
	}
}

// ユニット名から drop-in のパスを組み立てること。
func TestPath(t *testing.T) {
	t.Parallel()

	got, err := dropin.Path("/tmp/root", "actions.runner.foo.service")
	if err != nil {
		t.Fatalf("Path() でエラー: %v", err)
	}
	want := "/tmp/root/actions.runner.foo.service.d/override.conf"
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}

	// root が空なら既定の配置先を使う。
	got, err = dropin.Path("", "u.service")
	if err != nil {
		t.Fatalf("Path() でエラー: %v", err)
	}
	if want := dropin.DefaultRoot + "/u.service.d/override.conf"; got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

// ユニット名は .service ファイルの中身に由来し、runner 実行ユーザーが書き換え
// られる。パスの外へ抜ける値を弾かないと root が任意の場所へ drop-in を書く。
func TestPathRejectsUnsafeUnitName(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"空":        "",
		"スラッシュ":    "../../etc/systemd/system/sshd",
		"単独のスラッシュ": "a/b.service",
		"逆スラッシュ":   `a\b.service`,
		"カレント":     ".",
		"親":        "..",
		"NUL":      "a\x00b",
	}

	for name, unit := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := dropin.Path("/tmp/root", unit); !errors.Is(err, dropin.ErrBadUnit) {
				t.Fatalf("Path(%q) のエラー = %v, want ErrBadUnit", unit, err)
			}
		})
	}
}

// 読み書きの往復。親ディレクトリが無くても作ること。
func TestLoadSaveRoundTrip(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path, err := dropin.Path(root, "u.service")
	if err != nil {
		t.Fatalf("Path() でエラー: %v", err)
	}

	// 無ければ空。
	got, err := dropin.Load(path)
	if err != nil || len(got.Directives) != 0 {
		t.Fatalf("不在の Load() = %+v, %v, want 空, nil", got, err)
	}

	want := dropin.DropIn{Directives: []dropin.Directive{{Key: "Restart", Value: "always"}}}
	if err := dropin.Save(want, path); err != nil {
		t.Fatalf("Save() でエラー: %v", err)
	}

	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("drop-in ディレクトリが作られていない: %v", err)
	}

	got, err = dropin.Load(path)
	if err != nil {
		t.Fatalf("Load() でエラー: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

// 単一値のディレクティブが重複していたら、書き換えたうえで後続を落とすこと。
//
// systemd は最後に書かれた値を採るため、最初だけ書き換えて後続を残すと、
// 書き込んだ内容は承認どおりなのに効く値が変わらない。
func TestSetCollapsesDuplicateKeys(t *testing.T) {
	t.Parallel()

	d := dropin.Parse("[Service]\nMemoryMax=1G\nRestart=always\nMemoryMax=8G\n")
	d.Set("MemoryMax", "4G")

	got := d.Render()
	if strings.Count(got, "MemoryMax=") != 1 {
		t.Errorf("MemoryMax の行が 1 つになっていない:\n%s", got)
	}
	if !strings.Contains(got, "MemoryMax=4G") {
		t.Errorf("書き換えた値が入っていない:\n%s", got)
	}
	if !strings.Contains(got, "Restart=always") {
		t.Errorf("別のキーまで落ちている:\n%s", got)
	}
	if v, _ := d.Get("MemoryMax"); v != "4G" {
		t.Errorf("Get() = %q, want 4G", v)
	}
}
